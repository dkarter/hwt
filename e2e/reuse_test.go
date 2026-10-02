package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func (s *sandbox) worktreeList(repo string, items ...map[string]any) {
	s.t.Helper()
	data, err := json.Marshal(map[string]any{"result": map[string]any{"source": map[string]string{"source_checkout_path": repo}, "worktrees": items}})
	if err != nil {
		s.t.Fatal(err)
	}
	path := filepath.Join(s.root, "worktree-list.json")
	mustWrite(s.t, path, string(data)+"\n", 0o600)
	s.env = append(s.env, "HWT_FAKE_LIST="+path)
}

func TestWT014_WT015_CreateReusesWithoutRepeatingPreparation(t *testing.T) {
	for _, mode := range []string{"focus", "reuse", "closed", "detached source", "changed port range", "removed services"} {
		t.Run(mode, func(t *testing.T) {
			s := newSandbox(t)
			repo := s.repo()
			herdr := s.fakeHerdr(repo)
			mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), "files:\n  copy: [local.txt]\nports:\n  start: 32700\n  end: 32710\n  services: [web]\npost_create:\n  - 'printf once >> hook.txt'\n", 0o600)
			mustWrite(t, filepath.Join(repo, "local.txt"), "source", 0o600)
			first := decode(t, s.run(repo, "--herdr-bin", herdr, "create", "--branch", "feature/reuse", "--json"))
			if first["reused_worktree"] != false || first["reused_workspace"] != false {
				t.Fatalf("fresh flags: %#v", first)
			}
			path := first["path"].(string)
			// Reuse must not republish an identical environment and wake watchers.
			envPath := filepath.Join(path, ".env.worktree")
			envBefore, err := os.Stat(envPath)
			if err != nil {
				t.Fatal(err)
			}
			mustWrite(t, filepath.Join(path, "local.txt"), "user changes", 0o600)
			mustWrite(t, filepath.Join(path, "README"), "dirty tracked file", 0o600)
			gitDir := s.git(path, "rev-parse", "--git-dir")
			mustWrite(t, filepath.Join(gitDir, "hwt-ticket-metadata-v1.json"), "{\"identifier\":\"TEST-36\"}\n", 0o600)
			// A changed base/placement must not be applied to an existing checkout.
			mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), "worktree_dir: elsewhere\nfiles:\n  copy: [local.txt]\nports:\n  start: 32700\n  end: 32710\n  services: [web]\npost_create:\n  - 'exit 99'\n", 0o600)
			if mode == "changed port range" {
				mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), "ports:\n  start: 32900\n  end: 32910\n  services: [web]\npost_create:\n  - 'exit 99'\n", 0o600)
			}
			if mode == "removed services" {
				mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), "ports:\n  services: []\npost_create:\n  - 'exit 99'\n", 0o600)
			}
			item := map[string]any{"branch": "feature/reuse", "path": path, "is_linked_worktree": true, "open_workspace_id": "ws1"}
			flag := "--reuse"
			if mode == "focus" {
				flag = "--focus"
			}
			if mode == "closed" {
				delete(item, "open_workspace_id")
			}
			s.worktreeList(repo, item)
			mustWrite(t, filepath.Join(s.root, "herdr.log"), "", 0o600)
			args := []string{"--herdr-bin", herdr, "create", "--branch", "feature/reuse", flag, "--json"}
			if mode == "detached source" {
				s.git(repo, "checkout", "--detach")
			} else {
				args = append(args, "--base", "unused-base")
			}
			result := decode(t, s.run(repo, args...))
			if result["reused_worktree"] != true || result["reused_workspace"] != (mode != "closed") || result["path"] != path || result["base"] != "main" {
				t.Fatalf("reuse result: %#v", result)
			}
			if !reflect.DeepEqual(result["environment"], first["environment"]) {
				t.Fatal("reuse changed port allocation")
			}
			envAfter, err := os.Stat(envPath)
			if err != nil || !os.SameFile(envBefore, envAfter) {
				t.Fatalf("reuse republished unchanged environment: %v", err)
			}
			for file, want := range map[string]string{"local.txt": "user changes", "README": "dirty tracked file", "hook.txt": "once"} {
				if got := mustRead(t, filepath.Join(path, file)); got != want {
					t.Fatalf("%s changed: %q", file, got)
				}
			}
			if got := mustRead(t, filepath.Join(gitDir, "hwt-ticket-metadata-v1.json")); got != "{\"identifier\":\"TEST-36\"}\n" {
				t.Fatal("ticket metadata changed")
			}
			log := mustRead(t, filepath.Join(s.root, "herdr.log"))
			if strings.Contains(log, "worktree create") || strings.Contains(log, "worktree remove") || strings.Contains(log, "workspace focus") != (mode == "focus") || strings.Contains(log, "worktree open") != (mode == "closed") {
				t.Fatalf("unexpected reuse calls: %s", log)
			}
		})
	}
}

func TestWT016_StaleRecordHasTargetedRepair(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	herdr := s.fakeHerdr(repo)
	missing := filepath.Join(s.root, "missing checkout's path")
	s.git(repo, "worktree", "add", "-b", "stale", missing, "main")
	missing = s.git(missing, "rev-parse", "--show-toplevel")
	// Move the test checkout away, retaining its Git record without deleting files.
	if err := os.Rename(missing, missing+"-moved"); err != nil {
		t.Fatal(err)
	}
	other := s.linked(repo, "unrelated-stale")
	other = s.git(other, "rev-parse", "--show-toplevel")
	if err := os.Rename(other, other+"-moved"); err != nil {
		t.Fatal(err)
	}
	live := s.linked(repo, "unrelated-live")
	live = s.git(live, "rev-parse", "--show-toplevel")
	s.worktreeList(repo, map[string]any{"branch": "stale", "path": missing, "is_linked_worktree": true, "is_prunable": true})
	before := s.git(repo, "worktree", "list", "--porcelain")
	stdout, stderr, err := s.command(repo, "--herdr-bin", herdr, "create", "--branch", "stale", "--base", "main", "--focus", "--json")
	if err == nil {
		t.Fatal("stale record succeeded")
	}
	result := decode(t, stdout)
	if result["error"] != "stale_worktree" || result["branch"] != "stale" || result["path"] != missing {
		t.Fatalf("stale diagnostic: %#v", result)
	}
	requireContains(t, stderr, "stale worktree")
	if s.git(repo, "worktree", "list", "--porcelain") != before {
		t.Fatal("stale diagnostic mutated records")
	}
	argv := result["repair"].([]any)
	var args []string
	for _, arg := range argv[1:] {
		args = append(args, arg.(string))
	}
	cmd := exec.Command(gitBinary, args...)
	cmd.Env = s.env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("repair failed: %s: %v", out, err)
	}
	after := s.git(repo, "worktree", "list", "--porcelain")
	if strings.Contains(after, "worktree "+missing+"\n") || !strings.Contains(after, "worktree "+other+"\n") || !strings.Contains(after, "worktree "+live+"\n") {
		t.Fatalf("repair affected wrong records: %s", after)
	}
	s.worktreeList(repo)
	fresh := decode(t, s.run(repo, "--herdr-bin", herdr, "create", "--branch", "stale", "--base", "main", "--path", missing, "--focus", "--json"))
	if fresh["reused_worktree"] != false {
		t.Fatalf("retry not fresh: %#v", fresh)
	}
}

func TestWT017_ReuseFailuresNeverRemoveCheckout(t *testing.T) {
	for _, failure := range []string{"worktree list", "pane list", "worktree open", "workspace focus", "environment"} {
		t.Run(failure, func(t *testing.T) {
			s := newSandbox(t)
			repo := s.repo()
			herdr := s.fakeHerdr(repo)
			path := s.linked(repo, "reuse")
			mustWrite(t, filepath.Join(path, "dirty.txt"), "keep me", 0o600)
			mustWrite(t, filepath.Join(s.root, "herdr-worktree"), path+"\n", 0o600)
			item := map[string]any{"branch": "reuse", "path": path, "is_linked_worktree": true, "open_workspace_id": "ws1"}
			if failure == "worktree open" {
				delete(item, "open_workspace_id")
			}
			s.worktreeList(repo, item)
			if failure == "environment" {
				mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), "environment:\n  variables:\n    BAD: ${UNKNOWN}\n", 0o600)
			} else {
				s.env = append(s.env, "HWT_FAKE_FAIL="+failure)
			}
			_, _, err := s.command(repo, "--herdr-bin", herdr, "create", "--branch", "reuse", "--focus", "--json")
			if err == nil {
				t.Fatal("injected failure succeeded")
			}
			if mustRead(t, filepath.Join(path, "dirty.txt")) != "keep me" {
				t.Fatal("dirty data lost")
			}
			log := mustRead(t, filepath.Join(s.root, "herdr.log"))
			if strings.Contains(log, "worktree remove") || strings.Contains(log, "worktree create") {
				t.Fatalf("destructive reuse failure: %s", log)
			}
		})
	}
}
