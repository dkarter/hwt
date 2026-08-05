package worktree

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type RemoveOptions struct {
	WorkspaceID string
	Force       bool
}

type RemoveResult struct {
	WorkspaceID string `json:"workspace_id"`
	Path        string `json:"path"`
}

func Remove(client Client, options RemoveOptions) (RemoveResult, error) {
	var err error
	if options.WorkspaceID == "" {
		options.WorkspaceID, err = client.CurrentWorkspaceID()
		if err != nil {
			return RemoveResult{}, err
		}
	}
	workspace, err := client.Workspace(options.WorkspaceID)
	if err != nil {
		return RemoveResult{}, err
	}
	if workspace.CheckoutPath == "" || !workspace.LinkedWorktree {
		return RemoveResult{}, fmt.Errorf("workspace %s is not a linked Herdr worktree", options.WorkspaceID)
	}
	if !options.Force {
		if err := safeToRemove(workspace.CheckoutPath); err != nil {
			return RemoveResult{}, err
		}
	}

	gitDir, commonDir, root, err := metadata(workspace.CheckoutPath)
	if err != nil {
		return RemoveResult{}, err
	}
	if filepath.Dir(gitDir) != filepath.Join(commonDir, "worktrees") {
		return RemoveResult{}, fmt.Errorf("refusing unexpected worktree metadata path %s", gitDir)
	}
	metadataPath, err := os.ReadFile(filepath.Join(gitDir, "gitdir"))
	if err != nil {
		return RemoveResult{}, fmt.Errorf("read worktree metadata: %w", err)
	}
	if strings.TrimSpace(string(metadataPath)) != filepath.Join(root, ".git") {
		return RemoveResult{}, fmt.Errorf("worktree metadata does not point to %s", root)
	}

	parent := filepath.Dir(workspace.CheckoutPath)
	trashRoot, err := os.MkdirTemp(parent, ".herdr-trash-"+filepath.Base(workspace.CheckoutPath)+".")
	if err != nil {
		return RemoveResult{}, fmt.Errorf("create worktree trash directory: %w", err)
	}
	trashPath := filepath.Join(trashRoot, filepath.Base(workspace.CheckoutPath))
	if err := os.Rename(workspace.CheckoutPath, trashPath); err != nil {
		_ = os.Remove(trashRoot)
		return RemoveResult{}, fmt.Errorf("move worktree aside: %w", err)
	}
	if _, err := client.Run("workspace", "close", options.WorkspaceID); err != nil {
		rollbackErr := os.Rename(trashPath, workspace.CheckoutPath)
		_ = os.Remove(trashRoot)
		return RemoveResult{}, errors.Join(err, rollbackErr)
	}
	if err := os.RemoveAll(gitDir); err != nil {
		restoreErr := os.Rename(trashPath, workspace.CheckoutPath)
		_ = os.Remove(trashRoot)
		return RemoveResult{}, errors.Join(fmt.Errorf("workspace closed but worktree metadata removal failed: %w", err), restoreErr)
	}
	if err := removeInBackground(trashRoot); err != nil {
		if removeErr := os.RemoveAll(trashRoot); removeErr != nil {
			return RemoveResult{}, errors.Join(err, removeErr)
		}
	}
	return RemoveResult{WorkspaceID: options.WorkspaceID, Path: workspace.CheckoutPath}, nil
}

func safeToRemove(path string) error {
	gitDir, _, _, err := metadata(path)
	if err != nil {
		return fmt.Errorf("inspect worktree metadata: %w", err)
	}
	if _, err := os.Stat(filepath.Join(gitDir, "locked")); err == nil {
		return fmt.Errorf("worktree is locked: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect worktree lock: %w", err)
	}
	status, err := gitOutput(path, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return err
	}
	if status != "" {
		return fmt.Errorf("worktree has uncommitted or untracked files: %s (use --force to delete)", path)
	}
	return nil
}

func metadata(path string) (string, string, string, error) {
	output, err := gitOutput(path, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir", "--show-toplevel")
	if err != nil {
		return "", "", "", err
	}
	lines := strings.Split(output, "\n")
	if len(lines) != 3 {
		return "", "", "", fmt.Errorf("git returned incomplete worktree metadata")
	}
	return lines[0], lines[1], lines[2], nil
}

func removeInBackground(path string) error {
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer null.Close()
	cmd := exec.Command("/bin/rm", "-rf", "--", path)
	cmd.Stdin = null
	cmd.Stdout = null
	cmd.Stderr = null
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start background cleanup: %w", err)
	}
	return cmd.Process.Release()
}
