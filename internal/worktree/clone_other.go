//go:build !darwin

package worktree

func clonePath(_, _ string) error {
	return errCloneUnavailable
}
