//go:build darwin

package worktree

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

func clonePath(source, destination string) error {
	err := unix.Clonefile(source, destination, 0)
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EXDEV) || errors.Is(err, unix.EEXIST) {
		return fmt.Errorf("%w: %v", errCloneUnavailable, err)
	}
	return err
}
