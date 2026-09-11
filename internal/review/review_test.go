package review

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dkarter/hwt/internal/gitutil"
	"github.com/dkarter/hwt/internal/herdr"
	"github.com/dkarter/hwt/internal/pullrequest"
)

type fakeClient struct {
	source    string
	worktrees []herdr.Worktree
	panes     []herdr.Pane
	processes map[string][]herdr.Process
	runs      [][]string
	created   herdr.Created
	opened    herdr.Created
	createErr error
	openErr   error
}

func (f *fakeClient) Run(args ...string) ([]byte, error) {
	f.runs = append(f.runs, append([]string(nil), args...))
	return []byte(`{}`), nil
}

func (f *fakeClient) SourceCheckout(string) (string, error) { return f.source, nil }
func (f *fakeClient) Workspace(string) (herdr.Workspace, error) {
	return herdr.Workspace{}, nil
}
func (f *fakeClient) CurrentWorkspaceID() (string, error) { return "", nil }
func (f *fakeClient) Worktrees(string) ([]herdr.Worktree, error) {
	return append([]herdr.Worktree(nil), f.worktrees...), nil
}
func (f *fakeClient) Panes(string) ([]herdr.Pane, error) {
	return append([]herdr.Pane(nil), f.panes...), nil
}
func (f *fakeClient) ProcessInfo(paneID string) ([]herdr.Process, error) {
	return append([]herdr.Process(nil), f.processes[paneID]...), nil
}
func (f *fakeClient) Split(string, string) (herdr.Pane, error) {
	return herdr.Pane{ID: "w-existing:p2", WorkspaceID: "w-existing"}, nil
}
func (f *fakeClient) Open(...string) (herdr.Created, error) { return f.opened, f.openErr }

func (f *fakeClient) Create(args ...string) (herdr.Created, error) {
	if f.createErr != nil {
		return herdr.Created{}, f.createErr
	}
	branch := argument(args, "--branch")
	path := f.created.Path
	if path == "" {
		path = filepath.Join(filepath.Dir(f.source), "review-worktree")
	}
	command := exec.Command("git", "-C", f.source, "worktree", "add", path, branch)
	command.Env = gitutil.Environment()
	if output, err := command.CombinedOutput(); err != nil {
		return herdr.Created{}, errors.New(strings.TrimSpace(string(output)) + ": " + err.Error())
	}
	created := f.created
	created.Path = path
	if created.WorkspaceID == "" {
		created.WorkspaceID = "w-review"
	}
	if created.PaneID == "" {
		created.PaneID = "w-review:p1"
	}
	return created, nil
}

func TestRunCreatesBranchReviewWorkspaceAndLaunchesConfiguredArgv(t *testing.T) {
	repo := initRepository(t)
	write(t, filepath.Join(repo, ".herdr-worktree.yaml"), "review_command: [review-tool, 'literal; $(unsafe)']\n")
	runGit(t, repo, "branch", "feature/test")
	client := &fakeClient{source: repo}

	result, err := run(client, Options{CWD: repo, Selector: "feature/test"}, dependencies{
		resolvePR: pullrequest.ResolveMetadata,
		runGit:    gitOutput,
		findTool:  func(string, string) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Identity.Kind != "branch" || result.Reused || result.Launch.Status != "launched" || result.WorkspaceID != "w-review" {
		t.Fatalf("unexpected result: %#v", result)
	}
	want := []string{"pane", "run", "w-review:p1", "'review-tool' 'literal; $(unsafe)'"}
	if !reflect.DeepEqual(client.runs[len(client.runs)-1], want) {
		t.Fatalf("launch = %#v, want %#v", client.runs[len(client.runs)-1], want)
	}
	if branch := outputGit(t, repo, "branch", "--show-current"); branch != "main" {
		t.Fatalf("primary checkout changed to %q", branch)
	}
}

func TestRunFetchesForkPullRequestRefAndVerifiesSHA(t *testing.T) {
	repo := initRepository(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	runGit(t, repo, "clone", "--bare", repo, remote)
	runGit(t, repo, "remote", "add", "origin", remote)
	head := outputGit(t, repo, "rev-parse", "HEAD")
	runGit(t, remote, "update-ref", "refs/pull/7/head", head)
	client := &fakeClient{source: repo}
	metadata := pullrequest.Metadata{Number: 7, URL: "https://github.com/acme/app/pull/7", Title: "Fork change", HeadRefName: "feature/fork", HeadRefOID: head, BaseRefName: "main", IsCrossRepository: true}

	result, err := run(client, Options{CWD: repo, Selector: metadata.URL, Remote: "origin"}, dependencies{
		resolvePR: func(pullrequest.Options) (pullrequest.Metadata, error) { return metadata, nil },
		runGit:    gitOutput,
		findTool:  func(string, string) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Branch != "hwt/review/pr-7" || result.Commit != head || !result.Identity.Fork || result.Identity.Remote != "origin" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRunRejectsReviewBranchAtDifferentCommit(t *testing.T) {
	repo := initRepository(t)
	runGit(t, repo, "branch", "feature")
	reviewBranch := branchName("feature")
	runGit(t, repo, "branch", reviewBranch)
	write(t, filepath.Join(repo, "change"), "change")
	runGit(t, repo, "add", "change")
	runGit(t, repo, "commit", "-m", "change")
	runGit(t, repo, "branch", "-f", "feature")

	_, err := run(&fakeClient{source: repo}, Options{CWD: repo, Selector: "feature"}, dependencies{pullrequest.ResolveMetadata, gitOutput, func(string, string) error { return nil }})
	if err == nil || !strings.Contains(err.Error(), "not requested commit") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunReusesExactOpenWorkspaceWithoutDuplicateLaunch(t *testing.T) {
	repo := initRepository(t)
	runGit(t, repo, "branch", "feature")
	commit := outputGit(t, repo, "rev-parse", "feature")
	reviewBranch := branchName("feature")
	runGit(t, repo, "branch", reviewBranch, commit)
	path := filepath.Join(filepath.Dir(repo), "existing-review")
	runGit(t, repo, "worktree", "add", path, reviewBranch)
	runGit(t, repo, "config", "branch."+reviewBranch+".herdr-base", commit)
	runGit(t, repo, "config", "branch."+reviewBranch+".hwt-review-pane", "w-existing:p1")
	client := &fakeClient{
		source:    repo,
		worktrees: []herdr.Worktree{{Branch: reviewBranch, Path: path, Linked: true, OpenWorkspaceID: "w-existing"}},
		panes:     []herdr.Pane{{ID: "w-existing:p1", WorkspaceID: "w-existing"}},
		processes: map[string][]herdr.Process{"w-existing:p1": {{Name: "tuicr", Argv0: "tuicr", Argv: []string{"tuicr"}}}},
	}

	result, err := run(client, Options{CWD: repo, Selector: "feature"}, dependencies{pullrequest.ResolveMetadata, gitOutput, func(string, string) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Reused || result.Launch.Status != "already_open" || result.PaneID != "w-existing:p1" || len(client.runs) != 0 {
		t.Fatalf("unexpected reuse: result=%#v runs=%#v", result, client.runs)
	}
}

func TestRunRelaunchesReviewerInIdleOpenWorkspace(t *testing.T) {
	repo := initRepository(t)
	runGit(t, repo, "branch", "feature")
	commit := outputGit(t, repo, "rev-parse", "feature")
	reviewBranch := branchName("feature")
	runGit(t, repo, "branch", reviewBranch, commit)
	path := filepath.Join(filepath.Dir(repo), "idle-review")
	runGit(t, repo, "worktree", "add", path, reviewBranch)
	baseCommit := outputGit(t, repo, "rev-parse", "main")
	runGit(t, repo, "config", "branch."+reviewBranch+".herdr-base", baseCommit)
	client := &fakeClient{
		source:    repo,
		worktrees: []herdr.Worktree{{Branch: reviewBranch, Path: path, Linked: true, OpenWorkspaceID: "w-existing"}},
		panes:     []herdr.Pane{{ID: "w-existing:p1", WorkspaceID: "w-existing"}},
	}

	result, err := run(client, Options{CWD: repo, Selector: "feature"}, dependencies{pullrequest.ResolveMetadata, gitOutput, func(string, string) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	if result.Launch.Status != "launched" || result.PaneID != "w-existing:p1" {
		t.Fatalf("unexpected relaunch result: %#v", result)
	}
}

func TestShellQuoteKeepsEveryArgumentLiteral(t *testing.T) {
	if got, want := shellQuote("it's $(unsafe); here"), `'it'\''s $(unsafe); here'`; got != want {
		t.Fatalf("shellQuote() = %q, want %q", got, want)
	}
}

func TestSelectBranchRemoteRejectsConflictingSelectors(t *testing.T) {
	_, _, err := selectBranchRemote("upstream/feature", "origin", []string{"origin", "upstream"}, "/repo", gitOutput)
	if err == nil || !strings.Contains(err.Error(), `selects remote "upstream" but --remote selects "origin"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFindToolAcceptsExecutableReviewStub(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review-stub")
	write(t, path, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := findTool(filepath.Dir(path), "./review-stub"); err != nil {
		t.Fatal(err)
	}
}

func TestRunKeepsWorkspaceWhenReviewToolIsMissing(t *testing.T) {
	repo := initRepository(t)
	runGit(t, repo, "branch", "feature")
	client := &fakeClient{source: repo}

	result, err := run(client, Options{CWD: repo, Selector: "feature"}, dependencies{
		resolvePR: pullrequest.ResolveMetadata,
		runGit:    gitOutput,
		findTool:  func(string, string) error { return errors.New("missing tool") },
	})
	if err == nil || !strings.Contains(err.Error(), "workspace w-review is ready") || result.Path == "" || result.Launch.Status != "failed" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if _, statErr := os.Stat(result.Path); statErr != nil {
		t.Fatalf("review worktree was not retained: %v", statErr)
	}
}

func initRepository(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "main")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test")
	write(t, filepath.Join(repo, "README.md"), "test")
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "initial")
	return repo
}

type testingT interface {
	Helper()
	Fatal(args ...any)
}

func runGit(t testingT, cwd string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	command.Env = gitutil.Environment()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatal(string(output) + ": " + err.Error())
	}
}

func outputGit(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	command.Env = gitutil.Environment()
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(output))
}

func write(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func argument(args []string, name string) string {
	for index, value := range args {
		if value == name && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}
