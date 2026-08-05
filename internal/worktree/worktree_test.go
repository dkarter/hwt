package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dkarter/hwt/internal/config"
	"github.com/dkarter/hwt/internal/herdr"
)

type fakeClient struct {
	created   herdr.Created
	workspace herdr.Workspace
	runs      [][]string
}

func (f *fakeClient) Run(args ...string) ([]byte, error) {
	f.runs = append(f.runs, append([]string(nil), args...))
	return []byte(`{"result":{}}`), nil
}

func (f *fakeClient) SourceCheckout(cwd string) (string, error) {
	return cwd, nil
}

func (f *fakeClient) Create(_ ...string) (herdr.Created, error) {
	return f.created, nil
}

func (f *fakeClient) Workspace(_ string) (herdr.Workspace, error) {
	return f.workspace, nil
}

func (f *fakeClient) CurrentWorkspaceID() (string, error) {
	return f.workspace.ID, nil
}

func TestCreateCopiesFilesRunsHooksAndRecordsBase(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := initRepo(t)
	checkout := filepath.Join(t.TempDir(), "feature")
	run(t, repo, "git", "worktree", "add", "-b", "feature/test", checkout, "main")
	write(t, filepath.Join(repo, ".env.local"), "secret\n")
	write(t, filepath.Join(repo, "nested", "settings"), "value\n")
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), `
agent: opencode --port
files:
  copy: [.env.local, nested]
post_create:
  - touch hook-ran
`)
	client := &fakeClient{created: herdr.Created{WorkspaceID: "w2", PaneID: "w2:p1", Path: checkout}}

	result, err := Create(client, CreateOptions{CWD: repo, Branch: "feature/test", Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Agent != "opencode --port" || !reflect.DeepEqual(result.Copied, []string{".env.local", "nested"}) {
		t.Fatalf("unexpected result: %#v", result)
	}
	assertFile(t, filepath.Join(checkout, ".env.local"), "secret\n")
	assertFile(t, filepath.Join(checkout, "nested", "settings"), "value\n")
	if _, err := os.Stat(filepath.Join(checkout, "hook-ran")); err != nil {
		t.Fatalf("hook did not run: %v", err)
	}
	base := output(t, checkout, "git", "config", "--local", "--get", "branch.feature/test.herdr-base")
	if base != "main" {
		t.Fatalf("unexpected recorded base: %q", base)
	}
}

func TestWorktreePathUsesConfiguredNaming(t *testing.T) {
	cfg := config.Config{WorktreeDir: "../trees", WorktreeNaming: "basename", WorktreePrefix: "repo-"}
	path, err := worktreePath("/tmp/project", "", "feature/my-work", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/tmp/trees/repo-my-work" {
		t.Fatalf("unexpected worktree path: %s", path)
	}
}

func TestCopyConfiguredRejectsSymlinkedSourceParent(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	external := t.TempDir()
	write(t, filepath.Join(external, "secret"), "outside\n")
	if err := os.Symlink(external, filepath.Join(source, "link")); err != nil {
		t.Fatal(err)
	}

	_, err := copyConfigured(source, destination, []string{"link/secret"})
	if err == nil || !strings.Contains(err.Error(), "is a symlink") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func TestCopyConfiguredRejectsSymlinkDestination(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	external := filepath.Join(t.TempDir(), "external")
	write(t, filepath.Join(source, "secret"), "source\n")
	write(t, external, "outside\n")
	if err := os.Symlink(external, filepath.Join(destination, "secret")); err != nil {
		t.Fatal(err)
	}

	_, err := copyConfigured(source, destination, []string{"secret"})
	if err == nil || !strings.Contains(err.Error(), "destination is a symlink") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
	assertFile(t, external, "outside\n")
}

func TestRemoveRenamesCheckoutAndRemovesMetadata(t *testing.T) {
	repo := initRepo(t)
	checkout := filepath.Join(t.TempDir(), "remove-me")
	run(t, repo, "git", "worktree", "add", "-b", "remove-me", checkout, "main")
	gitDir := output(t, checkout, "git", "rev-parse", "--path-format=absolute", "--git-dir")
	client := &fakeClient{workspace: herdr.Workspace{
		ID:             "w9",
		CheckoutPath:   checkout,
		LinkedWorktree: true,
	}}

	result, err := Remove(client, RemoveOptions{WorkspaceID: "w9"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != checkout {
		t.Fatalf("unexpected removed path: %s", result.Path)
	}
	if _, err := os.Stat(checkout); !os.IsNotExist(err) {
		t.Fatalf("checkout still exists: %v", err)
	}
	if _, err := os.Stat(gitDir); !os.IsNotExist(err) {
		t.Fatalf("git metadata still exists: %v", err)
	}
	if len(client.runs) != 1 || strings.Join(client.runs[0], " ") != "workspace close w9" {
		t.Fatalf("unexpected Herdr calls: %#v", client.runs)
	}
}

func TestRemoveRejectsDirtyWorktree(t *testing.T) {
	repo := initRepo(t)
	checkout := filepath.Join(t.TempDir(), "dirty")
	run(t, repo, "git", "worktree", "add", "-b", "dirty", checkout, "main")
	write(t, filepath.Join(checkout, "dirty.txt"), "dirty\n")
	client := &fakeClient{workspace: herdr.Workspace{ID: "w3", CheckoutPath: checkout, LinkedWorktree: true}}

	_, err := Remove(client, RemoveOptions{WorkspaceID: "w3"})
	if err == nil || !strings.Contains(err.Error(), "uncommitted or untracked") {
		t.Fatalf("expected dirty worktree error, got %v", err)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	run(t, repo, "git", "init", "-b", "main")
	run(t, repo, "git", "config", "user.name", "Test")
	run(t, repo, "git", "config", "user.email", "test@example.com")
	write(t, filepath.Join(repo, "README.md"), "test\n")
	run(t, repo, "git", "add", "README.md")
	run(t, repo, "git", "commit", "-m", "initial")
	return repo
}

func run(t *testing.T, cwd, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = cwd
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %s: %v", name, strings.Join(args, " "), data, err)
	}
}

func output(t *testing.T, cwd, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = cwd
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %s: %v", name, strings.Join(args, " "), data, err)
	}
	return strings.TrimSpace(string(data))
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != expected {
		t.Fatalf("unexpected contents of %s: %q", path, data)
	}
}
