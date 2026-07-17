package skills

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

type PackageObjectStore interface {
	Put(context.Context, []byte, PackageObjectDescriptor) (PackageObjectReceipt, error)
	Verify(context.Context, PackageObjectDescriptor) (PackageObjectReceipt, error)
}

// LocalPackageObjectStore stores only validated deterministic ZIP bytes. It has
// no removal or execution method.
type LocalPackageObjectStore struct {
	home string
}

func NewLocalPackageObjectStore(home string) (*LocalPackageObjectStore, error) {
	if home == "" {
		return nil, errors.New("skill package object home is required")
	}
	absolute, err := filepath.Abs(home)
	if err != nil {
		return nil, errors.New("skill package object home is invalid")
	}
	return &LocalPackageObjectStore{home: absolute}, nil
}

func (s *LocalPackageObjectStore) Put(ctx context.Context, raw []byte,
	descriptor PackageObjectDescriptor,
) (PackageObjectReceipt, error) {
	if s == nil {
		return PackageObjectReceipt{}, errors.New("skill package object store is required")
	}
	if err := descriptor.Validate(); err != nil {
		return PackageObjectReceipt{}, err
	}
	if err := validatePackageObjectBytes(raw, descriptor); err != nil {
		return PackageObjectReceipt{}, err
	}
	if err := ctx.Err(); err != nil {
		return PackageObjectReceipt{}, err
	}
	if err := os.MkdirAll(s.home, 0o700); err != nil {
		return PackageObjectReceipt{}, errors.New("skill package object home cannot be prepared")
	}
	root, err := os.OpenRoot(s.home)
	if err != nil {
		return PackageObjectReceipt{}, errors.New("skill package object home cannot be opened")
	}
	defer root.Close()
	objectKey, _ := PackageObjectKey(descriptor.ArchiveSHA256)
	parent := path.Dir(path.Join(PackageObjectRoot, objectKey))
	if err := root.MkdirAll(parent, 0o700); err != nil {
		return PackageObjectReceipt{}, errors.New("skill package object directory cannot be prepared")
	}
	fullKey := path.Join(PackageObjectRoot, objectKey)
	if receipt, found, verifyErr := verifyPackageObjectAt(ctx, root, fullKey, objectKey,
		descriptor); verifyErr != nil {
		return PackageObjectReceipt{}, verifyErr
	} else if found {
		return receipt, nil
	}
	temporary := parent + "/." + descriptor.ArchiveSHA256 + "." + randomObjectSuffix() + ".tmp"
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return PackageObjectReceipt{}, errors.New("skill package temporary object cannot be created")
	}
	defer func() {
		_ = file.Close()
		_ = root.Remove(temporary)
	}()
	if err := writePackageObject(ctx, file, raw); err != nil {
		return PackageObjectReceipt{}, err
	}
	if err := file.Sync(); err != nil {
		return PackageObjectReceipt{}, errors.New("skill package temporary object cannot be synchronized")
	}
	if err := file.Close(); err != nil {
		return PackageObjectReceipt{}, errors.New("skill package temporary object cannot be closed")
	}
	if err := ctx.Err(); err != nil {
		return PackageObjectReceipt{}, err
	}
	if err := root.Link(temporary, fullKey); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return PackageObjectReceipt{}, errors.New("skill package object cannot be published atomically")
		}
	}
	if receipt, found, verifyErr := verifyPackageObjectAt(ctx, root, fullKey, objectKey,
		descriptor); verifyErr != nil {
		return PackageObjectReceipt{}, verifyErr
	} else if !found {
		return PackageObjectReceipt{}, errors.New("skill package object publication was not durable")
	} else {
		_ = syncPackageObjectDirectory(root, parent)
		return receipt, nil
	}
}

func (s *LocalPackageObjectStore) Verify(ctx context.Context,
	descriptor PackageObjectDescriptor,
) (PackageObjectReceipt, error) {
	if s == nil {
		return PackageObjectReceipt{}, errors.New("skill package object store is required")
	}
	if err := descriptor.Validate(); err != nil {
		return PackageObjectReceipt{}, err
	}
	if err := ctx.Err(); err != nil {
		return PackageObjectReceipt{}, err
	}
	root, err := os.OpenRoot(s.home)
	if err != nil {
		return PackageObjectReceipt{}, errors.New("skill package object home cannot be opened")
	}
	defer root.Close()
	objectKey, _ := PackageObjectKey(descriptor.ArchiveSHA256)
	receipt, found, err := verifyPackageObjectAt(ctx, root,
		path.Join(PackageObjectRoot, objectKey), objectKey, descriptor)
	if err != nil {
		return PackageObjectReceipt{}, err
	}
	if !found {
		return PackageObjectReceipt{}, errors.New("skill package object is missing")
	}
	return receipt, nil
}

func verifyPackageObjectAt(ctx context.Context, root *os.Root, fullKey, objectKey string,
	descriptor PackageObjectDescriptor,
) (PackageObjectReceipt, bool, error) {
	info, err := root.Lstat(fullKey)
	if errors.Is(err, fs.ErrNotExist) {
		return PackageObjectReceipt{}, false, nil
	}
	if err != nil {
		return PackageObjectReceipt{}, false, errors.New("skill package object cannot be inspected")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() ||
		info.Size() != int64(descriptor.ArchiveBytes) {
		return PackageObjectReceipt{}, false, errors.New("skill package object identity is invalid")
	}
	file, err := root.Open(fullKey)
	if err != nil {
		return PackageObjectReceipt{}, false, errors.New("skill package object cannot be opened")
	}
	defer file.Close()
	raw, err := readPackageObject(ctx, file, MaxPackageArchiveBytes)
	if err != nil {
		return PackageObjectReceipt{}, false, err
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(info, after) || after.Size() != int64(len(raw)) {
		return PackageObjectReceipt{}, false, errors.New("skill package object changed while it was read")
	}
	if err := validatePackageObjectBytes(raw, descriptor); err != nil {
		return PackageObjectReceipt{}, false, err
	}
	return PackageObjectReceipt{Descriptor: descriptor, ObjectKey: objectKey}, true, nil
}

func validatePackageObjectBytes(raw []byte, descriptor PackageObjectDescriptor) error {
	if len(raw) != descriptor.ArchiveBytes {
		return errors.New("skill package object byte count does not match its descriptor")
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != descriptor.ArchiveSHA256 {
		return errors.New("skill package object digest does not match its descriptor")
	}
	parsed, err := ParsePackage(raw)
	if err != nil {
		return fmt.Errorf("skill package object failed strict validation: %w", err)
	}
	preview := parsed.Preview()
	if preview.ArchiveSHA256 != descriptor.ArchiveSHA256 ||
		preview.PackageFingerprint != descriptor.PackageFingerprint ||
		preview.ArchiveBytes != descriptor.ArchiveBytes {
		return errors.New("skill package object semantic fingerprint does not match its descriptor")
	}
	return nil
}

func writePackageObject(ctx context.Context, writer io.Writer, raw []byte) error {
	for offset := 0; offset < len(raw); {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := min(offset+4096, len(raw))
		written, err := writer.Write(raw[offset:end])
		if err != nil {
			return errors.New("skill package temporary object cannot be written")
		}
		if written <= 0 {
			return io.ErrShortWrite
		}
		offset += written
	}
	return nil
}

func readPackageObject(ctx context.Context, reader io.Reader, limit int) ([]byte, error) {
	var buffer bytes.Buffer
	chunk := make([]byte, 4096)
	for buffer.Len() <= limit {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		remaining := limit + 1 - buffer.Len()
		readSize := min(len(chunk), remaining)
		count, err := reader.Read(chunk[:readSize])
		if count > 0 {
			_, _ = buffer.Write(chunk[:count])
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.New("skill package object cannot be read")
		}
		if count == 0 {
			return nil, io.ErrNoProgress
		}
	}
	if buffer.Len() > limit {
		return nil, errors.New("skill package object exceeds its size bound")
	}
	return buffer.Bytes(), nil
}

func randomObjectSuffix() string {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "fallback"
	}
	return hex.EncodeToString(value[:])
}

func syncPackageObjectDirectory(root *os.Root, name string) error {
	directory, err := root.Open(name)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
