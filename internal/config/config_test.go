package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
worktree_dir: ~/.herdr/worktrees
files:
  copy: [.env, global.txt]
  parallel: false
post_create: [global-command]
`)
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), `
agent: repo-agent
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
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %s: %v", strings.Join(args, " "), output, err)
	}
}
