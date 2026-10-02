package worktree

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dkarter/hwt/internal/herdr"
)

func TestCreateReuseRejectsUnsafeCandidates(t *testing.T) {
	for _, test := range []struct{ name, message string }{
		{"no opt in", "--reuse or --focus"},
		{"primary", "primary checkout"},
		{"ambiguous", "multiple recorded worktrees"},
		{"detached", "invalid recorded checkout"},
		{"wrong branch", "not a linked worktree"},
		{"wrong repository", "not a linked worktree"},
		{"invalid directory", "verify existing worktree"},
		{"path conflict", "--path"},
		{"workspace mismatch", "not associated"},
		{"no panes", "has no panes"},
		{"prunable existing", "stale worktree"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			repo := initRepo(t)
			path := filepath.Join(t.TempDir(), "checkout")
			run(t, repo, "git", "worktree", "add", "-b", "reuse", path, "main")
			item := herdr.Worktree{Branch: "reuse", Path: path, Linked: true, OpenWorkspaceID: "w2"}
			client := &fakeClient{workspace: herdr.Workspace{ID: "w2", CheckoutPath: path, LinkedWorktree: true}, panes: []herdr.Pane{{ID: "w2:p1", WorkspaceID: "w2"}}}
			options := CreateOptions{CWD: repo, Branch: "reuse", Reuse: true}
			switch test.name {
			case "no opt in":
				options.Reuse = false
			case "primary":
				item.Path, item.Linked = repo, false
			case "detached":
				item.Detached = true
			case "wrong branch":
				run(t, path, "git", "checkout", "--detach")
			case "wrong repository":
				item.Path = initRepo(t)
			case "invalid directory":
				item.Path = t.TempDir()
			case "path conflict":
				options.Path = "../elsewhere"
			case "workspace mismatch":
				client.workspace.CheckoutPath = repo
			case "no panes":
				client.panes = nil
			case "prunable existing":
				item.Prunable = true
			}
			client.worktrees = []herdr.Worktree{item}
			if test.name == "ambiguous" {
				client.worktrees = append(client.worktrees, item)
			}
			_, err := Create(client, options)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %q, got %v", test.message, err)
			}
			if len(client.creates) != 0 || len(client.opens) != 0 || len(client.runs) != 0 {
				t.Fatalf("rejection mutated Herdr: %#v", client)
			}
			var stale *StaleWorktreeError
			if errors.As(err, &stale) && len(stale.Repair) != 0 {
				t.Fatal("existing path got destructive repair guidance")
			}
		})
	}
}

func TestCreateLockedMissingRecordDoesNotRecommendRemoval(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := initRepo(t)
	path := filepath.Join(t.TempDir(), "missing")
	run(t, repo, "git", "worktree", "add", "-b", "locked", path, "main")
	run(t, repo, "git", "worktree", "lock", path)
	if err := os.Rename(path, path+"-moved"); err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{worktrees: []herdr.Worktree{{Branch: "locked", Path: path, Linked: true}}}
	_, err := Create(client, CreateOptions{CWD: repo, Branch: "locked", Focus: true})
	var stale *StaleWorktreeError
	if !errors.As(err, &stale) || len(stale.Repair) != 0 {
		t.Fatalf("unsafe locked guidance: %v", err)
	}
}

func TestCreatePartialOpenFailureDoesNotRollback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := initRepo(t)
	path := filepath.Join(t.TempDir(), "checkout")
	run(t, repo, "git", "worktree", "add", "-b", "reuse", path, "main")
	write(t, filepath.Join(path, "dirty.txt"), "keep me")
	client := &fakeClient{
		worktrees: []herdr.Worktree{{Branch: "reuse", Path: path, Linked: true}},
		created:   herdr.Created{WorkspaceID: "w2", Path: path},
		openErr:   errors.New("incomplete open response"),
	}
	_, err := Create(client, CreateOptions{CWD: repo, Branch: "reuse", Reuse: true})
	if err == nil || len(client.runs) != 0 || len(client.creates) != 0 {
		t.Fatalf("partial open failure invoked rollback: %v, %#v", err, client)
	}
	assertFile(t, filepath.Join(path, "dirty.txt"), "keep me")
}

func TestCreateReuseInspectionPermissionError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := initRepo(t)
	parent := t.TempDir()
	path := filepath.Join(parent, "checkout")
	run(t, repo, "git", "worktree", "add", "-b", "reuse", path, "main")
	if err := os.Chmod(parent, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrPermission) {
		t.Skip("filesystem does not enforce permission denial")
	}
	client := &fakeClient{worktrees: []herdr.Worktree{{Branch: "reuse", Path: path, Linked: true}}}
	_, err := Create(client, CreateOptions{CWD: repo, Branch: "reuse", Focus: true})
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("permission error not preserved: %v", err)
	}
	var stale *StaleWorktreeError
	if errors.As(err, &stale) {
		t.Fatal("permission failure misclassified as missing")
	}
}
