package worktree

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dkarter/hwt/internal/config"
	"github.com/dkarter/hwt/internal/herdr"
	"github.com/dkarter/hwt/internal/localdns"
)

func TestCopyCopiesFromPrimaryWorktreeOnlyOnce(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := initRepo(t)
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), "files:\n  copy: [.env.local]\n")
	run(t, repo, "git", "add", "-f", ".herdr-worktree.yaml")
	run(t, repo, "git", "commit", "-m", "add worktree config")
	write(t, filepath.Join(repo, ".env.local"), "primary\n")
	checkout := filepath.Join(t.TempDir(), "feature")
	run(t, repo, "git", "worktree", "add", "-b", "copy-once", checkout, "main")

	first, err := Copy(CopyOptions{CWD: checkout})
	if err != nil {
		t.Fatal(err)
	}
	if !samePath(first.Source, repo) || !samePath(first.Path, checkout) || first.AlreadyPrepared || !reflect.DeepEqual(first.Copied, []string{".env.local"}) {
		t.Fatalf("unexpected first copy result: %#v", first)
	}
	assertFile(t, filepath.Join(checkout, ".env.local"), "primary\n")

	write(t, filepath.Join(checkout, ".env.local"), "worktree\n")
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), "files:\n  copy: [changed-but-not-recopied]\n")
	second, err := Copy(CopyOptions{CWD: checkout})
	if err != nil {
		t.Fatal(err)
	}
	if !second.AlreadyPrepared || !reflect.DeepEqual(second.Copied, []string{".env.local"}) {
		t.Fatalf("unexpected second copy result: %#v", second)
	}
	assertFile(t, filepath.Join(checkout, ".env.local"), "worktree\n")
}

func TestCopyCanCopyUntrackedProjectConfigFromPrimaryWorktree(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := initRepo(t)
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), "files:\n  copy: [.herdr-worktree.yaml, .env.local]\n")
	write(t, filepath.Join(repo, ".env.local"), "primary\n")
	checkout := filepath.Join(t.TempDir(), "feature")
	run(t, repo, "git", "worktree", "add", "-b", "copy-project-config", checkout, "main")

	result, err := Copy(CopyOptions{CWD: checkout})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Copied, []string{".herdr-worktree.yaml", ".env.local"}) {
		t.Fatalf("unexpected copied files: %#v", result.Copied)
	}
	assertFile(t, filepath.Join(checkout, ".herdr-worktree.yaml"), "files:\n  copy: [.herdr-worktree.yaml, .env.local]\n")
	assertFile(t, filepath.Join(checkout, ".env.local"), "primary\n")
}

func TestCopyRejectsPrimaryWorktree(t *testing.T) {
	repo := initRepo(t)
	if _, err := Copy(CopyOptions{CWD: repo}); err == nil || !strings.Contains(err.Error(), "linked worktree") {
		t.Fatalf("expected linked-worktree error, got %v", err)
	}
}

func TestCopySerializesConcurrentCalls(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := initRepo(t)
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), "files:\n  copy: [.env.local]\n")
	run(t, repo, "git", "add", "-f", ".herdr-worktree.yaml")
	run(t, repo, "git", "commit", "-m", "add worktree config")
	write(t, filepath.Join(repo, ".env.local"), "primary\n")
	checkout := filepath.Join(t.TempDir(), "feature")
	run(t, repo, "git", "worktree", "add", "-b", "concurrent-copy", checkout, "main")

	results := make(chan CopyResult, 2)
	errors := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Go(func() {
			result, err := Copy(CopyOptions{CWD: checkout})
			results <- result
			errors <- err
		})
	}
	wait.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	prepared := 0
	for result := range results {
		if result.AlreadyPrepared {
			prepared++
			if !reflect.DeepEqual(result.Copied, []string{".env.local"}) {
				t.Fatalf("concurrent no-op lost copied paths: %#v", result)
			}
		}
	}
	if prepared != 1 {
		t.Fatalf("expected one concurrent no-op, got %d", prepared)
	}
}

func TestCopyDoesNotMarkFailedCopyComplete(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := initRepo(t)
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), "files:\n  copy: [local]\n")
	run(t, repo, "git", "add", "-f", ".herdr-worktree.yaml")
	run(t, repo, "git", "commit", "-m", "add worktree config")
	if err := os.Mkdir(filepath.Join(repo, "local"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(repo, "local", "file"), "primary\n")
	checkout := filepath.Join(t.TempDir(), "feature")
	run(t, repo, "git", "worktree", "add", "-b", "retry-copy", checkout, "main")
	if err := os.Symlink(t.TempDir(), filepath.Join(checkout, "local")); err != nil {
		t.Fatal(err)
	}

	if _, err := Copy(CopyOptions{CWD: checkout}); err == nil {
		t.Fatal("expected the first copy to fail")
	}
	gitDir := output(t, checkout, "git", "rev-parse", "--path-format=absolute", "--git-dir")
	if _, err := os.Stat(filepath.Join(gitDir, copyMarkerName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed copy wrote marker: %v", err)
	}
	if err := os.Remove(filepath.Join(checkout, "local")); err != nil {
		t.Fatal(err)
	}
	result, err := Copy(CopyOptions{CWD: checkout})
	if err != nil {
		t.Fatal(err)
	}
	if result.AlreadyPrepared {
		t.Fatalf("retry was unexpectedly skipped: %#v", result)
	}
	assertFile(t, filepath.Join(checkout, "local", "file"), "primary\n")
}

type fakeClient struct {
	created   herdr.Created
	workspace herdr.Workspace
	runs      [][]string
	creates   [][]string
	createFn  func(...string) (herdr.Created, error)
	runErr    error
}

func (f *fakeClient) Run(args ...string) ([]byte, error) {
	f.runs = append(f.runs, append([]string(nil), args...))
	return []byte(`{"result":{}}`), f.runErr
}

func (f *fakeClient) SourceCheckout(cwd string) (string, error) {
	return cwd, nil
}

func (f *fakeClient) Create(args ...string) (herdr.Created, error) {
	f.creates = append(f.creates, append([]string(nil), args...))
	if f.createFn != nil {
		return f.createFn(args...)
	}
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

func TestNormalizeBranchName(t *testing.T) {
	if got := NormalizeBranchName("  feature/my new\tthing  "); got != "feature/my-new-thing" {
		t.Fatalf("NormalizeBranchName() = %q", got)
	}
}

func TestCreateFromDescriptionUsesTicketCommandAndConfiguredPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := initRepo(t)
	worktreeDir := t.TempDir()
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), "ticket_commands:\n  default:\n    command: [lnr, quick, '{input}', --json]\nworktree_dir: "+worktreeDir+"\nworktree_naming: basename\nworktree_prefix: project-\n")
	arguments := filepath.Join(t.TempDir(), "arguments")
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "lnr"), "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$HWT_TEST_ARGUMENTS\"\nprintf '{\"branchName\":\"dorian/rms-90-ticket-flow\",\"metadata\":{\"identifier\":\"RMS-90\"},\"url\":\"not metadata\"}\\n'\n")
	t.Setenv("HWT_TEST_ARGUMENTS", arguments)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	expectedPath := filepath.Join(worktreeDir, "project-rms-90-ticket-flow")
	client := &fakeClient{}
	client.createFn = func(_ ...string) (herdr.Created, error) {
		run(t, repo, "git", "worktree", "add", "-b", "dorian/rms-90-ticket-flow", expectedPath, "main")
		return herdr.Created{WorkspaceID: "w-ticket", PaneID: "w-ticket:p1", Path: expectedPath}, nil
	}

	result, err := Create(client, CreateOptions{CWD: repo, Input: "something; $(unsafe)", Ticket: "default", Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Branch != "dorian/rms-90-ticket-flow" || result.Path != expectedPath {
		t.Fatalf("unexpected ticket worktree result: %#v", result)
	}
	assertFile(t, arguments, "quick\nsomething; $(unsafe)\n--json\n")
	metadata, err := ReadTicketMetadata(expectedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(metadata, map[string]string{"identifier": "RMS-90"}) {
		t.Fatalf("unexpected ticket metadata: %#v", metadata)
	}
	gitDir := output(t, expectedPath, "git", "rev-parse", "--path-format=absolute", "--git-dir")
	info, err := os.Stat(filepath.Join(gitDir, ticketMetadataName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("ticket metadata mode = %o", info.Mode().Perm())
	}
	if status := output(t, expectedPath, "git", "status", "--porcelain"); status != "" {
		t.Fatalf("ticket metadata appeared in worktree status: %q", status)
	}
	joined := strings.Join(client.creates[0], "\n")
	if !strings.Contains(joined, "--branch\ndorian/rms-90-ticket-flow") || !strings.Contains(joined, "--path\n"+expectedPath) {
		t.Fatalf("ticket branch or configured path not forwarded to Herdr: %#v", client.creates[0])
	}
}

func TestCreateWithTicketRequiresDefaultCommand(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := initRepo(t)
	client := &fakeClient{}

	_, err := Create(client, CreateOptions{CWD: repo, Input: "a task", Ticket: "default", Base: "main"})
	if err == nil || !strings.Contains(err.Error(), "--ticket requires ticket_commands.default") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.creates) != 0 {
		t.Fatalf("Herdr create called without ticket_commands.default: %#v", client.creates)
	}
}

func TestCreateFromDescriptionRejectsTicketCommandFailuresBeforeHerdrCreate(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := initRepo(t)
	binDir := t.TempDir()
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{name: "command failure", script: "#!/bin/sh\nprintf 'authentication required' >&2\nexit 23\n", want: "exit status 23"},
		{name: "malformed output", script: "#!/bin/sh\nprintf 'not json'\n", want: "decode ticket command JSON output"},
		{name: "missing branch", script: "#!/bin/sh\nprintf '{}\\n'\n", want: `field "branchName" is missing`},
		{name: "malformed metadata", script: "#!/bin/sh\nprintf '{\"branchName\":\"feature/test\",\"metadata\":{\"number\":42}}\\n'\n", want: "must be a string"},
		{name: "null metadata value", script: "#!/bin/sh\nprintf '{\"branchName\":\"feature/test\",\"metadata\":{\"identifier\":null}}\\n'\n", want: "must be a string"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writeExecutable(t, filepath.Join(binDir, "tickets"), test.script)
			write(t, filepath.Join(repo, ".herdr-worktree.yaml"), "ticket_commands:\n  default:\n    command: [tickets, create, '{input}', --json]\n")
			client := &fakeClient{}
			_, err := Create(client, CreateOptions{CWD: repo, Input: "a task", Ticket: "default", Base: "main"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q error, got %v", test.want, err)
			}
			if len(client.creates) != 0 {
				t.Fatalf("Herdr create called after ticket failure: %#v", client.creates)
			}
		})
	}
}

func TestCreateFromDescriptionRejectsBranchConflictBeforeHerdrCreate(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := initRepo(t)
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "tickets"), "#!/bin/sh\nprintf '{\"branchName\":\"main\"}\\n'\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), "ticket_commands:\n  default:\n    command: [tickets]\n")
	client := &fakeClient{}

	_, err := Create(client, CreateOptions{CWD: repo, Input: "a task", Ticket: "default", Base: "main"})
	if err == nil || !strings.Contains(err.Error(), "local branch already exists") {
		t.Fatalf("expected branch conflict, got %v", err)
	}
	if len(client.creates) != 0 {
		t.Fatalf("Herdr create called after branch conflict: %#v", client.creates)
	}
}

func TestCreateFromDescriptionRejectsWorktreePathConflictBeforeHerdrCreate(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := initRepo(t)
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "tickets"), "#!/bin/sh\nprintf '{\"branchName\":\"feature/new-ticket\"}\\n'\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	worktreeDir := t.TempDir()
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), "ticket_commands:\n  default:\n    command: [tickets]\nworktree_dir: "+worktreeDir+"\nworktree_naming: basename\nworktree_prefix: project-\n")
	existingPath := filepath.Join(worktreeDir, "project-new-ticket")
	if err := os.Mkdir(existingPath, 0o755); err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{}

	_, err := Create(client, CreateOptions{CWD: repo, Input: "a task", Ticket: "default", Base: "main"})
	if err == nil || !strings.Contains(err.Error(), "worktree path "+existingPath+" already exists") {
		t.Fatalf("expected worktree path conflict, got %v", err)
	}
	if len(client.creates) != 0 {
		t.Fatalf("Herdr create called after path conflict: %#v", client.creates)
	}
}

func TestCreateKeepsAllocationWhenRollbackFails(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repo := initRepo(t)
	checkout := filepath.Join(t.TempDir(), "failed-rollback")
	run(t, repo, "git", "worktree", "add", "-b", "failed-rollback", checkout, "main")
	start := availablePort(t)
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), fmt.Sprintf("ports:\n  start: %d\n  end: %d\n  services: [web]\npost_create: [false]\n", start, start+20))
	client := &fakeClient{
		created: herdr.Created{WorkspaceID: "w-failed", PaneID: "w-failed:p1", Path: checkout},
		runErr:  errors.New("rollback failed"),
	}
	_, err := Create(client, CreateOptions{CWD: repo, Branch: "failed-rollback", Base: "main"})
	if err == nil || !strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("expected rollback failure, got %v", err)
	}
	allocation, err := reservePorts(checkout, config.Ports{Start: start, End: start + 20, Services: []string{"web"}}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if allocation["web"] == 0 {
		t.Fatal("failed rollback lost its port allocation")
	}
}

func TestEnvironmentAllocatesStableDistinctPortsAndWritesIgnoredFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repo := initRepo(t)
	start := availablePort(t)
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), fmt.Sprintf(`
ports:
  start: %d
  end: %d
  services: [web, assets, api_]
environment:
  variables:
    APP_URL: http://localhost:${HWT_PORT_WEB}
`, start, start+20))
	run(t, repo, "git", "add", "-f", ".herdr-worktree.yaml")
	run(t, repo, "git", "commit", "-m", "configure environment")
	firstPath := filepath.Join(t.TempDir(), "Feature API ++")
	secondPath := filepath.Join(t.TempDir(), "second")
	run(t, repo, "git", "worktree", "add", "-b", "first-env", firstPath, "main")
	run(t, repo, "git", "worktree", "add", "-b", "second-env", secondPath, "main")

	first, err := Environment(firstPath, false)
	if err != nil {
		t.Fatal(err)
	}
	stable, err := Environment(firstPath, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Environment(secondPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Variables["HWT_PORT_WEB"] != stable.Variables["HWT_PORT_WEB"] {
		t.Fatal("allocation was not stable")
	}
	if first.Variables["HWT_PORT_WEB"] == first.Variables["HWT_PORT_ASSETS"] || first.Variables["HWT_PORT_WEB"] == second.Variables["HWT_PORT_WEB"] {
		t.Fatalf("allocations collided: first=%#v second=%#v", first.Variables, second.Variables)
	}
	if first.Variables["HWT_WORKTREE_HOSTNAME"] != "feature-api.localhost" {
		t.Fatalf("localhost hostname = %q", first.Variables["HWT_WORKTREE_HOSTNAME"])
	}
	if first.Variables["HWT_URL_WEB"] != "http://feature-api.web.localhost:"+first.Variables["HWT_PORT_WEB"] || first.Variables["HWT_URL_ASSETS"] != "http://feature-api.assets.localhost:"+first.Variables["HWT_PORT_ASSETS"] {
		t.Fatalf("localhost URLs = %#v", first.Variables)
	}
	if first.Variables["HWT_URL_API_"] != "http://feature-api.api.localhost:"+first.Variables["HWT_PORT_API_"] {
		t.Fatalf("localhost URL did not normalize trailing separator: %#v", first.Variables)
	}
	if first.Variables["APP_URL"] != "http://localhost:"+first.Variables["HWT_PORT_WEB"] {
		t.Fatalf("variable was not expanded: %#v", first.Variables)
	}
	info, err := os.Stat(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("environment mode = %o", info.Mode().Perm())
	}
	if status := output(t, firstPath, "git", "status", "--porcelain"); status != "" {
		t.Fatalf("generated environment is not ignored: %q", status)
	}
}

func TestEnvironmentRegistersAndRefreshesLocalDNS(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repo := initRepo(t)
	start := availablePort(t)
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), fmt.Sprintf("ports:\n  start: %d\n  end: %d\n  services: [web, asset_server]\nlocal_dns:\n  enabled: true\n  domain: dev.test\n", start, start+20))
	run(t, repo, "git", "add", "-f", ".herdr-worktree.yaml")
	run(t, repo, "git", "commit", "-m", "configure local DNS")
	checkout := filepath.Join(t.TempDir(), "Feature API")
	run(t, repo, "git", "worktree", "add", "-b", "local-dns", checkout, "main")

	first, err := Environment(checkout, false)
	if err != nil {
		t.Fatal(err)
	}
	hostname := first.Variables["HWT_WORKTREE_HOSTNAME"]
	if hostname == "" || first.Variables["HWT_URL_WEB"] != "http://web."+hostname || first.Variables["HWT_URL_ASSET_SERVER"] != "http://asset-server."+hostname {
		t.Fatalf("unexpected local DNS variables: %#v", first.Variables)
	}
	status, err := localdns.Status(localdns.Config{Enabled: true, Domain: "dev.test"})
	if err != nil || len(status.Entries) != 1 {
		t.Fatalf("status = %#v, %v", status, err)
	}
	caddy, err := os.ReadFile(status.Paths.Caddyfile)
	if err != nil || !strings.Contains(string(caddy), "127.0.0.1:"+first.Variables["HWT_PORT_WEB"]) {
		t.Fatalf("Caddyfile does not consume assigned port: %s, %v", caddy, err)
	}

	oldPort, _ := strconv.Atoi(first.Variables["HWT_PORT_WEB"])
	listener, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(oldPort))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	refreshed, err := Environment(checkout, true)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Variables["HWT_WORKTREE_HOSTNAME"] != hostname || refreshed.Variables["HWT_PORT_WEB"] == first.Variables["HWT_PORT_WEB"] {
		t.Fatalf("refresh changed hostname or retained occupied port: first=%#v refreshed=%#v", first.Variables, refreshed.Variables)
	}
	caddy, err = os.ReadFile(status.Paths.Caddyfile)
	if err != nil || !strings.Contains(string(caddy), "127.0.0.1:"+refreshed.Variables["HWT_PORT_WEB"]) {
		t.Fatalf("refreshed Caddyfile does not consume new port: %s, %v", caddy, err)
	}
}

func TestPortRegistrySerializesConcurrentWorktrees(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	start := availablePort(t)
	ports := config.Ports{Start: start, End: start + 20, Services: []string{"web"}}
	results := make(chan int, 4)
	errors := make(chan error, 4)
	var wait sync.WaitGroup
	for range 4 {
		root := t.TempDir()
		wait.Go(func() {
			allocation, err := reservePorts(root, ports, false, nil)
			results <- allocation["web"]
			errors <- err
		})
	}
	wait.Wait()
	close(results)
	close(errors)
	seen := map[int]bool{}
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	for port := range results {
		if seen[port] {
			t.Fatalf("concurrent allocation reused port %d", port)
		}
		seen[port] = true
	}
}

func TestPortRegistryReclaimsStalePathsAndRefreshesConflicts(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	start := availablePort(t)
	ports := config.Ports{Start: start, End: start + 20, Services: []string{"web"}}
	staleParent := t.TempDir()
	stale := filepath.Join(staleParent, "stale")
	if err := os.Mkdir(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	first, err := reservePorts(stale, ports, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(stale); err != nil {
		t.Fatal(err)
	}
	live := t.TempDir()
	reclaimed, err := reservePorts(live, ports, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed["web"] != first["web"] {
		t.Fatalf("stale port was not reclaimed: first=%d next=%d", first["web"], reclaimed["web"])
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(reclaimed["web"]))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	refreshed, err := reservePorts(live, ports, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed["web"] == reclaimed["web"] {
		t.Fatalf("refresh retained occupied port %d", reclaimed["web"])
	}
}

func TestPortAvailableRejectsIPv4LoopbackListener(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	if portAvailable(port) {
		t.Fatalf("occupied IPv4 loopback port %d reported available", port)
	}
}

func TestEnvironmentFailureDoesNotPublishOrReplaceAllocation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	repo := initRepo(t)
	start := availablePort(t)
	valid := fmt.Sprintf("ports:\n  start: %d\n  end: %d\n  services: [web]\n", start, start+20)
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), valid)
	run(t, repo, "git", "add", "-f", ".herdr-worktree.yaml")
	run(t, repo, "git", "commit", "-m", "configure ports")
	checkout := filepath.Join(t.TempDir(), "transactional")
	run(t, repo, "git", "worktree", "add", "-b", "transactional", checkout, "main")
	initial, err := Environment(checkout, false)
	if err != nil {
		t.Fatal(err)
	}
	oldPort := initial.Variables["HWT_PORT_WEB"]
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), valid+"environment:\n  variables:\n    BROKEN: ${UNKNOWN}\n")
	if _, err := Environment(checkout, true); err == nil || !strings.Contains(err.Error(), "UNKNOWN") {
		t.Fatalf("expected expansion failure, got %v", err)
	}
	registry, err := readPortRegistry(filepath.Join(state, "hwt", "ports.json"))
	if err != nil {
		t.Fatal(err)
	}
	canonicalCheckout, err := canonicalWorktreePath(checkout)
	if err != nil {
		t.Fatal(err)
	}
	if port := registry.Worktrees[canonicalCheckout].Ports["web"]; strconv.Itoa(port) != oldPort {
		t.Fatalf("failed refresh replaced allocation: old=%s new=%d", oldPort, port)
	}
	other := t.TempDir()
	otherAllocation, err := reservePorts(other, config.Ports{Start: start, End: start + 20, Services: []string{"other"}}, false, func(map[string]int) error { return errors.New("publish failed") })
	if err == nil || otherAllocation != nil {
		t.Fatalf("expected publication failure, got %#v, %v", otherAllocation, err)
	}
	registry, err = readPortRegistry(filepath.Join(state, "hwt", "ports.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := registry.Worktrees[other]; exists {
		t.Fatal("failed publication leaked allocation")
	}
}

func TestEnvironmentReloadFailureRestoresPortsDNSAndEnvironment(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	repo := initRepo(t)
	start := availablePort(t)
	baseConfig := fmt.Sprintf("ports:\n  start: %d\n  end: %d\n  services: [web]\nlocal_dns:\n  enabled: true\n", start, start+20)
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), baseConfig)
	run(t, repo, "git", "add", "-f", ".herdr-worktree.yaml")
	run(t, repo, "git", "commit", "-m", "configure local DNS")
	checkout := filepath.Join(t.TempDir(), "reload-rollback")
	run(t, repo, "git", "worktree", "add", "-b", "reload-rollback", checkout, "main")
	result, err := Environment(checkout, false)
	if err != nil {
		t.Fatal(err)
	}
	status, err := localdns.Status(localdns.Config{Domain: "hwt.test"})
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{result.Path, filepath.Join(state, "hwt", "ports.json"), status.Paths.Registry, status.Paths.DNSMasq, status.Paths.Caddyfile}
	before := map[string][]byte{}
	for _, path := range paths {
		before[path], err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	failing := filepath.Join(t.TempDir(), "reload")
	writeExecutable(t, failing, "#!/bin/sh\necho reload-failed >&2\nexit 1\n")
	failingConfig := fmt.Sprintf("ports:\n  start: %d\n  end: %d\n  services: [web]\nlocal_dns:\n  enabled: true\n  reload: [%s]\n", start+1, start+20, failing)
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), failingConfig)
	if _, err := Environment(checkout, true); err == nil || !strings.Contains(err.Error(), "reload-failed") {
		t.Fatalf("expected reload failure, got %v", err)
	}
	for _, path := range paths {
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(after, before[path]) {
			t.Fatalf("failed refresh changed %s", path)
		}
	}
}

func TestEnvironmentRemovingServicesReleasesAllocation(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	start := availablePort(t)
	root := t.TempDir()
	configured := config.Ports{Start: start, End: start + 20, Services: []string{"web"}}
	first, err := reservePorts(root, configured, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reservePorts(root, config.Ports{Start: start, End: start + 20}, false, nil); err != nil {
		t.Fatal(err)
	}
	next, err := reservePorts(t.TempDir(), configured, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if next["web"] != first["web"] {
		t.Fatalf("released port was not reused: first=%d next=%d", first["web"], next["web"])
	}
}

func TestRunHooksReplacesAmbientEnvironment(t *testing.T) {
	t.Setenv("HWT_PORT_WEB", "ambient")
	path := filepath.Join(t.TempDir(), "hook-env")
	if err := runHooks(filepath.Dir(path), []string{"printf %s \"$HWT_PORT_WEB\" > hook-env"}, map[string]string{"HWT_PORT_WEB": "reserved"}); err != nil {
		t.Fatal(err)
	}
	assertFile(t, path, "reserved")
}

func TestDotenvQuotePreventsShellExpansion(t *testing.T) {
	quoted := dotenvQuote("$HOME `echo unsafe` \\\"\n")
	expected := "\"\\$HOME \\`echo unsafe\\` \\\\\\\"\\n\""
	if quoted != expected {
		t.Fatalf("dotenvQuote() = %q, want %q", quoted, expected)
	}
}

func availablePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	if port > 65515 {
		return 40000
	}
	return port
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
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repo := initRepo(t)
	start := availablePort(t)
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), fmt.Sprintf("ports:\n  start: %d\n  end: %d\n  services: [web]\nlocal_dns:\n  enabled: true\n", start, start+10))
	run(t, repo, "git", "add", "-f", ".herdr-worktree.yaml")
	run(t, repo, "git", "commit", "-m", "configure local DNS")
	checkout := filepath.Join(t.TempDir(), "remove-me")
	run(t, repo, "git", "worktree", "add", "-b", "remove-me", checkout, "main")
	if _, err := Environment(checkout, false); err != nil {
		t.Fatal(err)
	}
	gitDir := output(t, checkout, "git", "rev-parse", "--path-format=absolute", "--git-dir")
	if err := writeTicketMetadata(checkout, map[string]string{"identifier": "RMS-85"}); err != nil {
		t.Fatal(err)
	}
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
	status, err := localdns.Status(localdns.Config{Domain: "hwt.test"})
	if err != nil || len(status.Entries) != 0 {
		t.Fatalf("local DNS registration survived removal: %#v, %v", status.Entries, err)
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
	if !CanForceRemove(err) {
		t.Fatalf("dirty worktree error should permit force removal: %v", err)
	}
}

func TestRemoveReloadFailureRestoresRouteAfterCleanup(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repo := initRepo(t)
	failing := filepath.Join(t.TempDir(), "reload")
	writeExecutable(t, failing, "#!/bin/sh\necho cleanup-reload-failed >&2\nexit 1\n")
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), "ports:\n  services: [web]\nlocal_dns:\n  enabled: true\n  reload: ["+failing+"]\n")
	run(t, repo, "git", "add", "-f", ".herdr-worktree.yaml")
	run(t, repo, "git", "commit", "-m", "configure failing reload")
	checkout := filepath.Join(t.TempDir(), "cleanup-rollback")
	run(t, repo, "git", "worktree", "add", "-b", "cleanup-rollback", checkout, "main")
	if _, err := localdns.Register(localdns.Config{Enabled: true, Domain: "hwt.test"}, localdns.Registration{Repository: repo, Worktree: checkout, Services: map[string]int{"web": 30000}}); err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{workspace: herdr.Workspace{ID: "w-cleanup", CheckoutPath: checkout, LinkedWorktree: true}}

	_, err := Remove(client, RemoveOptions{WorkspaceID: "w-cleanup"})
	if err == nil || !strings.Contains(err.Error(), "worktree removed but local DNS cleanup failed") || !strings.Contains(err.Error(), "cleanup-reload-failed") {
		t.Fatalf("expected cleanup reload failure, got %v", err)
	}
	if _, statErr := os.Stat(checkout); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("worktree was restored after completed cleanup: %v", statErr)
	}
	status, statusErr := localdns.Status(localdns.Config{Domain: "hwt.test"})
	if statusErr != nil || len(status.Entries) != 1 {
		t.Fatalf("failed cleanup did not restore route: %#v, %v", status.Entries, statusErr)
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

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	write(t, path, content)
	if err := os.Chmod(path, 0o755); err != nil {
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
