//go:build unix

package local

import (
	"errors"
	"os"
	"syscall"
)

// checkDefaultRoot refuses a pre-existing default root that is a symlink, not
// a directory, or owned by another user: /tmp/storage is predictable, so
// another local user could otherwise pre-create it (or point a symlink at a
// victim directory that New would then chmod). A missing root is fine.
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

	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return errors.New("owned by another user")
	}

	return nil
}
