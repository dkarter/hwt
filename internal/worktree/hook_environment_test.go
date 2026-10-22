package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dkarter/hwt/internal/herdr"
)

// Check presence as well as value: supported fields are always set, while
// unknown ambient ticket fields must not leak into the hook.
const ticketHookProbe = `printf '%s\n' "${HWT_TICKET_IDENTIFIER-unset}" "${HWT_TICKET_TITLE-unset}" "${HWT_TICKET_URL-unset}" "${HWT_TICKET_CUSTOM-unset}"`

func contaminateTicketEnvironment(t *testing.T) {
	t.Helper()
	for _, field := range []string{"IDENTIFIER", "TITLE", "URL", "CUSTOM"} {
		t.Setenv("HWT_TICKET_"+field, "stale")
	}
}

func TestRemoveHooksTicketEnvironment(t *testing.T) {
	for _, async := range []bool{false, true} {
		for _, assignment := range []string{"assigned", "partial", "unassigned"} {
			t.Run(fmt.Sprintf("%s/async=%t", assignment, async), func(t *testing.T) {
				t.Setenv("XDG_CONFIG_HOME", t.TempDir())
				t.Setenv("XDG_STATE_HOME", t.TempDir())
				t.Setenv("XDG_CACHE_HOME", t.TempDir())
				t.Setenv("HOME", t.TempDir())
				contaminateTicketEnvironment(t)
				repo := initRepo(t)
				checkout := filepath.Join(t.TempDir(), "ticket-hooks")
				// A ticket-looking branch must never imply assignment.
				run(t, repo, "git", "worktree", "add", "-b", "ABC-999-unrelated", checkout, "main")
				ticket := map[string]string(nil)
				want := "\n\n\nunset\n"
				if assignment != "unassigned" {
					ticket = map[string]string{"identifier": "TKT-42", "custom": "not exported"}
					want = "TKT-42\n\n\nunset\n"
					if assignment == "assigned" {
						ticket["title"] = "Fix 'quotes' $(not-a-command)\nand newlines"
						ticket["url"] = "https://tickets.example/TKT-42?a=1&b=2"
						want = ticket["identifier"] + "\n" + ticket["title"] + "\n" + ticket["url"] + "\nunset\n"
					}
				}
				if err := writeTicketMetadata(checkout, ticket); err != nil {
					t.Fatal(err)
				}
				gitDir, _, _, err := metadata(checkout)
				if err != nil {
					t.Fatal(err)
				}
				pre := filepath.Join(t.TempDir(), "pre")
				post := filepath.Join(t.TempDir(), "post")
				// Hold async hooks until post_remove proves metadata is gone.
				gate := filepath.Join(t.TempDir(), "gate")
				defer func() { _ = os.WriteFile(gate, nil, 0o600) }()
				wait := ""
				if async {
					wait = fmt.Sprintf("while [ ! -f %q ]; do sleep 0.05; done; ", gate)
				}
				preCommand := wait + ticketHookProbe + fmt.Sprintf(" > %q", pre)
				postCommand := fmt.Sprintf("test ! -e %q && touch %q && ", gitDir, gate) + ticketHookProbe + fmt.Sprintf(" > %q", post)
				write(t, filepath.Join(repo, ".herdr-worktree.yaml"), fmt.Sprintf("pre_remove:\n  - cmd: %q\n    async: %t\npost_remove:\n  - cmd: %q\n    async: %t\n", preCommand, async, postCommand, async))
				client := &fakeClient{workspace: herdr.Workspace{ID: "w-ticket", CheckoutPath: checkout, LinkedWorktree: true}}
				result, err := Remove(client, RemoveOptions{WorkspaceID: "w-ticket"})
				if err != nil {
					t.Fatal(err)
				}
				if async && len(result.AsyncLogs) != 2 {
					t.Fatalf("expected two async hooks, got %#v", result)
				}
				for _, path := range []string{pre, post} {
					waitFor(t, func() bool { data, err := os.ReadFile(path); return err == nil && string(data) == want })
					assertFile(t, path, want)
				}
			})
		}
	}
}

func TestCreateHooksTicketEnvironment(t *testing.T) {
	for _, assigned := range []bool{false, true} {
		t.Run(fmt.Sprintf("assigned=%t", assigned), func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			contaminateTicketEnvironment(t)
			repo := initRepo(t)
			checkout := filepath.Join(t.TempDir(), "created")
			write(t, filepath.Join(repo, ".herdr-worktree.yaml"), fmt.Sprintf("post_create:\n  - %q\nticket_commands:\n  default:\n    command: [sh, -c, %q]\n", ticketHookProbe+" > hook-ticket", `printf '%s' '{"branchName":"TKT-42","metadata":{"identifier":"TKT-42","title":"A title","url":"https://tickets.example/42"}}'`))
			client := &fakeClient{createFn: func(_ ...string) (herdr.Created, error) {
				run(t, repo, "git", "worktree", "add", "-b", "TKT-42", checkout, "main")
				return herdr.Created{WorkspaceID: "w-create", Path: checkout}, nil
			}}
			options := CreateOptions{CWD: repo, Branch: "TKT-42", Base: "main"}
			want := "\n\n\nunset\n"
			if assigned {
				options.Branch, options.Ticket = "", "default"
				want = "TKT-42\nA title\nhttps://tickets.example/42\nunset\n"
			}
			result, err := Create(client, options)
			if err != nil {
				t.Fatal(err)
			}
			assertFile(t, filepath.Join(checkout, "hook-ticket"), want)
			data, err := os.ReadFile(result.Environment.Path)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"HWT_TICKET_IDENTIFIER", "HWT_TICKET_TITLE", "HWT_TICKET_URL"} {
				if _, ok := result.Environment.Variables[name]; ok || strings.Contains(string(data), name) {
					t.Fatalf("hook-only variable %s persisted in worktree environment", name)
				}
			}
		})
	}
}
