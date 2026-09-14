package pullrequest

import (
	"errors"
	"fmt"
	"os/exec"
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

func TestResolveRepositoryThroughGitHub(t *testing.T) {
	commands := &fakeRunner{responses: []response{
		{output: "origin\n"},
		{output: `{"url":"https://github.com/acme/app"}`},
	}}
	reference, err := resolveRepository(commands, Options{CWD: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	want := RepositoryReference{Host: "github.com", Owner: "acme", Repository: "app"}
	if reference != want {
		t.Fatalf("reference = %#v, want %#v", reference, want)
	}
	wantCalls := [][]string{{"git", "remote"}, {"gh", "repo", "view", "--json", "url"}}
	if !reflect.DeepEqual(commands.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", commands.calls, wantCalls)
	}
}

func TestResolveRepositoryUsesExplicitRepository(t *testing.T) {
	commands := &fakeRunner{responses: []response{{output: `{"url":"https://github.example/acme/app"}`}}}
	reference, err := resolveRepository(commands, Options{CWD: "/repo", Repository: "github.example/acme/app"})
	if err != nil {
		t.Fatal(err)
	}
	want := RepositoryReference{Host: "github.example", Owner: "acme", Repository: "app"}
	if reference != want {
		t.Fatalf("reference = %#v, want %#v", reference, want)
	}
	wantCall := []string{"gh", "repo", "view", "github.example/acme/app", "--json", "url"}
	if !reflect.DeepEqual(commands.calls[0], wantCall) {
		t.Fatalf("GitHub call = %#v, want %#v", commands.calls[0], wantCall)
	}
}

func TestResolveRepositoryWithoutRemotesIsUnavailable(t *testing.T) {
	commands := &fakeRunner{responses: []response{{output: ""}}}
	_, err := resolveRepository(commands, Options{CWD: "/repo"})
	if !errors.Is(err, ErrRepositoryNotFound) {
		t.Fatalf("error = %v", err)
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

func TestResolveReferenceUsesCanonicalPullRequestURL(t *testing.T) {
	reference, err := referenceFromURL("https://github.example/acme/app/pull/42")
	if err != nil {
		t.Fatal(err)
	}
	want := Reference{Host: "github.example", Owner: "acme", Repository: "app", Number: 42}
	if !reflect.DeepEqual(reference, want) {
		t.Fatalf("reference = %#v, want %#v", reference, want)
	}
}

func TestResolveMetadata(t *testing.T) {
	title := "Fix $(touch /tmp/nope); `whoami`"
	head := "feature; echo nope"
	commands := &fakeRunner{responses: []response{{output: `{"number":42,"url":"https://github.com/acme/app/pull/42","title":"Fix $(touch /tmp/nope); ` + "`whoami`" + `","headRefName":"feature; echo nope","headRefOid":"abc123","baseRefName":"main","isCrossRepository":true}`}}}

	metadata, err := resolveMetadata(commands, Options{CWD: "/repo", Branch: "https://github.com/acme/app/pull/42", Repository: "acme/app"})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Number != 42 || metadata.URL != "https://github.com/acme/app/pull/42" || metadata.Title != title || metadata.HeadRefName != head || metadata.HeadRefOID != "abc123" || metadata.BaseRefName != "main" || !metadata.IsCrossRepository {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
	want := []string{"gh", "pr", "view", "https://github.com/acme/app/pull/42", "--json", "number,url,title,headRefName,headRefOid,baseRefName,isCrossRepository", "--repo", "acme/app"}
	if !reflect.DeepEqual(commands.calls, [][]string{want}) {
		t.Fatalf("calls = %#v, want %#v", commands.calls, [][]string{want})
	}
}

func TestResolveMetadataWithoutRepository(t *testing.T) {
	commands := &fakeRunner{responses: []response{{output: `{"number":7,"url":"https://github.example.com/upstream/app/pull/7","title":"Title","headRefName":"fork","headRefOid":"def456","baseRefName":"main","isCrossRepository":false}`}}}
	_, err := resolveMetadata(commands, Options{CWD: "/repo", Branch: "https://github.example.com/upstream/app/pull/7"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"gh", "pr", "view", "https://github.example.com/upstream/app/pull/7", "--json", "number,url,title,headRefName,headRefOid,baseRefName,isCrossRepository"}
	if !reflect.DeepEqual(commands.calls, [][]string{want}) {
		t.Fatalf("calls = %#v, want %#v", commands.calls, [][]string{want})
	}
}

func TestResolveMetadataByNumberUsesCanonicalPullRequestURL(t *testing.T) {
	commands := &fakeRunner{responses: []response{{output: `{"number":42,"url":"https://github.com/acme/app/pull/42","title":"Title","headRefName":"feature","headRefOid":"abc123","baseRefName":"main","isCrossRepository":false}`}}}

	metadata, err := resolveMetadata(commands, Options{CWD: "/repo", Branch: "42"})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.URL != "https://github.com/acme/app/pull/42" {
		t.Fatalf("canonical URL = %q", metadata.URL)
	}
	want := []string{"gh", "pr", "view", "42", "--json", "number,url,title,headRefName,headRefOid,baseRefName,isCrossRepository"}
	if !reflect.DeepEqual(commands.calls, [][]string{want}) {
		t.Fatalf("calls = %#v, want %#v", commands.calls, [][]string{want})
	}
}

func TestIsSelectorAcceptsURLsAndCanonicalPositiveNumbers(t *testing.T) {
	tests := map[string]bool{
		"https://github.com/acme/app/pull/42": true,
		"42":                                  true,
		"0":                                   false,
		"042":                                 false,
		"feature/42":                          false,
	}
	for value, want := range tests {
		if got := IsSelector(value); got != want {
			t.Errorf("IsSelector(%q) = %t, want %t", value, got, want)
		}
	}
}

func TestResolveMetadataRejectsMalformedURLBeforeRunningCommands(t *testing.T) {
	values := []string{
		"", "http://github.com/acme/app/pull/1", "https://user@github.com/acme/app/pull/1",
		"https://github.com:443/acme/app/pull/1", "https://github.com:/acme/app/pull/1", "https://github.com/acme/app/pull/1?x=y",
		"https://github.com/acme/app/pull/1#fragment", "https://github.com/acme/app/pull/1/files",
		"https://github.com/acme/app/issues/1", "https://github.com/acme/app/pull/nope",
	}
	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			commands := &fakeRunner{}
			_, err := resolveMetadata(commands, Options{CWD: "/repo", Branch: value})
			if err == nil {
				t.Fatal("expected error")
			}
			if len(commands.calls) != 0 {
				t.Fatalf("unexpected calls: %#v", commands.calls)
			}
		})
	}
}

func TestResolveMetadataValidatesResponse(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "missing field", output: `{"number":42,"url":"https://github.com/acme/app/pull/42"}`, want: "incomplete"},
		{name: "different repository", output: `{"number":42,"url":"https://github.com/other/app/pull/42","title":"Title","headRefName":"head","headRefOid":"abc","baseRefName":"main","isCrossRepository":false}`, want: "different host, repository, or number"},
		{name: "invalid canonical URL", output: `{"number":42,"url":"http://github.com/acme/app/pull/42","title":"Title","headRefName":"head","headRefOid":"abc","baseRefName":"main","isCrossRepository":false}`, want: "invalid canonical"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			commands := &fakeRunner{responses: []response{{output: test.output}}}
			_, err := resolveMetadata(commands, Options{CWD: "/repo", Branch: "https://github.com/acme/app/pull/42"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestResolveMetadataErrorsAreActionable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "missing gh", err: fmt.Errorf("wrapped: %w", exec.ErrNotFound), want: "install it"},
		{name: "authentication", err: errors.New("HTTP 401"), want: "gh auth login"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			commands := &fakeRunner{responses: []response{{err: test.err}}}
			_, err := resolveMetadata(commands, Options{CWD: "/repo", Branch: "https://github.com/acme/app/pull/42"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
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

func TestResolveClassifiesMissingPullRequest(t *testing.T) {
	commands := &fakeRunner{responses: []response{{output: "true"}, {err: errors.New("no pull requests found for branch \"main\"")}}}
	_, err := resolve(commands, Options{CWD: "/repo", Branch: "main", Repository: "acme/app"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}
