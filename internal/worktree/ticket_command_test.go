package worktree

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/dkarter/hwt/internal/config"
)

func TestTicketCommandArgumentsSubstituteInputInPlace(t *testing.T) {
	configured := []string{"quick", "{input}", "--label={input}", "--json"}
	got, err := ticketCommandArguments(configured, "something; $(unsafe)")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"quick", "something; $(unsafe)", "--label=something; $(unsafe)", "--json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("arguments = %#v, want %#v", got, want)
	}
}

func TestTicketCommandArgumentsHandleMissingInput(t *testing.T) {
	got, err := ticketCommandArguments([]string{"issue", "search", "--json", "{input}"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"issue", "search", "--json"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("arguments = %#v, want %#v", got, want)
	}

	got, err = ticketCommandArguments([]string{"issue", "search", "--json"}, "unused")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"issue", "search", "--json"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("arguments without placeholder = %#v, want %#v", got, want)
	}

	_, err = ticketCommandArguments([]string{"issue", "search", "--query={input}"}, "")
	if err == nil || !strings.Contains(err.Error(), "requires a positional input") {
		t.Fatalf("embedded placeholder error = %v", err)
	}
}

func TestTicketOutputStringSelectsNestedString(t *testing.T) {
	got, err := ticketOutputString(map[string]any{"data": map[string]any{"git": map[string]any{"branch": "feature/test"}}}, "data.git.branch")
	if err != nil {
		t.Fatal(err)
	}
	if got != "feature/test" {
		t.Fatalf("selected value = %q", got)
	}
}

func TestRunTicketCommandMapsNestedOutput(t *testing.T) {
	ticket := config.TicketCommand{
		Command: []string{"sh", "-c", `printf '%s\n' '{"data":{"git":{"branch":"feature/mapped"},"issue":{"id":"ABC-123"}}}'`},
		Output: config.TicketCommandOutput{
			Branch:   "data.git.branch",
			Metadata: map[string]string{"identifier": "data.issue.id"},
		},
	}
	result, err := runTicketCommand(t.TempDir(), ticket, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.BranchName != "feature/mapped" || !reflect.DeepEqual(result.Metadata, map[string]string{"identifier": "ABC-123"}) {
		t.Fatalf("mapped result = %#v", result)
	}
}

func TestRunTicketCommandDistinguishesOmittedAndEmptyMetadataMappings(t *testing.T) {
	command := []string{"sh", "-c", `printf '%s\n' '{"branchName":"feature/test","metadata":{"identifier":"ABC-123"}}'`}
	for _, test := range []struct {
		name     string
		metadata map[string]string
		want     map[string]string
	}{
		{name: "omitted uses normalized metadata", want: map[string]string{"identifier": "ABC-123"}},
		{name: "empty disables metadata persistence", metadata: map[string]string{}, want: map[string]string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := runTicketCommand(t.TempDir(), config.TicketCommand{Command: command, Output: config.TicketCommandOutput{Metadata: test.metadata}}, "")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result.Metadata, test.want) {
				t.Fatalf("metadata = %#v, want %#v", result.Metadata, test.want)
			}
		})
	}
}

func TestRunTicketCommandRejectsInvalidMappedOutput(t *testing.T) {
	for _, test := range []struct {
		name   string
		output string
		config config.TicketCommandOutput
		want   string
	}{
		{name: "missing branch", output: `{}`, want: `field "branchName" is missing`},
		{name: "non-string branch", output: `{"branchName":42}`, want: "selected value must be a string"},
		{name: "empty branch", output: `{"branchName":""}`, want: "returned an empty string"},
		{name: "missing mapped metadata", output: `{"branchName":"feature/test"}`, config: config.TicketCommandOutput{Metadata: map[string]string{"identifier": "issue.id"}}, want: `field "issue" is missing`},
		{name: "non-string mapped metadata", output: `{"branchName":"feature/test","issue":{"id":42}}`, config: config.TicketCommandOutput{Metadata: map[string]string{"identifier": "issue.id"}}, want: "selected value must be a string"},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := []string{"sh", "-c", "printf '%s\\n' '" + test.output + "'"}
			_, err := runTicketCommand(t.TempDir(), config.TicketCommand{Command: command, Output: test.config}, "query")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestRunTicketCommandStreamsStderrWithInput(t *testing.T) {
	ticket := config.TicketCommand{Command: []string{
		"sh", "-c",
		`printf 'selecting %s\n' "$1" >&2; printf '%s\n' '{"branchName":"feature/selected"}'`,
		"hwt-ticket", "{input}",
	}}
	var terminalStderr bytes.Buffer

	result, err := runTicketCommandWithStderr(t.TempDir(), ticket, "authentication bug", &terminalStderr)
	if err != nil {
		t.Fatal(err)
	}
	if result.BranchName != "feature/selected" {
		t.Fatalf("selected branch = %q", result.BranchName)
	}
	if got := terminalStderr.String(); got != "selecting authentication bug\n" {
		t.Fatalf("streamed stderr = %q", got)
	}
}

func TestRunTicketCommandDoesNotRepeatStreamedFailureStderr(t *testing.T) {
	ticket := config.TicketCommand{Command: []string{"sh", "-c", `printf 'authentication required\n' >&2; exit 23`}}
	var terminalStderr bytes.Buffer

	_, err := runTicketCommandWithStderr(t.TempDir(), ticket, "query", &terminalStderr)
	if err == nil || strings.Contains(err.Error(), "authentication required") {
		t.Fatalf("failure error repeated streamed stderr: %v", err)
	}
	if got := terminalStderr.String(); got != "authentication required\n" {
		t.Fatalf("streamed stderr = %q", got)
	}
}
