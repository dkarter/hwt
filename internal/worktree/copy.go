package worktree

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dkarter/hwt/internal/config"
	"golang.org/x/sys/unix"
)

const (
	copyMarkerName = "hwt-copy-v1.done"
	copyLockName   = "hwt-copy-v1.lock"
)

type CopyOptions struct {
	CWD string
}

type CopyResult struct {
	Source          string   `json:"source"`
	Path            string   `json:"path"`
	Copied          []string `json:"copied,omitempty"`
	AlreadyPrepared bool     `json:"already_prepared"`
}

type copyMarker struct {
	Source string   `json:"source"`
	Copied []string `json:"copied,omitempty"`
}

func Copy(options CopyOptions) (CopyResult, error) {
	gitDir, commonDir, destination, err := metadata(options.CWD)
	if err != nil {
		return CopyResult{}, fmt.Errorf("resolve destination worktree: %w", err)
	}
	if samePath(gitDir, commonDir) {
		return CopyResult{}, errors.New("configured files can only be copied into a linked worktree")
	}
	return prepareFiles(destination, gitDir, func() (string, []string, error) {
		source, err := primaryWorktree(destination)
		if err != nil {
			return "", nil, err
		}
		cfg, _, err := config.Load(source, commonDir)
		if err != nil {
			return "", nil, err
		}
		copied, err := copyConfigured(source, destination, cfg.Files.Copy)
		return source, copied, err
	})
}

func prepareConfiguredFiles(source, destination string, cfg config.Config) (CopyResult, error) {
	gitDir, commonDir, destination, err := metadata(destination)
	if err != nil {
		return CopyResult{}, fmt.Errorf("resolve destination worktree: %w", err)
	}
	if samePath(gitDir, commonDir) {
		return CopyResult{}, errors.New("configured files can only be copied into a linked worktree")
	}
	return prepareFiles(destination, gitDir, func() (string, []string, error) {
		copied, err := copyConfigured(source, destination, cfg.Files.Copy)
		return source, copied, err
	})
}

func prepareFiles(destination, gitDir string, copyFiles func() (string, []string, error)) (CopyResult, error) {
	lock, err := os.OpenFile(filepath.Join(gitDir, copyLockName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return CopyResult{}, fmt.Errorf("open configured-file lock: %w", err)
	}
	defer lock.Close()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		return CopyResult{}, fmt.Errorf("lock configured files: %w", err)
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN) //nolint:errcheck

	marker := filepath.Join(gitDir, copyMarkerName)
	if data, err := os.ReadFile(marker); err == nil {
		var completed copyMarker
		if err := json.Unmarshal(data, &completed); err != nil {
			return CopyResult{}, fmt.Errorf("decode configured-file marker: %w", err)
		}
		return CopyResult{
			Source:          completed.Source,
			Path:            destination,
			Copied:          completed.Copied,
			AlreadyPrepared: true,
		}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return CopyResult{}, fmt.Errorf("read configured-file marker: %w", err)
	}

	source, copied, err := copyFiles()
	if err != nil {
		return CopyResult{}, err
	}
	result := CopyResult{Source: source, Path: destination, Copied: copied}
	if err := writeCopyMarker(gitDir, marker, copyMarker{Source: source, Copied: copied}); err != nil {
		return CopyResult{}, err
	}
	return result, nil
}

func writeCopyMarker(gitDir, marker string, completed copyMarker) error {
	temporary, err := os.CreateTemp(gitDir, "."+copyMarkerName+".*")
	if err != nil {
		return fmt.Errorf("create configured-file marker: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := json.NewEncoder(temporary).Encode(completed); err != nil {
		temporary.Close()
		return fmt.Errorf("write configured-file marker: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close configured-file marker: %w", err)
	}
	if err := os.Rename(temporaryPath, marker); err != nil {
		return fmt.Errorf("publish configured-file marker: %w", err)
	}
	return nil
}

func primaryWorktree(cwd string) (string, error) {
	output, err := gitOutput(cwd, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return "", fmt.Errorf("list Git worktrees: %w", err)
	}
	fields := strings.Split(output, "\x00")
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "worktree ") {
		return "", errors.New("Git did not report a primary worktree")
	}
	for _, field := range fields[1:] {
		if field == "" {
			break
		}
		if field == "bare" {
			return "", errors.New("cannot copy configured files from a bare primary repository")
		}
	}
	return strings.TrimPrefix(fields[0], "worktree "), nil
}

func samePath(left, right string) bool {
	canonical := func(path string) (string, error) {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		if evaluated, err := filepath.EvalSymlinks(absolute); err == nil {
			absolute = evaluated
		}
		return filepath.Clean(absolute), nil
	}
	leftPath, leftErr := canonical(left)
	rightPath, rightErr := canonical(right)
	return leftErr == nil && rightErr == nil && leftPath == rightPath
}
