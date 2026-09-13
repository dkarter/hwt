package namedurl

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/dkarter/hwt/internal/config"
	"github.com/dkarter/hwt/internal/localdns"
	"github.com/dkarter/hwt/internal/pullrequest"
)

type response struct {
	output string
	err    error
}

type fakeRunner struct {
	responses []response
	calls     [][]string
}

func (runner *fakeRunner) Run(_ string, name string, args ...string) ([]byte, error) {
	runner.calls = append(runner.calls, append([]string{name}, args...))
	if len(runner.responses) == 0 {
		return nil, errors.New("unexpected command")
	}
	response := runner.responses[0]
	runner.responses = runner.responses[1:]
	return []byte(response.output), response.err
}

func testDependencies(commands runner, cfg config.Config) dependencies {
	return dependencies{
		commands: commands,
		loadConfig: func(string, ...string) (config.Config, config.Sources, error) {
			return cfg, config.Sources{}, nil
		},
		readTicket: func(string) (map[string]string, error) { return nil, nil },
		resolvePR: func(pullrequest.Options) (pullrequest.Reference, error) {
			return pullrequest.Reference{}, errors.New("unexpected PR lookup")
		},
	}
}

func TestResolveMergesMetadataAndRunsOnlyRequiredCommands(t *testing.T) {
	commands := &fakeRunner{responses: []response{
		{output: "/worktrees/app-feature\n"},
		{output: "Feature/API v2\n"},
		{output: `{"host":"deploy.example","token":"secret"}`},
	}}
	cfg := config.Config{
		URLs: map[string]string{
			"preview": "https://{deployment.host}/{region}/{ticket.identifier}/{branch}",
		},
		Metadata: config.Metadata{
			Values: map[string]string{"region": "global", "ticket.identifier": "fallback", "deployment.host": "static.example"},
			Commands: map[string][]string{
				"deployment": {"metadata-helper", "--branch", "{branch}", "literal; $(not-a-shell)"},
				"unused":     {"must-not-run"},
			},
		},
	}
	deps := testDependencies(commands, cfg)
	deps.readTicket = func(string) (map[string]string, error) {
		return map[string]string{"identifier": "RMS-85"}, nil
	}

	result, err := resolve(deps, Options{Name: "preview", CWD: "/worktrees/app-feature/subdir"})
	if err != nil {
		t.Fatal(err)
	}
	wantURL := "https://deploy.example/global/RMS-85/Feature%2FAPI%20v2"
	if result.Name != "preview" || result.URL != wantURL {
		t.Fatalf("result = %#v, want URL %q", result, wantURL)
	}
	wantCommand := []string{"metadata-helper", "--branch", "Feature/API v2", "literal; $(not-a-shell)"}
	if !reflect.DeepEqual(commands.calls[2], wantCommand) {
		t.Fatalf("metadata command = %#v, want %#v", commands.calls[2], wantCommand)
	}
}

func TestResolveLoadsCurrentConfigAndTicketMetadataOnlyWhenRequested(t *testing.T) {
	commands := &fakeRunner{responses: []response{{output: "/worktrees/current\n"}, {output: "main\n"}}}
	cfg := config.Config{URLs: map[string]string{"preview": "https://example.com/{branch}"}}
	deps := testDependencies(commands, cfg)
	var loadedFrom string
	deps.loadConfig = func(root string, _ ...string) (config.Config, config.Sources, error) {
		loadedFrom = root
		return cfg, config.Sources{}, nil
	}
	deps.readTicket = func(string) (map[string]string, error) {
		return nil, errors.New("ticket metadata should not be read")
	}

	if _, err := resolve(deps, Options{Name: "preview", CWD: "/worktrees/current/subdir"}); err != nil {
		t.Fatal(err)
	}
	if loadedFrom != "/worktrees/current" {
		t.Fatalf("configuration loaded from %q", loadedFrom)
	}
}

func TestResolveDoesNotTreatCustomPRMetadataAsBuiltIn(t *testing.T) {
	commands := &fakeRunner{responses: []response{{output: "/worktrees/current\n"}, {output: "main\n"}}}
	cfg := config.Config{
		URLs:     map[string]string{"status": "https://example.com/{pr_label}"},
		Metadata: config.Metadata{Values: map[string]string{"pr_label": "ready"}},
	}

	result, err := resolve(testDependencies(commands, cfg), Options{Name: "status", CWD: "/worktrees/current"})
	if err != nil {
		t.Fatal(err)
	}
	if result.URL != "https://example.com/ready" {
		t.Fatalf("URL = %q", result.URL)
	}
}

func TestResolveAllCachesSharedRepositoryAndMetadataLookups(t *testing.T) {
	commands := &fakeRunner{responses: []response{
		{output: "/worktrees/current\n"},
		{output: "main\n"},
		{output: `{"host":"deploy.example"}`},
	}}
	cfg := config.Config{
		URLs: map[string]string{
			"alpha": "https://{deploy.host}/alpha/{branch}",
			"bravo": "https://{deploy.host}/bravo/{branch}",
		},
		Metadata: config.Metadata{Commands: map[string][]string{"deploy": {"metadata-helper"}}},
	}

	results, err := resolveAll(testDependencies(commands, cfg), Options{CWD: "/worktrees/current"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Name != "alpha" || results[1].Name != "bravo" {
		t.Fatalf("results = %#v", results)
	}
	if len(commands.calls) != 3 {
		t.Fatalf("shared lookups ran more than once: %#v", commands.calls)
	}
}

func TestResolveAllSkipsURLsWhenBranchHasNoPullRequest(t *testing.T) {
	commands := &fakeRunner{responses: []response{
		{output: "/worktrees/current\n"},
		{output: "main\n"},
	}}
	cfg := config.Config{URLs: map[string]string{
		"branch":  "https://example.com/{branch}",
		"pr":      "https://example.com/pull/{pr_number}",
		"preview": "https://preview.example/pr-{pr_number}",
	}}
	deps := testDependencies(commands, cfg)
	deps.resolvePR = func(pullrequest.Options) (pullrequest.Reference, error) {
		return pullrequest.Reference{}, pullrequest.ErrNotFound
	}

	results, err := resolveAll(deps, Options{CWD: "/worktrees/current"})
	if err != nil {
		t.Fatal(err)
	}
	want := []Result{{Name: "branch", URL: "https://example.com/main"}}
	if !reflect.DeepEqual(results, want) {
		t.Fatalf("results = %#v, want %#v", results, want)
	}
}

func TestResolveAllPreservesOtherPullRequestErrors(t *testing.T) {
	commands := &fakeRunner{responses: []response{{output: "/worktrees/current\n"}, {output: "main\n"}}}
	cfg := config.Config{URLs: map[string]string{"pr": "https://example.com/pull/{pr_number}"}}
	deps := testDependencies(commands, cfg)
	deps.resolvePR = func(pullrequest.Options) (pullrequest.Reference, error) {
		return pullrequest.Reference{}, errors.New("authentication failed")
	}

	_, err := resolveAll(deps, Options{CWD: "/worktrees/current"})
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveBuiltInGitAndWorktreeMetadata(t *testing.T) {
	commands := &fakeRunner{responses: []response{
		{output: "/worktrees/app-feature\n"},
		{output: "Feature/API v2\n"},
		{output: "worktree /projects/app\x00"},
	}}
	template := "https://example.com/{repository}/{branch}/{sanitized_branch}/{worktree}"
	result, err := resolve(testDependencies(commands, config.Config{URLs: map[string]string{"details": template}}), Options{Name: "details", CWD: "/worktrees/app-feature"})
	if err != nil {
		t.Fatal(err)
	}
	want := "https://example.com/app/Feature%2FAPI%20v2/feature-api-v2/app-feature"
	if result.URL != want {
		t.Fatalf("URL = %q, want %q", result.URL, want)
	}
}

func TestResolveExplicitBranchAndPullRequestValues(t *testing.T) {
	commands := &fakeRunner{responses: []response{{output: "/worktrees/current\n"}}}
	cfg := config.Config{URLs: map[string]string{"pr": "https://{pr_host}/{pr_owner}/{pr_repository}/merge_requests/{pr_number}?branch={branch}"}}
	deps := testDependencies(commands, cfg)
	var received pullrequest.Options
	deps.resolvePR = func(options pullrequest.Options) (pullrequest.Reference, error) {
		received = options
		return pullrequest.Reference{Host: "github.com", Owner: "acme", Repository: "app", Number: 42}, nil
	}

	result, err := resolve(deps, Options{Name: "pr", CWD: "/worktrees/current", Branch: "feature/new", Repository: "acme/app"})
	if err != nil {
		t.Fatal(err)
	}
	if result.URL != "https://github.com/acme/app/merge_requests/42?branch=feature%2Fnew" {
		t.Fatalf("unexpected URL: %q", result.URL)
	}
	if received.Branch != "feature/new" || received.Repository != "acme/app" {
		t.Fatalf("unexpected PR lookup: %#v", received)
	}
}

func TestResolveLocalHostnameWithoutSideEffects(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repository := t.TempDir()
	worktree := t.TempDir()
	commands := &fakeRunner{responses: []response{{output: worktree + "\n"}, {output: "feature/local\n"}, {output: "worktree " + repository + "\x00"}}}
	cfg := config.Config{
		LocalDNS: config.LocalDNS{Enabled: true, Domain: "hwt.test"},
		URLs:     map[string]string{"local": "http://web.{hostname}"},
	}
	result, err := resolve(testDependencies(commands, cfg), Options{Name: "local", CWD: worktree})
	if err != nil {
		t.Fatal(err)
	}
	hostname, err := localdns.Hostname(repository, worktree, "hwt.test")
	if err != nil {
		t.Fatal(err)
	}
	if result.URL != "http://web."+hostname {
		t.Fatalf("URL = %q", result.URL)
	}
	stateEntries, err := os.ReadDir(os.Getenv("XDG_STATE_HOME"))
	if err != nil || len(stateEntries) != 0 {
		t.Fatalf("hostname resolution wrote state: %#v, %v", stateEntries, err)
	}
}

func TestResolveRejectsUnavailableHostname(t *testing.T) {
	for _, test := range []struct {
		name    string
		branch  string
		enabled bool
		want    string
	}{
		{name: "explicit branch", branch: "feature/other", enabled: true, want: "unavailable for an explicit branch"},
		{name: "disabled", enabled: false, want: "requires local_dns.enabled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := t.TempDir()
			worktree := t.TempDir()
			responses := []response{{output: worktree + "\n"}}
			if test.branch == "" {
				responses = append(responses, response{output: "feature/local\n"}, response{output: "worktree " + repository + "\x00"})
			}
			cfg := config.Config{LocalDNS: config.LocalDNS{Enabled: test.enabled, Domain: "hwt.test"}, URLs: map[string]string{"local": "http://web.{hostname}"}}
			_, err := resolve(testDependencies(&fakeRunner{responses: responses}, cfg), Options{Name: "local", CWD: worktree, Branch: test.branch})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestResolveCurrentBranchPRNumber(t *testing.T) {
	commands := &fakeRunner{responses: []response{{output: "/worktrees/current\n"}, {output: "feature/current\n"}}}
	deps := testDependencies(commands, config.Config{URLs: map[string]string{"preview": "https://example.com/pr-{pr_number}"}})
	deps.resolvePR = func(options pullrequest.Options) (pullrequest.Reference, error) {
		if options.Branch != "feature/current" {
			t.Fatalf("PR branch = %q", options.Branch)
		}
		return pullrequest.Reference{Number: 9}, nil
	}

	result, err := resolve(deps, Options{Name: "preview", CWD: "/worktrees/current"})
	if err != nil {
		t.Fatal(err)
	}
	if result.URL != "https://example.com/pr-9" {
		t.Fatalf("unexpected URL: %q", result.URL)
	}
}

func TestResolveDoesNotPersistCommandSecretsOrURL(t *testing.T) {
	root := t.TempDir()
	commands := &fakeRunner{responses: []response{
		{output: root + "\n"},
		{output: `{"password":"secret@example"}`},
	}}
	cfg := config.Config{
		URLs:     map[string]string{"database": "postgres://user:{database.password}@db.example/app"},
		Metadata: config.Metadata{Commands: map[string][]string{"database": {"credentials", "--json"}}},
	}
	result, err := resolve(testDependencies(commands, cfg), Options{Name: "database", CWD: root, Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if result.URL != "postgres://user:secret%40example@db.example/app" {
		t.Fatalf("unexpected URL: %q", result.URL)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("resolver persisted secret state: %#v", entries)
	}
}

func TestResolveErrors(t *testing.T) {
	tests := []struct {
		name     string
		template string
		branch   string
		command  []string
		response string
		prError  error
		want     string
	}{
		{name: "unknown URL", want: "is not configured"},
		{name: "missing metadata", template: "https://example.com/{missing}", branch: "main", want: "unknown placeholder {missing}"},
		{name: "explicit ticket", template: "https://example.com/{ticket.identifier}", branch: "main", want: "ticket metadata is unavailable"},
		{name: "malformed command output", template: "https://example.com/{deploy.host}", branch: "main", command: []string{"helper"}, response: `{"host":42}`, want: "decode JSON object"},
		{name: "missing PR", template: "https://example.com/{pr_number}", branch: "main", prError: errors.New("no pull requests found"), want: "no pull requests found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			responses := []response{{output: "/worktrees/current\n"}}
			cfg := config.Config{URLs: map[string]string{}}
			if test.template != "" {
				cfg.URLs["preview"] = test.template
			}
			if test.command != nil {
				cfg.Metadata.Commands = map[string][]string{"deploy": test.command}
				responses = append(responses, response{output: test.response})
			}
			deps := testDependencies(&fakeRunner{responses: responses}, cfg)
			if test.prError != nil {
				deps.resolvePR = func(pullrequest.Options) (pullrequest.Reference, error) { return pullrequest.Reference{}, test.prError }
			}
			_, err := resolve(deps, Options{Name: "preview", CWD: "/worktrees/current", Branch: test.branch})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestBrowserURLRejectsNonBrowserSchemes(t *testing.T) {
	if err := BrowserURL("https://example.com"); err != nil {
		t.Fatal(err)
	}
	if err := BrowserURL("postgres://user:secret@example.com/database"); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("unexpected error: %v", err)
	}
}
