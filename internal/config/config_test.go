package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadMergesGlobalAndProjectConfig(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	repo := t.TempDir()
	writeFile(t, filepath.Join(configHome, "hwt", "config.yaml"), `
agent: global-agent
worktree_dir: ~/.herdr/worktrees
files:
  copy: [.env, global.txt]
post_create: [global-command]
`)
	writeFile(t, filepath.Join(repo, ".herdr-worktree.yaml"), `
agent: repo-agent
worktree_prefix: repo-
files:
  copy: [local.txt, <global>]
post_create: [<global>, local-command]
`)

	cfg, sources, err := Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent != "repo-agent" || cfg.WorktreeDir != "~/.herdr/worktrees" || cfg.WorktreePrefix != "repo-" {
		t.Fatalf("unexpected scalar merge: %#v", cfg)
	}
	if !reflect.DeepEqual(cfg.Files.Copy, []string{"local.txt", ".env", "global.txt"}) {
		t.Fatalf("unexpected copied files: %#v", cfg.Files.Copy)
	}
	if !reflect.DeepEqual(cfg.PostCreate, []string{"global-command", "local-command"}) {
		t.Fatalf("unexpected hooks: %#v", cfg.PostCreate)
	}
	if sources.Project != filepath.Join(repo, ".herdr-worktree.yaml") {
		t.Fatalf("unexpected project source: %s", sources.Project)
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
