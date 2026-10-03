package worktree

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/dkarter/hwt/internal/config"
)

var (
	errLocked = errors.New("worktree is locked")
	errDirty  = errors.New("worktree has uncommitted or untracked files")
)

func CanForceRemove(err error) bool {
	return errors.Is(err, errLocked) || errors.Is(err, errDirty)
}

func IsLocked(err error) bool {
	return errors.Is(err, errLocked)
}

type RemoveOptions struct {
	WorkspaceID string
	Force       bool
}

type RemoveResult struct {
	WorkspaceID string   `json:"workspace_id"`
	Path        string   `json:"path"`
	AsyncLogs   []string `json:"async_logs,omitempty"`
}

func Remove(client Client, options RemoveOptions) (result RemoveResult, err error) {
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
	var logs []string
	defer func() {
		if len(logs) > 0 {
			result.WorkspaceID = options.WorkspaceID
			result.Path = workspace.CheckoutPath
			result.AsyncLogs = logs
		}
	}()
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
	allocationPath, err := canonicalWorktreePath(workspace.CheckoutPath)
	if err != nil {
		return RemoveResult{}, fmt.Errorf("resolve port allocation path: %w", err)
	}
	source, err := primaryWorktree(root)
	cfg := config.Config{LocalDNS: config.LocalDNS{Domain: config.DefaultLocalDNSDomain}}
	if err == nil {
		loaded, _, loadErr := config.Load(source, commonDir)
		if loadErr != nil {
			return RemoveResult{}, loadErr
		}
		cfg = loaded
	}
	hookEnvironment := map[string]string(nil)
	if len(cfg.PreRemove) > 0 || len(cfg.PostRemove) > 0 {
		environment, environmentErr := prepareEnvironment(root, cfg, false)
		if environmentErr != nil {
			return RemoveResult{}, fmt.Errorf("prepare remove hook environment: %w", environmentErr)
		}
		hookEnvironment = environment.Variables
	}
	pending, preLogs, err := runRemoveHooks("pre_remove", root, cfg.PreRemove, hookEnvironment)
	logs = append(logs, preLogs...)
	if err != nil {
		return RemoveResult{}, err
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
	cleanupErr := removeInBackground(trashRoot, pending...)
	if cleanupErr != nil {
		if len(pending) > 0 {
			cleanupErr = fmt.Errorf("worktree moved to %s but background cleanup failed: %w", trashPath, cleanupErr)
		} else if removeErr := os.RemoveAll(trashRoot); removeErr != nil {
			cleanupErr = errors.Join(cleanupErr, removeErr)
		} else {
			cleanupErr = nil
		}
	}
	if err := releaseLocalDNS(allocationPath, cfg); err != nil {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("worktree removed but local DNS cleanup failed: %w", err))
	}
	if err := releasePorts(allocationPath); err != nil {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("worktree removed but port allocation cleanup failed: %w", err))
	}
	if cleanupErr != nil {
		return RemoveResult{}, cleanupErr
	}
	_, postLogs, err := runRemoveHooks("post_remove", source, cfg.PostRemove, hookEnvironment)
	logs = append(logs, postLogs...)
	if err != nil {
		return RemoveResult{}, fmt.Errorf("worktree removed but %w", err)
	}
	return RemoveResult{WorkspaceID: options.WorkspaceID, Path: workspace.CheckoutPath, AsyncLogs: logs}, nil
}

func safeToRemove(path string) error {
	gitDir, _, _, err := metadata(path)
	if err != nil {
		return fmt.Errorf("inspect worktree metadata: %w", err)
	}
	if _, err := os.Stat(filepath.Join(gitDir, "locked")); err == nil {
		return fmt.Errorf("%w: %s", errLocked, path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect worktree lock: %w", err)
	}
	status, err := gitOutput(path, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return err
	}
	if status != "" {
		return fmt.Errorf("%w: %s (use --force to delete)", errDirty, path)
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

func removeInBackground(path string, pending ...string) error {
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer null.Close()
	args := []string{"-c", `path=$1
shift
for marker in "$@"; do
  while [ -d "$marker" ]; do /bin/sleep 0.1; done
done
exec /bin/rm -rf -- "$path"`, "hwt-cleanup", path}
	args = append(args, pending...)
	cmd := exec.Command("/bin/sh", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = null
	cmd.Stdout = null
	cmd.Stderr = null
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start background cleanup: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
