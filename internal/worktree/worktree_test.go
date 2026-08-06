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

	_, err := copyConfigured(source, destination, copyEntries("link/secret"))
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

	_, err := copyConfigured(source, destination, copyEntries("secret"))
	if err == nil || !strings.Contains(err.Error(), "destination is a symlink") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
	assertFile(t, external, "outside\n")
}

func TestCopyConfiguredRunsCopiesInParallelAndWaits(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	write(t, filepath.Join(source, "first"), "first\n")
	write(t, filepath.Join(source, "second"), "second\n")
	started := make(chan string, 2)
	release := make(chan struct{})
	completed := make(chan struct{}, 2)
	type result struct {
		copied []string
		err    error
	}
	resultChannel := make(chan result, 1)

	go func() {
		copied, err := copyConfiguredWith(source, destination, copyEntries("first", "second"), func(_ config.CopyEntry, source, _ string) error {
			started <- filepath.Base(source)
			<-release
			completed <- struct{}{}
			return nil
		})
		resultChannel <- result{copied: copied, err: err}
	}()

	<-started
	<-started
	close(release)
	resultValue := <-resultChannel
	if resultValue.err != nil {
		t.Fatal(resultValue.err)
	}
	if !reflect.DeepEqual(resultValue.copied, []string{"first", "second"}) {
		t.Fatalf("unexpected copied files: %#v", resultValue.copied)
	}
	if len(completed) != 2 {
		t.Fatalf("returned before every copy completed: completed=%d", len(completed))
	}
}

func TestCopyConfiguredWaitsForParallelCopiesAfterError(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	write(t, filepath.Join(source, "slow"), "slow\n")
	write(t, filepath.Join(source, "failed"), "failed\n")
	started := make(chan string, 2)
	release := make(chan struct{})
	resultChannel := make(chan error, 1)

	go func() {
		_, err := copyConfiguredWith(source, destination, copyEntries("slow", "failed"), func(entry config.CopyEntry, _, _ string) error {
			started <- entry.Path
			if entry.Path == "failed" {
				return os.ErrPermission
			}
			<-release
			return nil
		})
		resultChannel <- err
	}()

	<-started
	<-started
	select {
	case err := <-resultChannel:
		t.Fatalf("returned before slow copy finished: %v", err)
	default:
	}
	close(release)
	if err := <-resultChannel; err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected copy error after all copies finished, got %v", err)
	}
}

func TestCopyConfiguredCanRunSequentially(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	write(t, filepath.Join(source, "first"), "first\n")
	write(t, filepath.Join(source, "second"), "second\n")
	var order []string

	entries := []config.CopyEntry{{Path: "first"}, {Path: "second"}}
	_, err := copyConfiguredWith(source, destination, entries, func(_ config.CopyEntry, source, _ string) error {
		order = append(order, filepath.Base(source))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"first", "second"}) {
		t.Fatalf("copies did not run sequentially: %#v", order)
	}
}

func TestCopyConfiguredTreatsSequentialEntryAsBarrier(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	for _, name := range []string{"first", "barrier", "last"} {
		write(t, filepath.Join(source, name), name+"\n")
	}
	entries := []config.CopyEntry{
		{Path: "first", Parallel: true},
		{Path: "barrier"},
		{Path: "last", Parallel: true},
	}
	var order []string

	_, err := copyConfiguredWith(source, destination, entries, func(_ config.CopyEntry, source, _ string) error {
		order = append(order, filepath.Base(source))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"first", "barrier", "last"}) {
		t.Fatalf("unexpected barrier order: %#v", order)
	}
}

func TestCopyConfiguredSerializesOverlappingPaths(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	write(t, filepath.Join(source, "dir", "file"), "value\n")
	entries := copyEntries("dir", "dir/file")
	var order []string

	_, err := copyConfiguredWith(source, destination, entries, func(entry config.CopyEntry, _, _ string) error {
		order = append(order, entry.Path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"dir", "dir/file"}) {
		t.Fatalf("overlapping copies ran out of order: %#v", order)
	}
}

func TestCopyPathConfiguredFallsBackWhenCloneIsUnavailable(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	destination := filepath.Join(t.TempDir(), "destination")
	write(t, filepath.Join(source, "file"), "source\n")
	write(t, filepath.Join(destination, "existing"), "existing\n")

	if err := copyPathConfigured(config.CopyEntry{CopyOnWrite: true}, source, destination); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(destination, "file"), "source\n")
	assertFile(t, filepath.Join(destination, "existing"), "existing\n")
}

func TestCopyPathConfiguredPreservesSourceSymlinkWithCopyOnWrite(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(t.TempDir(), "destination")
	write(t, filepath.Join(root, "target"), "target\n")
	if err := os.Symlink("target", source); err != nil {
		t.Fatal(err)
	}

	if err := copyPathConfigured(config.CopyEntry{CopyOnWrite: true}, source, destination); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(destination)
	if err != nil {
		t.Fatal(err)
	}
	if target != "target" {
		t.Fatalf("unexpected symlink target: %s", target)
	}
}

func TestCopyPathConfiguredCanSymlink(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	destination := filepath.Join(t.TempDir(), "nested", "destination")
	write(t, filepath.Join(source, "file"), "source\n")

	if err := copyPathConfigured(config.CopyEntry{Symlink: true}, source, destination); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(destination)
	if err != nil {
		t.Fatal(err)
	}
	if target != source {
		t.Fatalf("unexpected symlink target: %s", target)
	}
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
	if name == "git" {
		cmd.Env = gitEnvironment()
	}
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %s: %v", name, strings.Join(args, " "), data, err)
	}
}

func output(t *testing.T, cwd, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = cwd
	if name == "git" {
		cmd.Env = gitEnvironment()
	}
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

func copyEntries(paths ...string) []config.CopyEntry {
	entries := make([]config.CopyEntry, len(paths))
	for index, path := range paths {
		entries[index] = config.CopyEntry{Path: path, Parallel: true}
	}
	return entries
}
