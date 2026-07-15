//go:build linux

package sandbox

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

const maxHostInputTraversalDepth = 64

func validateHostInputArchiveName(value string) error {
	if value == "" || !utf8.ValidString(value) || len([]byte(value)) > MaxHostInputPathBytes ||
		strings.ContainsRune(value, 0) || strings.HasPrefix(value, "/") ||
		path.Clean(value) != value || value == "." || value == ".." ||
		strings.HasPrefix(value, "../") {
		return fmt.Errorf("host input archive name %q is invalid", value)
	}
	return nil
}

type linuxDockerHostInputStager struct {
	afterPin func()
}

type pinnedHostInputEntry struct {
	archiveName string
	kind        byte
	file        *os.File
	stat        unix.Stat_t
	content     []byte
	digest      string
}

type sealedHostInputBundle struct {
	file   *os.File
	report HostInputBundleReport
}

func (bundle *sealedHostInputBundle) Read(data []byte) (int, error) {
	if bundle == nil || bundle.file == nil {
		return 0, os.ErrClosed
	}
	return bundle.file.Read(data)
}

func (bundle *sealedHostInputBundle) Seek(offset int64, whence int) (int64, error) {
	if bundle == nil || bundle.file == nil {
		return 0, os.ErrClosed
	}
	return bundle.file.Seek(offset, whence)
}

func (bundle *sealedHostInputBundle) Report() HostInputBundleReport {
	if bundle == nil {
		return HostInputBundleReport{}
	}
	return bundle.report
}

func (bundle *sealedHostInputBundle) Close() error {
	if bundle == nil || bundle.file == nil {
		return nil
	}
	err := bundle.file.Close()
	bundle.file = nil
	return err
}

func NewLocalDockerHostInputStager() DockerHostInputStager {
	return &linuxDockerHostInputStager{}
}

func (stager *linuxDockerHostInputStager) Probe(ctx context.Context,
	workspaceRoot string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rootFD, err := openHostInputRoot(workspaceRoot)
	if err != nil {
		return err
	}
	defer unix.Close(rootFD)
	probeFD, err := openHostInputAt(rootFD, ".")
	if err != nil {
		return err
	}
	_ = unix.Close(probeFD)
	file, err := createSealableHostInputMemfd()
	if err != nil {
		return err
	}
	defer file.Close()
	return sealHostInputMemfd(file)
}

func (stager *linuxDockerHostInputStager) Stage(ctx context.Context,
	request HostInputBundleRequest,
) (HostInputBundleReport, error) {
	bundle, err := stager.Capture(ctx, request)
	if err != nil {
		return HostInputBundleReport{}, err
	}
	report := bundle.Report()
	if closeErr := bundle.Close(); closeErr != nil {
		return HostInputBundleReport{}, newDockerHostInputStagingError(
			DockerHostInputStagingErrorSealFailed)
	}
	return report, report.Validate()
}

func (stager *linuxDockerHostInputStager) Capture(ctx context.Context,
	request HostInputBundleRequest,
) (HostInputBundle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	normalized, _ := NormalizeManifest(request.Manifest)
	rootFD, err := openHostInputRoot(request.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	defer unix.Close(rootFD)

	entries := make([]pinnedHostInputEntry, 0, 128)
	closeEntries := func() {
		for index := range entries {
			if entries[index].file != nil {
				_ = entries[index].file.Close()
			}
		}
	}
	defer closeEntries()

	readOnlyOrdinal := 0
	for _, mount := range normalized.Mounts {
		if mount.Access != MountReadOnly {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		readOnlyOrdinal++
		archiveRoot := fmt.Sprintf("mounts/%03d", readOnlyOrdinal)
		fd, openErr := openHostInputAt(rootFD, mount.Source)
		if openErr != nil {
			return nil, fmt.Errorf("pin read-only mount %d: %w",
				readOnlyOrdinal, openErr)
		}
		if walkErr := pinHostInputTree(ctx, fd, archiveRoot, 0, &entries); walkErr != nil {
			return nil, fmt.Errorf("pin read-only mount %d: %w",
				readOnlyOrdinal, walkErr)
		}
	}
	if len(entries)+len(request.Artifacts) > MaxHostInputBundleEntries {
		return nil, newDockerHostInputStagingError(
			DockerHostInputStagingErrorResourceLimit)
	}
	if stager != nil && stager.afterPin != nil {
		stager.afterPin()
	}

	bundle, err := bundlePinnedHostInputs(ctx, entries, request.Artifacts, readOnlyOrdinal)
	if err != nil {
		return nil, err
	}
	return bundle, nil
}

func openHostInputRoot(workspaceRoot string) (int, error) {
	if workspaceRoot == "" || !pathIsAbsoluteClean(workspaceRoot) {
		return -1, newDockerHostInputStagingError(DockerHostInputStagingErrorUnsafeSource)
	}
	how := &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW),
		Resolve: uint64(unix.RESOLVE_NO_MAGICLINKS | unix.RESOLVE_NO_SYMLINKS),
	}
	fd, err := unix.Openat2(unix.AT_FDCWD, workspaceRoot, how)
	if err != nil {
		if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) {
			return -1, newDockerHostInputStagingError(DockerHostInputStagingErrorUnsupported)
		}
		return -1, newDockerHostInputStagingError(DockerHostInputStagingErrorUnsafeSource)
	}
	return fd, nil
}

func pathIsAbsoluteClean(value string) bool {
	return strings.HasPrefix(value, "/") && utf8.ValidString(value) &&
		!strings.ContainsRune(value, 0) && len([]byte(value)) <= MaxHostInputWorkspaceRootBytes &&
		path.Clean(value) == value
}

func openHostInputAt(parentFD int, name string) (int, error) {
	resolve := uint64(unix.RESOLVE_BENEATH | unix.RESOLVE_NO_MAGICLINKS |
		unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV)
	probeFD, err := unix.Openat2(parentFD, name, &unix.OpenHow{
		Flags:   uint64(unix.O_PATH | unix.O_CLOEXEC | unix.O_NOFOLLOW),
		Resolve: resolve,
	})
	if err != nil {
		return -1, classifyHostInputOpenError(err)
	}
	defer unix.Close(probeFD)
	var probeStat unix.Stat_t
	if unix.Fstat(probeFD, &probeStat) != nil {
		return -1, newDockerHostInputStagingError(DockerHostInputStagingErrorUnsafeSource)
	}
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	switch probeStat.Mode & unix.S_IFMT {
	case unix.S_IFREG:
	case unix.S_IFDIR:
		flags |= unix.O_DIRECTORY
	default:
		return -1, newDockerHostInputStagingError(DockerHostInputStagingErrorUnsafeSource)
	}
	fd, err := unix.Openat2(parentFD, name, &unix.OpenHow{
		Flags:   uint64(flags),
		Resolve: resolve,
	})
	if err != nil {
		return -1, classifyHostInputOpenError(err)
	}
	var openedStat unix.Stat_t
	if unix.Fstat(fd, &openedStat) != nil || openedStat.Dev != probeStat.Dev ||
		openedStat.Ino != probeStat.Ino || openedStat.Mode != probeStat.Mode {
		_ = unix.Close(fd)
		return -1, newDockerHostInputStagingError(DockerHostInputStagingErrorSourceChanged)
	}
	return fd, nil
}

func classifyHostInputOpenError(err error) error {
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) {
		return newDockerHostInputStagingError(DockerHostInputStagingErrorUnsupported)
	}
	return newDockerHostInputStagingError(DockerHostInputStagingErrorUnsafeSource)
}

func pinHostInputTree(ctx context.Context, fd int, archiveName string, depth int,
	entries *[]pinnedHostInputEntry,
) error {
	if depth > maxHostInputTraversalDepth || validateHostInputArchiveName(archiveName) != nil {
		return newDockerHostInputStagingError(DockerHostInputStagingErrorResourceLimit)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = unix.Close(fd)
		return newDockerHostInputStagingError(DockerHostInputStagingErrorUnsafeSource)
	}
	mode := stat.Mode & unix.S_IFMT
	file := os.NewFile(uintptr(fd), "host-input")
	if file == nil {
		_ = unix.Close(fd)
		return newDockerHostInputStagingError(DockerHostInputStagingErrorUnsafeSource)
	}
	entry := pinnedHostInputEntry{archiveName: archiveName, file: file, stat: stat}
	switch mode {
	case unix.S_IFREG:
		if stat.Nlink != 1 || stat.Size < 0 || stat.Size > MaxHostInputSourceBytes {
			_ = file.Close()
			return newDockerHostInputStagingError(DockerHostInputStagingErrorUnsafeSource)
		}
		entry.kind = tar.TypeReg
		*entries = append(*entries, entry)
	case unix.S_IFDIR:
		entry.kind = tar.TypeDir
		*entries = append(*entries, entry)
		if len(*entries) > MaxHostInputBundleEntries {
			return newDockerHostInputStagingError(DockerHostInputStagingErrorResourceLimit)
		}
		children, err := readBoundedHostInputDirectory(ctx, file,
			MaxHostInputBundleEntries-len(*entries))
		if err != nil {
			if DockerHostInputStagingErrorCode(err) != "" || errors.Is(err, context.Canceled) ||
				errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return newDockerHostInputStagingError(DockerHostInputStagingErrorUnsafeSource)
		}
		sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
		for _, child := range children {
			if err := ctx.Err(); err != nil {
				return err
			}
			name := child.Name()
			if name == "" || name == "." || name == ".." || strings.Contains(name, "/") ||
				strings.ContainsRune(name, 0) || !utf8.ValidString(name) {
				return newDockerHostInputStagingError(DockerHostInputStagingErrorUnsafeSource)
			}
			childFD, err := openHostInputAt(fd, name)
			if err != nil {
				return err
			}
			if err := pinHostInputTree(ctx, childFD, path.Join(archiveName, name),
				depth+1, entries); err != nil {
				return err
			}
			if len(*entries) > MaxHostInputBundleEntries {
				return newDockerHostInputStagingError(DockerHostInputStagingErrorResourceLimit)
			}
		}
	default:
		_ = file.Close()
		return newDockerHostInputStagingError(DockerHostInputStagingErrorUnsafeSource)
	}
	return nil
}

func readBoundedHostInputDirectory(ctx context.Context, file *os.File,
	remaining int,
) ([]os.DirEntry, error) {
	if file == nil || remaining < 0 {
		return nil, newDockerHostInputStagingError(DockerHostInputStagingErrorResourceLimit)
	}
	children := make([]os.DirEntry, 0, min(remaining, 256))
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batchLimit := min(256, remaining-len(children)+1)
		if batchLimit < 1 {
			return nil, newDockerHostInputStagingError(DockerHostInputStagingErrorResourceLimit)
		}
		batch, err := file.ReadDir(batchLimit)
		children = append(children, batch...)
		if len(children) > remaining {
			return nil, newDockerHostInputStagingError(DockerHostInputStagingErrorResourceLimit)
		}
		if errors.Is(err, io.EOF) {
			return children, nil
		}
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			return nil, newDockerHostInputStagingError(
				DockerHostInputStagingErrorUnsafeSource)
		}
	}
}

func bundlePinnedHostInputs(ctx context.Context, entries []pinnedHostInputEntry,
	artifacts []HostInputArtifact, readOnlyMountCount int,
) (HostInputBundle, error) {
	file, err := createSealableHostInputMemfd()
	if err != nil {
		return nil, err
	}
	keepOpen := false
	defer func() {
		if !keepOpen {
			_ = file.Close()
		}
	}()
	bundleHash := sha256.New()
	written := &countingWriter{writer: io.MultiWriter(file, bundleHash)}
	tarWriter := tar.NewWriter(written)
	sourceParts := []string{"sandbox_host_input_source_snapshot.v1", strconv.Itoa(len(entries))}
	regularFiles, directories := 0, 0
	var sourceBytes int64
	for index := range entries {
		if err := ctx.Err(); err != nil {
			_ = tarWriter.Close()
			return nil, err
		}
		entry := &entries[index]
		if err := verifyPinnedHostInputStat(entry); err != nil {
			_ = tarWriter.Close()
			return nil, err
		}
		header := &tar.Header{
			Name: entry.archiveName, Mode: 0o555, ModTime: time.Unix(0, 0).UTC(),
			AccessTime: time.Unix(0, 0).UTC(), ChangeTime: time.Unix(0, 0).UTC(),
			Uid: 65532, Gid: 65532, Uname: "", Gname: "", Format: tar.FormatPAX,
		}
		if entry.kind == tar.TypeDir {
			directories++
			header.Typeflag = tar.TypeDir
			header.Name += "/"
			header.Size = 0
			entry.digest = fingerprint("sandbox_host_input_directory.v1", entry.archiveName)
		} else {
			regularFiles++
			content, readErr := readPinnedHostInputFile(ctx, entry)
			if readErr != nil {
				_ = tarWriter.Close()
				return nil, readErr
			}
			entry.content = content
			entry.digest = hashHostInputBytes(content)
			sourceBytes += int64(len(content))
			if sourceBytes > MaxHostInputSourceBytes {
				_ = tarWriter.Close()
				return nil, newDockerHostInputStagingError(
					DockerHostInputStagingErrorResourceLimit)
			}
			header.Typeflag = tar.TypeReg
			header.Mode = 0o444
			header.Size = int64(len(content))
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			_ = tarWriter.Close()
			return nil, hostInputBundleWriteError(err)
		}
		if len(entry.content) > 0 {
			if _, err := tarWriter.Write(entry.content); err != nil {
				_ = tarWriter.Close()
				return nil, hostInputBundleWriteError(err)
			}
		}
		sourceParts = append(sourceParts, fingerprint("sandbox_host_input_archive_path.v1",
			entry.archiveName), strconv.Itoa(int(entry.kind)), strconv.FormatInt(header.Size, 10),
			entry.digest)
	}

	var artifactBytes int64
	for _, artifact := range artifacts {
		if err := ctx.Err(); err != nil {
			_ = tarWriter.Close()
			return nil, err
		}
		name := fmt.Sprintf("artifacts/%03d", artifact.Ordinal)
		header := &tar.Header{
			Name: name, Typeflag: tar.TypeReg, Mode: 0o444, Size: artifact.SizeBytes,
			ModTime: time.Unix(0, 0).UTC(), AccessTime: time.Unix(0, 0).UTC(),
			ChangeTime: time.Unix(0, 0).UTC(), Uid: 65532, Gid: 65532, Format: tar.FormatPAX,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			_ = tarWriter.Close()
			return nil, hostInputBundleWriteError(err)
		}
		if _, err := io.WriteString(tarWriter, artifact.Content); err != nil {
			_ = tarWriter.Close()
			return nil, hostInputBundleWriteError(err)
		}
		artifactBytes += artifact.SizeBytes
	}
	if err := tarWriter.Close(); err != nil {
		return nil, hostInputBundleWriteError(err)
	}
	if written.count < 1 || written.count > MaxHostInputBundleBytes {
		return nil, newDockerHostInputStagingError(
			DockerHostInputStagingErrorResourceLimit)
	}
	if err := sealHostInputMemfd(file); err != nil {
		return nil, err
	}
	bundleDigest := hex.EncodeToString(bundleHash.Sum(nil))
	if err := verifySealedHostInputMemfd(file, written.count, bundleDigest); err != nil {
		return nil, err
	}
	report, err := NewHostInputBundleReport(HostInputBundleMeasurements{
		ReadOnlyMountCount: readOnlyMountCount, ArtifactCount: len(artifacts),
		RegularFileCount: regularFiles, DirectoryCount: directories,
		SourceBytes: sourceBytes, ArtifactBytes: artifactBytes, BundleBytes: written.count,
		SourceSnapshotDigest:  fingerprint(sourceParts...),
		ArtifactPayloadDigest: hostInputArtifactPayloadDigest(artifacts),
		BundleDigest:          bundleDigest,
	}, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, newDockerHostInputStagingError(DockerHostInputStagingErrorSealFailed)
	}
	keepOpen = true
	return &sealedHostInputBundle{file: file, report: report}, nil
}

func hostInputBundleWriteError(err error) error {
	if DockerHostInputStagingErrorCode(err) == DockerHostInputStagingErrorResourceLimit {
		return err
	}
	return newDockerHostInputStagingError(DockerHostInputStagingErrorSealFailed)
}

func readPinnedHostInputFile(ctx context.Context, entry *pinnedHostInputEntry) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := entry.file.Seek(0, io.SeekStart); err != nil {
		return nil, newDockerHostInputStagingError(DockerHostInputStagingErrorUnsafeSource)
	}
	content, err := io.ReadAll(io.LimitReader(&hostInputContextReader{
		ctx: ctx, reader: entry.file,
	}, MaxHostInputSourceBytes+1))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, newDockerHostInputStagingError(DockerHostInputStagingErrorSourceChanged)
	}
	if int64(len(content)) > MaxHostInputSourceBytes {
		return nil, newDockerHostInputStagingError(DockerHostInputStagingErrorResourceLimit)
	}
	if int64(len(content)) != entry.stat.Size || verifyPinnedHostInputStat(entry) != nil {
		return nil, newDockerHostInputStagingError(DockerHostInputStagingErrorSourceChanged)
	}
	return content, nil
}

type hostInputContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *hostInputContextReader) Read(value []byte) (int, error) {
	if reader == nil || reader.reader == nil {
		return 0, syscall.EBADF
	}
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(value)
}

func verifyPinnedHostInputStat(entry *pinnedHostInputEntry) error {
	var current unix.Stat_t
	if entry == nil || entry.file == nil || unix.Fstat(int(entry.file.Fd()), &current) != nil ||
		current.Dev != entry.stat.Dev || current.Ino != entry.stat.Ino ||
		current.Mode != entry.stat.Mode || current.Nlink != entry.stat.Nlink ||
		current.Size != entry.stat.Size || current.Mtim != entry.stat.Mtim ||
		current.Ctim != entry.stat.Ctim {
		return newDockerHostInputStagingError(DockerHostInputStagingErrorSourceChanged)
	}
	return nil
}

func createSealableHostInputMemfd() (*os.File, error) {
	fd, err := unix.MemfdCreate("cyberagent-host-inputs",
		unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		return nil, newDockerHostInputStagingError(DockerHostInputStagingErrorUnsupported)
	}
	file := os.NewFile(uintptr(fd), "cyberagent-host-inputs")
	if file == nil {
		_ = unix.Close(fd)
		return nil, newDockerHostInputStagingError(DockerHostInputStagingErrorSealFailed)
	}
	return file, nil
}

func sealHostInputMemfd(file *os.File) error {
	if file == nil {
		return newDockerHostInputStagingError(DockerHostInputStagingErrorSealFailed)
	}
	seals := unix.F_SEAL_SEAL | unix.F_SEAL_SHRINK | unix.F_SEAL_GROW | unix.F_SEAL_WRITE
	if _, err := unix.FcntlInt(file.Fd(), unix.F_ADD_SEALS, seals); err != nil {
		return newDockerHostInputStagingError(DockerHostInputStagingErrorSealFailed)
	}
	actual, err := unix.FcntlInt(file.Fd(), unix.F_GET_SEALS, 0)
	if err != nil || actual&seals != seals {
		return newDockerHostInputStagingError(DockerHostInputStagingErrorSealFailed)
	}
	return nil
}

func verifySealedHostInputMemfd(file *os.File, size int64, expectedDigest string) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return newDockerHostInputStagingError(DockerHostInputStagingErrorSealFailed)
	}
	hasher := sha256.New()
	written, err := io.Copy(hasher, io.LimitReader(file, MaxHostInputBundleBytes+1))
	if err != nil || written != size || written > MaxHostInputBundleBytes ||
		hex.EncodeToString(hasher.Sum(nil)) != expectedDigest {
		return newDockerHostInputStagingError(DockerHostInputStagingErrorSealFailed)
	}
	return nil
}

type countingWriter struct {
	writer io.Writer
	count  int64
}

func (writer *countingWriter) Write(value []byte) (int, error) {
	if writer == nil || writer.writer == nil {
		return 0, syscall.EBADF
	}
	written, err := writer.writer.Write(value)
	writer.count += int64(written)
	if writer.count > MaxHostInputBundleBytes {
		return written, newDockerHostInputStagingError(DockerHostInputStagingErrorResourceLimit)
	}
	return written, err
}
