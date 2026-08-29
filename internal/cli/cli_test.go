package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dkarter/hwt/skills"
)

func TestConfigInitGitCommonRefusesExistingYMLConfig(t *testing.T) {
	repo := t.TempDir()
	command := exec.Command("git", "-C", repo, "init", "-b", "main")
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
