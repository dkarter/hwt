package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCLI001_CLI002_CLI004_DIST001_DIST002_HelpVersionCompletionAndEmbeddedAssets(t *testing.T) {
	s := newSandbox(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--help"}, "Frictionless Herdr worktree orchestration"},
		{[]string{"--version"}, "hwt version 1.2.3"},
		{[]string{"completion", "zsh"}, "#compdef hwt"},
		{[]string{"skill"}, "hwt create"},
		{[]string{"skill", "config"}, "worktree_naming"},
	} {
		requireContains(t, s.run(s.root, tc.args...), tc.want)
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(s.run(s.root, "schema")), &schema); err != nil {
		t.Fatalf("schema is not JSON: %v", err)
	}
	if schema["$schema"] == nil {
		t.Fatalf("schema has no $schema: %#v", schema)
	}
}

func TestCFG001_CFG002_CFG003_CFG004_CFG005_CFG006_CFG007_ConfigLifecycle(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	global := filepath.Join(s.config, "hwt", "config.yaml")
	mustWrite(t, global, "worktree_prefix: global-\nfiles:\n  copy: [global.env]\n", 0o600)
	project := filepath.Join(repo, ".herdr-worktree.yaml")
	mustWrite(t, project, "worktree_naming: basename\nfiles:\n  copy: [before.env, <global>, after.env]\nports:\n  start: 31000\n  end: 31010\n", 0o600)
	canonicalProject, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.TrimSpace(s.run(repo, "config", "path")); got != canonicalProject {
		t.Fatalf("project path = %q", got)
	}
	if got := strings.TrimSpace(s.run(repo, "config", "path", "--global")); got != global {
		t.Fatalf("global path = %q", got)
	}
	shown := decode(t, s.run(repo, "config", "show"))
	config := shown["config"].(map[string]any)
	if config["worktree_prefix"] != "global-" || config["worktree_naming"] != "basename" {
		t.Fatalf("configuration was not merged: %#v", config)
	}
	copyEntries := config["files"].(map[string]any)["copy"].([]any)
	paths := make([]string, len(copyEntries))
	for i, entry := range copyEntries {
		paths[i] = entry.(map[string]any)["path"].(string)
	}
	if !slices.Equal(paths, []string{"before.env", "global.env", "after.env"}) {
		t.Fatalf("global list insertion = %#v", copyEntries)
	}
	requireContains(t, s.run(repo, "config", "validate"), "configuration is valid")

	mustWrite(t, project, "worktree_naming: impossible\n", 0o600)
	_, stderr, err := s.command(repo, "config", "validate")
	if err == nil || !strings.Contains(stderr, "must be full or basename") {
		t.Fatalf("invalid config error = %q, %v", stderr, err)
	}
	if err := os.Remove(project); err != nil {
		t.Fatal(err)
	}
	created := strings.TrimSpace(s.run(repo, "config", "init"))
	if created != canonicalProject || !strings.Contains(mustRead(t, project), "files:") {
		t.Fatalf("config init result = %q", created)
	}
	_, stderr, err = s.command(repo, "config", "init")
	if err == nil || !strings.Contains(stderr, "refusing to overwrite") {
		t.Fatalf("overwrite error = %q, %v", stderr, err)
	}
}
