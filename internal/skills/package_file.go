package skills

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

var ErrPackageFileNotFound = errors.New("skill package file not found")

// ReadPackageFile reads one operator-selected package through a bounded,
// regular-file-only path. Errors deliberately omit the supplied source path.
func ReadPackageFile(ctx context.Context, value string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(value)
	if name == "" {
		return nil, errors.New("invalid skill package path: path is required")
	}
	if name != value {
		return nil, errors.New("invalid skill package path: leading or trailing whitespace is forbidden")
	}
	before, err := os.Lstat(name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrPackageFileNotFound
		}
		return nil, errors.New("invalid skill package path: package file cannot be inspected")
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return nil, errors.New("invalid skill package path: package must be a non-symlink regular file")
	}
	if before.Size() <= 0 || before.Size() > MaxPackageArchiveBytes {
		return nil, fmt.Errorf("invalid skill package path: archive must contain between 1 and %d bytes", MaxPackageArchiveBytes)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	file, err := os.Open(name)
	if err != nil {
		return nil, errors.New("invalid skill package path: package file cannot be opened")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, errors.New("invalid skill package path: package file identity cannot be verified")
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) ||
		before.Size() != opened.Size() || !before.ModTime().Equal(opened.ModTime()) {
		return nil, errors.New("invalid skill package path: package changed before it was opened")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	raw, err := io.ReadAll(io.LimitReader(file, MaxPackageArchiveBytes+1))
	if err != nil {
		return nil, errors.New("invalid skill package path: package file cannot be read")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil {
		return nil, errors.New("invalid skill package path: package file identity cannot be reverified")
	}
	if !after.Mode().IsRegular() || !os.SameFile(opened, after) ||
		opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) ||
		int64(len(raw)) != after.Size() {
		return nil, errors.New("invalid skill package path: package changed while it was read")
	}
	if len(raw) == 0 || len(raw) > MaxPackageArchiveBytes {
		return nil, fmt.Errorf("invalid skill package path: archive must contain between 1 and %d bytes", MaxPackageArchiveBytes)
	}
	return raw, nil
}

// ValidatePackageFile returns only the immutable metadata preview. It never
// exposes the selected path or untrusted Skill body to its caller.
func ValidatePackageFile(ctx context.Context, value string) (PackagePreview, error) {
	raw, err := ReadPackageFile(ctx, value)
	if err != nil {
		return PackagePreview{}, err
	}
	parsed, err := ParsePackage(raw)
	if err != nil {
		return PackagePreview{}, err
	}
	return parsed.Preview(), nil
}
