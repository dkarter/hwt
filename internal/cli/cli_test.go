package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dkarter/hwt/internal/gitutil"
	"github.com/dkarter/hwt/internal/namedurl"
	"github.com/dkarter/hwt/skills"
)

func TestConfigInitGitCommonRefusesExistingYMLConfig(t *testing.T) {
	repo := t.TempDir()
	command := exec.Command("git", "-C", repo, "init", "-b", "main")
	command.Env = gitutil.Environment()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", output, err)
	}
	t.Chdir(repo)
	existing := filepath.Join(repo, ".git", "hwt", "config.yml")
	if err := os.MkdirAll(filepath.Dir(existing), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existing, []byte("files: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := New("test")
	root.SetArgs([]string{"config", "init", "--git-common"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite ") || !strings.HasSuffix(err.Error(), "/.git/hwt/config.yml") {
		t.Fatalf("expected overwrite refusal, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "hwt", "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("config.yaml was unexpectedly created: %v", err)
	}
}

func TestPluginCommandsForwardToHerdr(t *testing.T) {
	tests := []struct {
		version string
		command []string
		want    string
	}{
		{version: "test", command: []string{"plugin", "install"}, want: "plugin\ninstall\ndkarter/hwt/plugins/herdr\n--yes\n"},
		{version: "1.2.3", command: []string{"plugin", "update"}, want: "plugin\ninstall\ndkarter/hwt/plugins/herdr\n--ref\nv1.2.3\n--yes\n"},
		{version: "1.2.4-dev.20260911.42.1.gabcdef0", command: []string{"plugin", "update"}, want: "plugin\ninstall\ndkarter/hwt/plugins/herdr\n--ref\nv1.2.4-dev.20260911.42.1.gabcdef0\n--yes\n"},
		{version: "test", command: []string{"plugin", "uninstall"}, want: "plugin\nuninstall\nhwt.worktrees\n"},
	}
	for _, test := range tests {
		t.Run(strings.Join(test.command, " "), func(t *testing.T) {
			arguments := filepath.Join(t.TempDir(), "arguments")
			herdr := filepath.Join(t.TempDir(), "herdr")
			if err := os.WriteFile(herdr, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$HWT_TEST_ARGUMENTS\"\nprintf 'herdr output\\n'\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HWT_TEST_ARGUMENTS", arguments)
			root := New(test.version)
			var output bytes.Buffer
			root.SetOut(&output)
			root.SetArgs(append([]string{"--herdr-bin", herdr}, test.command...))

			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			actual, err := os.ReadFile(arguments)
			if err != nil {
				t.Fatal(err)
			}
			if string(actual) != test.want {
				t.Fatalf("Herdr arguments = %q, want %q", actual, test.want)
			}
			if output.String() != "herdr output\n" {
				t.Fatalf("forwarded output = %q", output.String())
			}
		})
	}
}

func TestPluginCommandReportsHerdrFailure(t *testing.T) {
	herdr := filepath.Join(t.TempDir(), "herdr")
	if err := os.WriteFile(herdr, []byte("#!/bin/sh\nprintf 'install failed\\n' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := New("test")
	root.SetArgs([]string{"--herdr-bin", herdr, "plugin", "update"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "install failed") {
		t.Fatalf("expected Herdr failure, got %v", err)
	}
}

func TestDefaultPullRequestURLJSONDoesNotOpenBrowser(t *testing.T) {
	bin := t.TempDir()
	git := filepath.Join(bin, "git")
	if err := os.WriteFile(git, []byte("#!/bin/sh\ncase \"$1\" in\n  rev-parse) pwd ;;\n  branch) echo feature/test ;;\n  remote) echo origin ;;\nesac\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	gh := filepath.Join(bin, "gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\nprintf '%s' '{\"url\":\"https://github.com/acme/app/pull/9\"}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Chdir(t.TempDir())
	root := New("test")
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"url", "pr", "--json"})

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if output.String() != "{\n  \"name\": \"pr\",\n  \"url\": \"https://github.com/acme/app/pull/9\"\n}\n" {
		t.Fatalf("unexpected output: %q", output.String())
	}
}

func TestConfiguredPreviewURLJSONDoesNotOpenBrowser(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	command := exec.Command("git", "-C", repo, "init", "-b", "feature/test")
	command.Env = gitutil.Environment()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", output, err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".herdr-worktree.yaml"), []byte("urls:\n  preview:\n    template: https://preview.example/{branch}\n    label: Branch preview\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := New("test")
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"url", "preview", "--cwd", repo, "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if output.String() != "{\n  \"name\": \"preview\",\n  \"url\": \"https://preview.example/feature%2Ftest\",\n  \"label\": \"Branch preview\"\n}\n" {
		t.Fatalf("unexpected output: %q", output.String())
	}
}

func TestURLCommandOpeningIsExplicitAndBrowserSafe(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		url        string
		wantOutput string
		wantOpened bool
		wantError  string
	}{
		{name: "plain", args: []string{"url", "database"}, url: "postgres://db.example/app", wantOutput: "postgres://db.example/app\n"},
		{name: "json", args: []string{"url", "preview", "--json"}, url: "https://example.com", wantOutput: "{\n  \"name\": \"preview\",\n  \"url\": \"https://example.com\"\n}\n"},
		{name: "open", args: []string{"url", "preview", "--open"}, url: "https://example.com", wantOutput: "Opened https://example.com\n", wantOpened: true},
		{name: "reject database open", args: []string{"url", "database", "--open"}, url: "postgres://db.example/app", wantError: "refusing to open non-browser URL"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opened := false
			resolve := func(options namedurl.Options) (namedurl.Result, error) {
				return namedurl.Result{Name: options.Name, URL: test.url}, nil
			}
			command := newCommand("test", resolve, func(string) error { opened = true; return nil })
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetArgs(test.args)
			err := command.Execute()
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("error = %v, want containing %q", err, test.wantError)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if output.String() != test.wantOutput || opened != test.wantOpened {
				t.Fatalf("output = %q, opened = %t", output.String(), opened)
			}
		})
	}
}

func TestURLCommandPassesBranchRepositoryAndRefresh(t *testing.T) {
	var resolved namedurl.Options
	var opened string
	command := newCommand("test", func(options namedurl.Options) (namedurl.Result, error) {
		resolved = options
		return namedurl.Result{Name: options.Name, URL: "https://preview.example"}, nil
	}, func(value string) error {
		opened = value
		return nil
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"url", "preview", "feature/test", "--repo", "acme/app", "--refresh", "--open"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if resolved.Name != "preview" || resolved.Branch != "feature/test" || resolved.Repository != "acme/app" || !resolved.Refresh {
		t.Fatalf("unexpected options: %#v", resolved)
	}
	if opened != "https://preview.example" || output.String() != "Opened https://preview.example\n" {
		t.Fatalf("opened = %q, output = %q", opened, output.String())
	}
}

func TestSkillCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []byte
	}{
		{name: "usage", args: []string{"skill"}, want: skills.Usage},
		{name: "config reference", args: []string{"skill", "config"}, want: skills.ProjectConfig},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := New("test")
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetArgs(test.args)

			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(output.Bytes(), test.want) {
				t.Fatalf("unexpected output for hwt %v", test.args)
			}
		})
	}
}

func TestCreateRequiresPositionalBranchOrBranchFlagButNotBoth(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "neither", args: []string{"create"}},
		{name: "both", args: []string{"create", "a task", "--branch", "feature/task"}},
		{name: "branch and blank description", args: []string{"create", "   ", "--branch", "feature/task"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := New("test")
			command.SetArgs(test.args)
			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), "exactly one branch name or --branch") {
				t.Fatalf("expected unambiguous create contract error, got %v", err)
			}
		})
	}
}

func TestCreateRejectsTicketWithBranchFlag(t *testing.T) {
	command := New("test")
	command.SetArgs([]string{"create", "--branch", "feature/task", "--ticket"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "--ticket cannot be combined with --branch") {
		t.Fatalf("unexpected combined flag error: %v", err)
	}
}

func TestReviewRequiresExactlyOneUnambiguousSelector(t *testing.T) {
	for _, args := range [][]string{{"review"}, {"review", "one", "two"}} {
		command := New("test")
		command.SetArgs(args)
		err := command.Execute()
		if err == nil || !strings.Contains(err.Error(), "arg(s)") {
			t.Fatalf("hwt %v error = %v", args, err)
		}
	}
}

func TestDNSSetupStatusAndTeardownCommands(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repo := t.TempDir()
	command := exec.Command("git", "-C", repo, "init", "-b", "main")
	command.Env = gitutil.Environment()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", output, err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".herdr-worktree.yaml"), []byte("ports:\n  services: [web]\nlocal_dns:\n  enabled: true\n  domain: dev.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	for _, tool := range []string{"caddy", "dnsmasq"} {
		if err := os.WriteFile(filepath.Join(bin, tool), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := New("test")
	var setup bytes.Buffer
	root.SetOut(&setup)
	root.SetArgs([]string{"dns", "setup", "--cwd", repo, "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(setup.String(), `"dnsmasq_include"`) || !strings.Contains(setup.String(), `/hwt/local-dns/Caddyfile`) {
		t.Fatalf("unexpected setup JSON: %s", setup.String())
	}

	root = New("test")
	var status bytes.Buffer
	root.SetOut(&status)
	root.SetArgs([]string{"dns", "status", "--cwd", repo, "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.String(), `"entries": []`) {
		t.Fatalf("unexpected status JSON: %s", status.String())
	}

	root = New("test")
	var teardown bytes.Buffer
	root.SetOut(&teardown)
	root.SetArgs([]string{"dns", "teardown", "--cwd", repo, "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if teardown.String() != "{\n  \"removed\": true\n}\n" {
		t.Fatalf("unexpected teardown JSON: %s", teardown.String())
	}
}

func TestDNSSetupReportsMissingTools(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repo := t.TempDir()
	command := exec.Command("git", "-C", repo, "init", "-b", "main")
	command.Env = gitutil.Environment()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", output, err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".herdr-worktree.yaml"), []byte("ports:\n  services: [web]\nlocal_dns:\n  enabled: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(mustLookPath(t, "git")))
	root := New("test")
	root.SetArgs([]string{"dns", "setup", "--cwd", repo})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "required local DNS tools") {
		t.Fatalf("expected missing tool error, got %v", err)
	}
}

func mustLookPath(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Fatal(err)
	}
	return path
}
