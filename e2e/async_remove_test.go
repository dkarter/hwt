package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWT018_AsyncRemoveHooksSurviveCLIExitAndPreserveFiles(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	herdr := s.fakeHerdr(repo)
	created := decode(t, s.run(repo, "--herdr-bin", herdr, "create", "async-remove", "--json"))
	path := created["path"].(string)
	gate := filepath.Join(s.root, "gate")
	pre := filepath.Join(s.root, "pre")
	post := filepath.Join(s.root, "post")
	// The gate proves the CLI does not wait, without timing-dependent sleeps.
	var logs []any
	t.Cleanup(func() {
		_ = os.WriteFile(gate, nil, 0o600)
		for _, log := range logs {
			marker := strings.TrimSuffix(log.(string), ".log")
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, err := os.Stat(marker); os.IsNotExist(err) {
					break
				}
				if time.Now().After(deadline) {
					t.Errorf("hook did not finish during cleanup: %s", marker)
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
		}
	})
	mustWrite(t, filepath.Join(path, "keep"), "retained", 0o600)
	mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), fmt.Sprintf(`pre-remove:
  - cmd: 'while [ ! -f %s ]; do /bin/sleep 0.05; done; /bin/cat keep > %s; exit 17'
    async: true
post_remove:
  - cmd: 'while [ ! -f %s ]; do /bin/sleep 0.05; done; test ! -e "$HWT_WORKTREE_PATH" && echo post > %s'
    async: true
`, gate, pre, gate, post), 0o600)
	result := decode(t, s.run(repo, "--herdr-bin", herdr, "remove", "--workspace", "ws1", "--force", "--json"))
	logs = result["async_logs"].([]any)
	if len(logs) != 2 {
		t.Fatalf("async logs = %#v", result)
	}
	if _, err := os.Stat(pre); !os.IsNotExist(err) {
		t.Fatal("async hook finished before gate opened")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("checkout was not moved aside: %v", err)
	}
	trash, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".herdr-trash-"+filepath.Base(path)+".*"))
	if err != nil || len(trash) != 1 {
		t.Fatalf("retained trash = %v, %v", trash, err)
	}
	mustWrite(t, gate, "go", 0o600)
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, preErr := os.Stat(pre)
		_, postErr := os.Stat(post)
		_, trashErr := os.Stat(trash[0])
		if preErr == nil && postErr == nil && os.IsNotExist(trashErr) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("async removal did not finish: pre=%v post=%v trash=%v", preErr, postErr, trashErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if mustRead(t, pre) != "retained" || mustRead(t, post) != "post\n" {
		t.Fatal("async hooks lost files or environment")
	}
	requireContains(t, mustRead(t, logs[0].(string)), "hook failed with exit status 17")
}

func TestWT018_RemoveFailureReportsAlreadyStartedHookLogs(t *testing.T) {
	for _, name := range []string{"pre_remove", "post_remove"} {
		t.Run(name, func(t *testing.T) {
			s := newSandbox(t)
			repo := s.repo()
			herdr := s.fakeHerdr(repo)
			s.run(repo, "--herdr-bin", herdr, "create", "failed-hook", "--json")
			gate := filepath.Join(s.root, "gate")
			var marker string
			t.Cleanup(func() {
				_ = os.WriteFile(gate, nil, 0o600)
				deadline := time.Now().Add(5 * time.Second)
				for marker != "" {
					if _, err := os.Stat(marker); os.IsNotExist(err) {
						break
					}
					if time.Now().After(deadline) {
						t.Errorf("hook did not finish: %s", marker)
						break
					}
					time.Sleep(20 * time.Millisecond)
				}
			})
			mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), fmt.Sprintf("%s:\n  - cmd: 'while [ ! -f %s ]; do /bin/sleep 0.05; done; echo done'\n    async: true\n  - exit 17\n", name, gate), 0o600)
			stdout, stderr, err := s.command(repo, "--herdr-bin", herdr, "remove", "--workspace", "ws1", "--force", "--json")
			if err == nil {
				t.Fatal("synchronous hook failure was ignored")
			}
			requireContains(t, stderr, name+" command")
			result := decode(t, stdout)
			logs := result["async_logs"].([]any)
			if len(logs) != 1 || result["workspace_id"] != "ws1" {
				t.Fatalf("partial result lost async logs: %#v", result)
			}
			marker = strings.TrimSuffix(logs[0].(string), ".log")
			mustWrite(t, gate, "go", 0o600)
		})
	}
}
