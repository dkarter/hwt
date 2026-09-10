package preview

import (
	"errors"
	"strings"
	"testing"

	"github.com/dkarter/hwt/internal/config"
)

type fakeRunner struct {
	outputs []string
}

func (runner *fakeRunner) Run(_ string, _ ...string) ([]byte, error) {
	if len(runner.outputs) == 0 {
		return nil, errors.New("unexpected git call")
	}
	output := runner.outputs[0]
	runner.outputs = runner.outputs[1:]
	return []byte(output), nil
}

func fakeConfig(template string) func(string, ...string) (config.Config, config.Sources, error) {
	return func(string, ...string) (config.Config, config.Sources, error) {
		return config.Config{PreviewURL: template}, config.Sources{}, nil
	}
}

func TestResolveCurrentWorktree(t *testing.T) {
	runner := &fakeRunner{outputs: []string{"/worktrees/app-feature\n", "Feature/API v2\n", "worktree /projects/app\x00HEAD abc\x00branch refs/heads/main\x00"}}
	result, err := resolve(runner, fakeConfig("https://preview.example/{repository}/{branch}/{sanitized_branch}/{worktree}"), Options{CWD: "/worktrees/app-feature/subdir"})
	if err != nil {
		t.Fatal(err)
	}
	want := "https://preview.example/app/Feature%2FAPI%20v2/feature-api-v2/app-feature"
	if result.URL != want {
		t.Fatalf("URL = %q, want %q", result.URL, want)
	}
}

func TestResolveExplicitBranchDoesNotRequireLocalRef(t *testing.T) {
	runner := &fakeRunner{outputs: []string{"/worktrees/app-feature\n", "worktree /projects/app\x00"}}
	result, err := resolve(runner, fakeConfig("https://{sanitized_branch}.example/{repository}/{branch}"), Options{CWD: "/worktrees/app-feature", Branch: "other/new"})
	if err != nil {
		t.Fatal(err)
	}
	if result.URL != "https://other-new.example/app/other%2Fnew" {
		t.Fatalf("unexpected URL: %q", result.URL)
	}
}

func TestResolveErrors(t *testing.T) {
	tests := []struct {
		name     string
		outputs  []string
		branch   string
		template string
		want     string
	}{
		{name: "detached", outputs: []string{"/worktrees/app\n", "\n"}, template: "https://example.com/{branch}", want: "detached HEAD"},
		{name: "missing config", outputs: []string{"/worktrees/app\n"}, want: "preview_url is not configured"},
		{name: "explicit worktree", outputs: []string{"/worktrees/app\n"}, branch: "feature", template: "https://example.com/{worktree}", want: "unavailable for an explicit branch"},
		{name: "empty sanitized branch", outputs: []string{"/worktrees/app\n"}, branch: "///", template: "https://example.com/{sanitized_branch}", want: "unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := resolve(&fakeRunner{outputs: test.outputs}, fakeConfig(test.template), Options{CWD: "/worktrees/app", Branch: test.branch})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}
