package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dkarter/hwt/internal/config"
	"github.com/dkarter/hwt/internal/herdr"
)

func TestAsyncRemoveHookRetainsFilesUntilCompletionAndLogsFailures(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cwd := t.TempDir()
	trash := filepath.Join(t.TempDir(), "moved-checkout")
	gate := filepath.Join(t.TempDir(), "gate")
	output := filepath.Join(t.TempDir(), "output")
	write(t, filepath.Join(cwd, "keep"), "retained")
	pending, logs, err := runRemoveHooks("pre_remove", cwd, []config.RemoveHook{{Command: `while [ ! -f "$GATE" ]; do sleep 0.05; done; cat keep > "$OUTPUT"; exit 17`, Async: true}}, map[string]string{"GATE": gate, "OUTPUT": output})
	defer func() {
		_ = os.WriteFile(gate, nil, 0o600)
		for _, marker := range pending {
			waitFor(t, func() bool { _, err := os.Stat(marker); return os.IsNotExist(err) })
		}
	}()
	if err != nil || len(pending) != 1 || len(logs) != 1 {
		t.Fatalf("start: %v %v %v", pending, logs, err)
	}
	if err := os.Rename(cwd, trash); err != nil {
		t.Fatal(err)
	}
	if err := removeInBackground(trash, pending...); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(trash, "keep")); err != nil {
		t.Fatalf("deleted files before async hook completed: %v", err)
	}
	write(t, gate, "go")
	waitFor(t, func() bool { _, err := os.Stat(trash); return os.IsNotExist(err) })
	assertFile(t, output, "retained")
	log, err := os.ReadFile(logs[0])
	if err != nil || !strings.Contains(string(log), "hook failed with exit status 17") {
		t.Fatalf("failure log = %s, %v", log, err)
	}
}

func TestRemoveRetryWaitsForAsyncPreHooksFromFailedAttempt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	repo := initRepo(t)
	checkout := filepath.Join(t.TempDir(), "retry-remove")
	run(t, repo, "git", "worktree", "add", "-b", "retry-remove", checkout, "main")
	gate := filepath.Join(t.TempDir(), "gate")
	output := filepath.Join(t.TempDir(), "output")
	write(t, filepath.Join(checkout, "keep"), "retained")
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), "pre_remove:\n  - cmd: 'while [ ! -f "+gate+" ]; do sleep 0.05; done; cat keep > "+output+"'\n    async: true\n  - exit 17\n")
	client := &fakeClient{workspace: herdr.Workspace{ID: "w-retry", CheckoutPath: checkout, LinkedWorktree: true}}
	first, err := Remove(client, RemoveOptions{WorkspaceID: "w-retry", Force: true})
	defer func() {
		_ = os.WriteFile(gate, nil, 0o600)
		pending, _, _ := pendingRemoveHooks("pre_remove", checkout)
		for _, marker := range pending {
			waitFor(t, func() bool { _, err := os.Stat(marker); return os.IsNotExist(err) })
		}
	}()
	if err == nil || len(first.AsyncLogs) != 1 {
		t.Fatalf("failed removal lost logs: %#v, %v", first, err)
	}
	if _, err := os.Stat(checkout); err != nil {
		t.Fatal("blocking failure removed the checkout")
	}
	// The retry must discover the first attempt's job even after a config change.
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), "pre_remove: []\n")
	second, err := Remove(client, RemoveOptions{WorkspaceID: "w-retry", Force: true})
	if err != nil || len(second.AsyncLogs) != 1 || second.AsyncLogs[0] != first.AsyncLogs[0] {
		t.Fatalf("retry lost existing job: %#v, %v", second, err)
	}
	trash, err := filepath.Glob(filepath.Join(filepath.Dir(checkout), ".herdr-trash-"+filepath.Base(checkout)+".*"))
	if err != nil || len(trash) != 1 {
		t.Fatalf("files not retained: %v, %v", trash, err)
	}
	write(t, gate, "go")
	waitFor(t, func() bool { _, err := os.Stat(trash[0]); return os.IsNotExist(err) })
	assertFile(t, output, "retained")
}

func TestRemoveHookObjectsAreBlockingByDefault(t *testing.T) {
	cwd := t.TempDir()
	pending, logs, err := runRemoveHooks("pre_remove", cwd, []config.RemoveHook{{Command: "echo done > marker"}}, nil)
	if err != nil || len(pending) != 0 || len(logs) != 0 {
		t.Fatalf("unexpected async result: %v %v %v", pending, logs, err)
	}
	assertFile(t, filepath.Join(cwd, "marker"), "done\n")
	if _, _, err := runRemoveHooks("pre_remove", cwd, []config.RemoveHook{{Command: "exit 17"}}, nil); err == nil {
		t.Fatal("blocking failure was ignored")
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for background hook")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
