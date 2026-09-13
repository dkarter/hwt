package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dkarter/hwt/internal/gitutil"
)

func TestLoadUsesGitCommonConfigWhenProjectConfigIsMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "main")
	path, err := GitCommonPath(repo)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, "worktree_prefix: local-\nfiles:\n  copy: [node_modules]\n")

	cfg, sources, err := Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorktreePrefix != "local-" || len(cfg.Files.Copy) != 1 || cfg.Files.Copy[0].Path != "node_modules" {
		t.Fatalf("unexpected Git-common config: %#v", cfg)
	}
	if sources.GitCommon != path || sources.Project != "" {
		t.Fatalf("unexpected config sources: %#v", sources)
	}
}

func TestLoadPrefersProjectConfigOverGitCommonConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "main")
	path, err := GitCommonPath(repo)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, "worktree_prefix: local-\n")
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yml"), "worktree_prefix: project-\n")

	cfg, sources, err := Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorktreePrefix != "project-" || sources.GitCommon != "" || sources.Project != filepath.Join(repo, ".herdr-worktree.yml") {
		t.Fatalf("project config did not take precedence: config=%#v sources=%#v", cfg, sources)
	}
}

func TestGitCommonPathIgnoresAmbientGitDirectory(t *testing.T) {
	target := t.TempDir()
	other := t.TempDir()
	runGit(t, target, "init", "-b", "main")
	runGit(t, other, "init", "-b", "main")
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))

	path, err := GitCommonPath(target)
	if err != nil {
		t.Fatal(err)
	}
	canonicalTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(canonicalTarget, ".git", "hwt", "config.yaml") {
		t.Fatalf("ambient GIT_DIR changed config path: %s", path)
	}
}

func TestLoadMergesGlobalAndProjectConfig(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	repo := t.TempDir()
	writeFile(t, filepath.Join(configHome, "hwt", "config.yaml"), `
agent: global-agent
ticket_command: [global-tickets, create, --json]
review_command: [global-review]
worktree_dir: ~/.herdr/worktrees
files:
  copy: [.env, global.txt]
  parallel: false
post_create: [global-command]
`)
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), `
agent: repo-agent
ticket_command: [project-tickets, quick]
review_command: [project-review, --local]
worktree_prefix: repo-
files:
  copy: [local.txt, <global>]
  parallel: true
post_create: [<global>, local-command]
`)

	cfg, sources, err := Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent != "repo-agent" || cfg.WorktreeDir != "~/.herdr/worktrees" || cfg.WorktreePrefix != "repo-" {
		t.Fatalf("unexpected scalar merge: %#v", cfg)
	}
	if !reflect.DeepEqual(cfg.TicketCommand, []string{"project-tickets", "quick"}) {
		t.Fatalf("project ticket command did not replace global command: %#v", cfg.TicketCommand)
	}
	if !reflect.DeepEqual(cfg.ReviewCommand, []string{"project-review", "--local"}) {
		t.Fatalf("project review command did not replace global command: %#v", cfg.ReviewCommand)
	}
	expectedCopy := []CopyEntry{
		{Path: "local.txt", Parallel: true},
		{Path: ".env", Parallel: true},
		{Path: "global.txt", Parallel: true},
	}
	if !reflect.DeepEqual(cfg.Files.Copy, expectedCopy) {
		t.Fatalf("unexpected copied files: %#v", cfg.Files.Copy)
	}
	if !cfg.Files.Parallel {
		t.Fatal("expected project parallel setting to override global setting")
	}
	if !reflect.DeepEqual(cfg.PostCreate, []string{"global-command", "local-command"}) {
		t.Fatalf("unexpected hooks: %#v", cfg.PostCreate)
	}
	if sources.Project != filepath.Join(repo, ".herdr-worktree.yaml") {
		t.Fatalf("unexpected project source: %s", sources.Project)
	}
}

func TestLoadDefaultsTicketCommandToLNR(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, _, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.TicketCommand, []string{"lnr", "quick", "--json"}) {
		t.Fatalf("unexpected default ticket command: %#v", cfg.TicketCommand)
	}
}

func TestLoadProvidesOverridableGitHubPullRequestURL(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	cfg, _, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URLs["pr"] != "https://{pr_host}/{pr_owner}/{pr_repository}/pull/{pr_number}" {
		t.Fatalf("unexpected default pull request URL: %q", cfg.URLs["pr"])
	}

	writeFile(t, filepath.Join(configHome, "hwt", "config.yaml"), "urls:\n  pr: https://gitlab.example/group/project/-/merge_requests?source_branch={branch}\n")
	cfg, _, err = Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URLs["pr"] != "https://gitlab.example/group/project/-/merge_requests?source_branch={branch}" {
		t.Fatalf("pull request URL was not overridden: %q", cfg.URLs["pr"])
	}
}

func TestLoadDefaultsReviewCommandToTuicr(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, _, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.ReviewCommand, []string{"tuicr"}) {
		t.Fatalf("unexpected default review command: %#v", cfg.ReviewCommand)
	}
}

func TestLoadUsesGlobalTicketCommand(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	writeFile(t, filepath.Join(configHome, "hwt", "config.yaml"), "ticket_command: [tickets, create, --json]\n")

	cfg, _, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.TicketCommand, []string{"tickets", "create", "--json"}) {
		t.Fatalf("unexpected global ticket command: %#v", cfg.TicketCommand)
	}
}

func TestLoadMergesNamedURLsAndMetadata(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	repo := t.TempDir()
	writeFile(t, filepath.Join(configHome, "hwt", "config.yaml"), "urls:\n  ticket: https://tickets.example/{ticket.identifier}\n  preview: https://global.example/{branch}\nmetadata:\n  values:\n    region: global\n    shared: global\n  commands:\n    deploy: [global-deploy, --json]\n")
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), "urls:\n  preview: https://{sanitized_branch}.project.example/{repository}\nmetadata:\n  values:\n    region: project\n  commands:\n    deploy: [project-deploy, --json]\n")

	cfg, _, err := Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URLs["ticket"] != "https://tickets.example/{ticket.identifier}" || cfg.URLs["preview"] != "https://{sanitized_branch}.project.example/{repository}" {
		t.Fatalf("unexpected URLs: %#v", cfg.URLs)
	}
	if !reflect.DeepEqual(cfg.Metadata.Values, map[string]string{"region": "project", "shared": "global"}) {
		t.Fatalf("unexpected metadata values: %#v", cfg.Metadata.Values)
	}
	if !reflect.DeepEqual(cfg.Metadata.Commands["deploy"], []string{"project-deploy", "--json"}) {
		t.Fatalf("unexpected metadata command: %#v", cfg.Metadata.Commands)
	}
}

func TestLoadRejectsInvalidNamedURL(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), "urls:\n  preview: https://example.com/{bad value}\n")

	_, _, err := Load(repo)
	if err == nil || !strings.Contains(err.Error(), "urls.preview: invalid placeholder") {
		t.Fatalf("expected named URL validation error, got %v", err)
	}
}

func TestLoadRejectsLegacyPreviewURL(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), "preview_url: https://example.com/{branch}\n")

	_, _, err := Load(repo)
	if err == nil || !strings.Contains(err.Error(), "field preview_url not found") {
		t.Fatalf("expected legacy preview_url rejection, got %v", err)
	}
}

func TestLoadRejectsReservedMetadataNames(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, contents := range []string{
		"metadata:\n  values:\n    branch: override\n",
		"metadata:\n  commands:\n    pr_number: [helper]\n",
	} {
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), contents)
		if _, _, err := Load(repo); err == nil || !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("expected reserved metadata rejection for %q, got %v", contents, err)
		}
	}
}

func TestLoadRejectsUnsupportedMetadataCommandPlaceholder(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), "metadata:\n  commands:\n    deploy: [helper, '{ticket.identifier}']\n")

	_, _, err := Load(repo)
	if err == nil || !strings.Contains(err.Error(), "unsupported placeholder") {
		t.Fatalf("expected command placeholder validation error, got %v", err)
	}
}

func TestLoadRejectsInvalidTicketCommand(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), "ticket_command: []\n")

	_, _, err := Load(repo)
	if err == nil || !strings.Contains(err.Error(), "must contain an executable") {
		t.Fatalf("expected ticket command validation error, got %v", err)
	}
}

func TestLoadRejectsInvalidReviewCommand(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), "review_command: []\n")

	_, _, err := Load(repo)
	if err == nil || !strings.Contains(err.Error(), "review_command must contain an executable") {
		t.Fatalf("expected review command validation error, got %v", err)
	}
}

func TestLoadResolvesPerEntryCopyOptions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), `
files:
  parallel: false
  copy_on_write: true
  copy:
    - inherited
    - path: overridden
      parallel: true
      copy_on_write: false
      symlink: true
`)

	cfg, _, err := Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	expected := []CopyEntry{
		{Path: "inherited", Parallel: false, CopyOnWrite: true},
		{Path: "overridden", Parallel: true, Symlink: true},
	}
	if !reflect.DeepEqual(cfg.Files.Copy, expected) {
		t.Fatalf("unexpected copy entries: %#v", cfg.Files.Copy)
	}
}

func TestLoadRejectsConflictingCopyStrategies(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), "files:\n  copy_on_write: true\n  copy:\n    - path: deps\n      symlink: true\n")

	_, _, err := Load(repo)
	if err == nil || !strings.Contains(err.Error(), "cannot enable both") {
		t.Fatalf("expected conflicting strategy error, got %v", err)
	}
}

func TestLoadDefaultsToParallelCopies(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, _, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Files.Parallel {
		t.Fatal("expected copies to be parallel by default")
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), "unknown: true\n")

	_, _, err := Load(repo)
	if err == nil || !strings.Contains(err.Error(), "field unknown not found") {
		t.Fatalf("expected strict YAML error, got %v", err)
	}
}

func TestLoadRejectsEscapingCopyPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), "files:\n  copy: [../secret]\n")

	_, _, err := Load(repo)
	if err == nil || !strings.Contains(err.Error(), "must not escape") {
		t.Fatalf("expected path validation error, got %v", err)
	}
}

func TestLoadRejectsGeneratedEnvironmentCopyPaths(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, path := range []string{".env.worktree", ".env.worktree/child"} {
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), "files:\n  copy: ["+path+"]\n")
		if _, _, err := Load(repo); err == nil || !strings.Contains(err.Error(), "generated .env.worktree") {
			t.Fatalf("expected generated environment copy rejection for %s, got %v", path, err)
		}
	}
}

func TestProjectPathRejectsAmbiguousExtensions(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), "{}\n")
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yml"), "{}\n")

	_, err := ProjectPath(repo)
	if err == nil {
		t.Fatal("expected ambiguous config error")
	}
}

func TestLoadRejectsGlobalMarkerInGlobalConfig(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	writeFile(t, filepath.Join(configHome, "hwt", "config.yaml"), "post_create: [<global>]\n")

	_, _, err := Load(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "cannot be used in the global config") {
		t.Fatalf("expected global marker error, got %v", err)
	}
}

func TestLoadRejectsMultipleDocuments(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), "worktree_naming: full\n---\nunknown: true\n")

	_, _, err := Load(repo)
	if err == nil || !strings.Contains(err.Error(), "multiple YAML documents") {
		t.Fatalf("expected multiple document error, got %v", err)
	}
}

func TestLoadResolvesPortsAndEnvironment(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), `
ports:
  start: 31000
  end: 31999
  services: [web, asset-server]
environment:
  variables:
    APP_URL: http://localhost:${HWT_PORT_WEB}
`)
	cfg, _, err := Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Ports.Start != 31000 || cfg.Ports.End != 31999 || !reflect.DeepEqual(cfg.Ports.Services, []string{"web", "asset-server"}) {
		t.Fatalf("unexpected ports: %#v", cfg.Ports)
	}
	if cfg.Environment.Variables["APP_URL"] != "http://localhost:${HWT_PORT_WEB}" {
		t.Fatalf("unexpected environment: %#v", cfg.Environment)
	}
}

func TestLoadResolvesLocalDNS(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	repo := t.TempDir()
	writeFile(t, filepath.Join(configHome, "hwt", "config.yaml"), "ports:\n  services: [web]\nlocal_dns:\n  enabled: true\n  domain: global.test\n  reload: [global-reload]\n")
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), "local_dns:\n  domain: dev.test\n  reload: [reload, '{caddyfile}', '{dnsmasq}']\n")

	cfg, _, err := Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.LocalDNS.Enabled || cfg.LocalDNS.Domain != "dev.test" || !reflect.DeepEqual(cfg.LocalDNS.Reload, []string{"reload", "{caddyfile}", "{dnsmasq}"}) {
		t.Fatalf("unexpected local DNS config: %#v", cfg.LocalDNS)
	}
}

func TestLoadDefaultsLocalDNSToDisabled(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, _, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LocalDNS.Enabled || cfg.LocalDNS.Domain != "hwt.test" {
		t.Fatalf("unexpected local DNS defaults: %#v", cfg.LocalDNS)
	}
}

func TestLoadRejectsInvalidLocalDNSReload(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), "local_dns:\n  reload: [reload, '{unknown}']\n")
	_, _, err := Load(repo)
	if err == nil || !strings.Contains(err.Error(), "unsupported placeholder") {
		t.Fatalf("expected local DNS reload validation error, got %v", err)
	}
}

func TestLocalDNSReloadReplacesGlobalAndCanBeCleared(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	writeFile(t, filepath.Join(configHome, "hwt", "config.yaml"), "local_dns:\n  reload: [global-reload]\n")
	for _, contents := range []string{"local_dns:\n  reload: [project-reload]\n", "local_dns:\n  reload: []\n"} {
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), contents)
		cfg, _, err := Load(repo)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.Join(cfg.LocalDNS.Reload, " "), "global") {
			t.Fatalf("project reload did not replace global argv: %#v", cfg.LocalDNS.Reload)
		}
	}
}

func TestLoadRejectsMalformedLocalDNSReloadPlaceholder(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), "local_dns:\n  reload: [reload, '{caddyfile']\n")
	if _, _, err := Load(repo); err == nil || !strings.Contains(err.Error(), "unclosed") {
		t.Fatalf("expected malformed reload placeholder error, got %v", err)
	}
}

func TestLoadRejectsInvalidLocalDNSDomain(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, domain := range []string{"localhost", "single", "bad_name.test", "-bad.test", "bad..test", "path/test"} {
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), "local_dns:\n  domain: "+domain+"\n")
		if _, _, err := Load(repo); err == nil || !strings.Contains(err.Error(), "local_dns.domain") {
			t.Fatalf("expected domain validation error for %q, got %v", domain, err)
		}
	}
}

func TestLoadRejectsInvalidPortConfiguration(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	tests := []string{
		"ports:\n  start: 40000\n  end: 30000\n",
		"ports:\n  services: [web-api, web_api]\n",
		"ports:\n  services: [" + strings.Repeat("a", 64) + "]\n",
		"environment:\n  variables:\n    HWT_PORT_WEB: override\n",
	}
	for _, contents := range tests {
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), contents)
		if _, _, err := Load(repo); err == nil {
			t.Fatalf("expected invalid configuration for %q", contents)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, cwd string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	command.Env = gitutil.Environment()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %s: %v", strings.Join(args, " "), output, err)
	}
}
