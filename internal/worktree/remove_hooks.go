package worktree

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/dkarter/hwt/internal/config"
)

// runRemoveHooks starts async entries in order, but only waits for synchronous
// entries. Completion markers let deletion retain files used by pre-remove jobs.
func runRemoveHooks(name, cwd string, hooks []config.RemoveHook, environment map[string]string) (pending, logs []string, err error) {
	if name == "pre_remove" {
		pending, logs, err = pendingRemoveHooks(name, cwd)
		if err != nil {
			return pending, logs, err
		}
	}
	for _, hook := range hooks {
		if !hook.Async {
			if err := runHooks(name, cwd, []string{hook.Command}, environment); err != nil {
				return pending, logs, err
			}
			continue
		}
		marker, log, err := startRemoveHook(name, cwd, hook.Command, environment)
		if err != nil {
			return pending, logs, err
		}
		pending = append(pending, marker)
		logs = append(logs, log)
	}
	return pending, logs, nil
}

func removeHookDirectory() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "hwt", "remove-hooks"), nil
}

func removeHookPrefix(name, cwd string) string {
	return fmt.Sprintf("pending-%s-%x-", name, sha256.Sum256([]byte(filepath.Clean(cwd))))
}

// Retain job identity across failed removals, retries, and config changes.
func pendingRemoveHooks(name, cwd string) (pending, logs []string, err error) {
	dir, err := removeHookDirectory()
	if err != nil {
		return nil, nil, err
	}
	matches, err := filepath.Glob(filepath.Join(dir, removeHookPrefix(name, cwd)+"*.pending"))
	if err != nil {
		return nil, nil, err
	}
	for _, path := range matches {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return pending, logs, err
		}
		if info.IsDir() {
			pending = append(pending, path)
			logs = append(logs, path+".log")
		}
	}
	return pending, logs, nil
}

func startRemoveHook(name, cwd, command string, environment map[string]string) (string, string, error) {
	dir, err := removeHookDirectory()
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	marker, err := os.MkdirTemp(dir, removeHookPrefix(name, cwd)+"*.pending")
	if err != nil {
		return "", "", err
	}
	started := false
	defer func() {
		if !started {
			_ = os.Remove(marker)
			_ = os.Remove(marker + ".log")
		}
	}()
	log, err := os.OpenFile(marker+".log", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", "", err
	}
	defer log.Close()
	null, err := os.Open(os.DevNull)
	if err != nil {
		return "", "", err
	}
	defer null.Close()
	// No terminal descriptors are inherited. A new session lets the hook outlive
	// both the HWT command and closure of the removed Herdr workspace.
	cmd := exec.Command("/bin/sh", "-c", `trap '/bin/rmdir "$2"' EXIT
/bin/sh -lc "$1"
status=$?
if [ "$status" -ne 0 ]; then printf 'hook failed with exit status %s\n' "$status" >&2; fi
exit "$status"`, "hwt-"+name, command, marker)
	cmd.Dir = cwd
	cmd.Env = environmentList(environment)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = null, log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return "", "", fmt.Errorf("start async %s command %q: %w", name, command, err)
	}
	started = true
	// Reap while this process is alive; after CLI exit the detached hook is
	// adopted by the OS and still removes its completion marker.
	go func() { _ = cmd.Wait() }()
	return marker, log.Name(), nil
}
