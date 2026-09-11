package pullrequest

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type response struct {
	output string
	err    error
}

type fakeRunner struct {
	responses []response
	calls     [][]string
}

func (f *fakeRunner) Run(_ string, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	response := f.responses[0]
	f.responses = f.responses[1:]
	return []byte(response.output), response.err
}

func TestResolveUsesCurrentBranchAndGitHubAPI(t *testing.T) {
	commands := &fakeRunner{responses: []response{
		{output: "true\n"},
		{output: "feature/current\n"},
		{output: "origin\n"},
		{output: `{"url":"https://github.com/acme/app/pull/42"}`},
	}}

	result, err := resolve(commands, Options{CWD: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	if result.URL != "https://github.com/acme/app/pull/42" {
		t.Fatalf("unexpected URL: %q", result.URL)
	}
	want := []string{"gh", "pr", "view", "feature/current", "--json", "url"}
	if !reflect.DeepEqual(commands.calls[3], want) {
		t.Fatalf("GitHub call = %#v, want %#v", commands.calls[3], want)
	}
}

func TestResolveExplicitBranchAndRepositorySupportsForkPR(t *testing.T) {
	commands := &fakeRunner{responses: []response{
		{output: "true\n"},
		{output: `{"url":"https://github.com/upstream/app/pull/7"}`},
	}}

	result, err := resolve(commands, Options{CWD: "/repo", Branch: "contributor:feature", Repository: "upstream/app"})
	if err != nil {
		t.Fatal(err)
	}
	if result.URL != "https://github.com/upstream/app/pull/7" {
		t.Fatalf("unexpected URL: %q", result.URL)
	}
	want := []string{"gh", "pr", "view", "contributor:feature", "--json", "url", "--repo", "upstream/app"}
	if !reflect.DeepEqual(commands.calls[1], want) {
		t.Fatalf("GitHub call = %#v, want %#v", commands.calls[1], want)
	}
}

func TestResolveNumberUsesExistingLookup(t *testing.T) {
	commands := &fakeRunner{responses: []response{
		{output: "true\n"},
		{output: `{"number":42}`},
	}}

	number, err := resolveNumber(commands, Options{CWD: "/repo", Branch: "feature", Repository: "acme/app"})
	if err != nil {
		t.Fatal(err)
	}
	if number != 42 {
		t.Fatalf("number = %d, want 42", number)
	}
	want := []string{"gh", "pr", "view", "feature", "--json", "number", "--repo", "acme/app"}
	if !reflect.DeepEqual(commands.calls[1], want) {
		t.Fatalf("GitHub call = %#v, want %#v", commands.calls[1], want)
	}
}

func TestResolveNumberRejectsMissingNumber(t *testing.T) {
	commands := &fakeRunner{responses: []response{{output: "true\n"}, {output: `{}`}}}
	_, err := resolveNumber(commands, Options{CWD: "/repo", Branch: "feature", Repository: "acme/app"})
	if err == nil || !strings.Contains(err.Error(), "returned no number") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveActionableErrors(t *testing.T) {
	tests := []struct {
		name      string
		options   Options
		responses []response
		want      string
	}{
		{name: "detached head", options: Options{CWD: "/repo"}, responses: []response{{output: "true"}, {output: "\n"}}, want: "detached HEAD"},
		{name: "missing remote", options: Options{CWD: "/repo", Branch: "feature"}, responses: []response{{output: "true"}, {output: "\n"}}, want: "no Git remotes"},
		{name: "ambiguous remotes", options: Options{CWD: "/repo", Branch: "feature"}, responses: []response{{output: "true"}, {output: "origin\nupstream\n"}}, want: "--repo OWNER/REPO"},
		{name: "missing URL", options: Options{CWD: "/repo", Branch: "feature", Repository: "acme/app"}, responses: []response{{output: "true"}, {output: `{}`}}, want: "returned no URL"},
		{name: "missing pull request", options: Options{CWD: "/repo", Branch: "feature", Repository: "acme/app"}, responses: []response{{output: "true"}, {err: errors.New("no pull requests found for branch")}}, want: "no pull requests found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := resolve(&fakeRunner{responses: test.responses}, test.options)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}
