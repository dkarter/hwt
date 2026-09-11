package herdr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClientDecodesReviewLifecycleResponses(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "herdr")
	script := `#!/bin/sh
case "$1 $2" in
  "pane process-info") printf '%s' '{"result":{"process_info":{"foreground_processes":[{"name":"tuicr","argv0":"tuicr","argv":["tuicr"]}]}}}' ;;
  "pane split") printf '%s' '{"result":{"pane":{"pane_id":"w1:p2","workspace_id":"w1"}}}' ;;
  "worktree list") printf '%s' '{"result":{"worktrees":[{"branch":"hwt/review/pr-7","path":"/review","is_linked_worktree":true,"is_detached":false,"open_workspace_id":"w7"}]}}' ;;
  "worktree open") printf '%s' '{"result":{"workspace":{"workspace_id":"w7"},"root_pane":{"pane_id":"w7:p1"},"worktree":{"path":"/review"}}}' ;;
  "pane list") printf '%s' '{"result":{"panes":[{"pane_id":"w7:p1","workspace_id":"w7"}]}}' ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	client := Client{Binary: bin}

	worktrees, err := client.Worktrees("/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(worktrees) != 1 || worktrees[0].Branch != "hwt/review/pr-7" || worktrees[0].OpenWorkspaceID != "w7" || !worktrees[0].Linked {
		t.Fatalf("unexpected worktrees: %#v", worktrees)
	}
	opened, err := client.Open("--cwd", "/repo", "--path", "/review", "--no-focus", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if opened.WorkspaceID != "w7" || opened.PaneID != "w7:p1" || opened.Path != "/review" {
		t.Fatalf("unexpected open result: %#v", opened)
	}
	panes, err := client.Panes("w7")
	if err != nil {
		t.Fatal(err)
	}
	if len(panes) != 1 || panes[0].ID != "w7:p1" || panes[0].WorkspaceID != "w7" {
		t.Fatalf("unexpected panes: %#v", panes)
	}
	processes, err := client.ProcessInfo("w7:p1")
	if err != nil || len(processes) != 1 || processes[0].Argv0 != "tuicr" {
		t.Fatalf("unexpected processes: %#v, error: %v", processes, err)
	}
	pane, err := client.Split("w7:p1", "/review")
	if err != nil || pane.ID != "w1:p2" {
		t.Fatalf("unexpected split pane: %#v, error: %v", pane, err)
	}
}
