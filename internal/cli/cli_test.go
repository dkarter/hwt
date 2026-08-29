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

func TestPluginCommandsForwardToHerdr(t *testing.T) {
	tests := []struct {
		version string
		command []string
		want    string
	}{
		{version: "test", command: []string{"plugin", "install"}, want: "plugin\ninstall\ndkarter/hwt/plugins/herdr\n--yes\n"},
		{version: "1.2.3", command: []string{"plugin", "update"}, want: "plugin\ninstall\ndkarter/hwt/plugins/herdr\n--ref\nv1.2.3\n--yes\n"},
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
