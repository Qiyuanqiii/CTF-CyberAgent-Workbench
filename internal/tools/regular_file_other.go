//go:build windows || plan9 || js || wasip1

package tools

import (
	"fmt"
	"os"
)

func openRegularFile(path string) (*os.File, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) {
		_ = file.Close()
		return nil, fmt.Errorf("%s changed while it was being opened", path)
	}
	return file, nil
}
