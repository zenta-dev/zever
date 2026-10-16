//go:build !unix

package local

import (
	"errors"
	"os"
)

// checkDefaultRoot refuses a pre-existing default root that is a symlink or
// not a directory. A missing root is fine.
func checkDefaultRoot(root string) error {
	fi, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return errors.New("not a plain directory")
	}

	return nil
}
