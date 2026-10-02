package worktree

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dkarter/hwt/internal/config"
	"github.com/dkarter/hwt/internal/herdr"
)

// StaleWorktreeError describes a record that cannot safely be reused. Repair is
// provided only for a confirmed missing, unlocked linked checkout.
type StaleWorktreeError struct {
	Code   string   `json:"error"`
	Branch string   `json:"branch"`
	Path   string   `json:"path"`
	Repair []string `json:"repair,omitempty"`
}

func (e *StaleWorktreeError) Error() string {
	message := fmt.Sprintf("stale worktree for branch %q at %s", e.Branch, e.Path)
	if len(e.Repair) == 0 {
		return message + "; inspect the recorded checkout and any lock before repairing it manually"
	}
	quoted := make([]string, len(e.Repair))
	for i, arg := range e.Repair {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	}
	return message + "; to remove only this missing record, run " + strings.Join(quoted, " ") + ", then retry creation"
}

func reuseExisting(client Client, source string, options CreateOptions, cfg config.Config, sources config.Sources) (CreateResult, bool, error) {
	items, err := client.Worktrees(source)
	if err != nil {
		return CreateResult{}, false, fmt.Errorf("inspect Herdr worktrees: %w", err)
	}
	var matches []herdr.Worktree
	for _, item := range items {
		if item.Branch == options.Branch {
			matches = append(matches, item)
		}
	}
	if len(matches) == 0 {
		return CreateResult{}, false, nil
	}
	if len(matches) != 1 {
		return CreateResult{}, true, fmt.Errorf("branch %q has multiple recorded worktrees; inspect them before retrying", options.Branch)
	}
	item := matches[0]
	fail := func(err error) (CreateResult, bool, error) { return CreateResult{}, true, err }
	if !item.Linked || samePath(item.Path, source) {
		return fail(fmt.Errorf("branch %q is checked out in the primary checkout at %s; refusing reuse", options.Branch, item.Path))
	}
	if item.Detached || item.Path == "" {
		return fail(fmt.Errorf("invalid recorded checkout for branch %q at %s; refusing reuse", options.Branch, item.Path))
	}
	info, statErr := os.Lstat(item.Path)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fail(fmt.Errorf("inspect worktree path %s: %w", item.Path, statErr))
	}
	if item.Prunable || errors.Is(statErr, os.ErrNotExist) {
		stale := &StaleWorktreeError{Code: "stale_worktree", Branch: options.Branch, Path: item.Path}
		if errors.Is(statErr, os.ErrNotExist) {
			canRepair, err := missingRecordRepairable(source, item)
			if err != nil {
				return fail(err)
			}
			if canRepair {
				stale.Repair = []string{"git", "-C", source, "worktree", "remove", "--force", "--", item.Path}
			}
		}
		return fail(stale)
	}
	if !info.IsDir() {
		return fail(fmt.Errorf("recorded checkout %s is not a directory; refusing reuse", item.Path))
	}
	gitDir, common, root, err := metadata(item.Path)
	if err != nil {
		return fail(fmt.Errorf("verify existing worktree %s: %w", item.Path, err))
	}
	sourceCommon, err := gitOutput(source, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return fail(err)
	}
	branch, err := gitOutput(item.Path, "branch", "--show-current")
	if err != nil {
		return fail(err)
	}
	if samePath(gitDir, common) || !samePath(common, sourceCommon) || !samePath(root, item.Path) || branch != options.Branch {
		return fail(fmt.Errorf("checkout %s is not a linked worktree of this repository on branch %q; refusing reuse", item.Path, options.Branch))
	}
	if options.Path != "" {
		path, err := absoluteFrom(source, options.Path)
		if err != nil {
			return fail(err)
		}
		if !samePath(path, item.Path) {
			return fail(fmt.Errorf("--path %s conflicts with existing checkout %s", path, item.Path))
		}
	}
	if !options.Reuse && !options.Focus {
		return fail(fmt.Errorf("branch %q is already checked out at %s; use --reuse or --focus", options.Branch, item.Path))
	}
	base, err := recordedBase(source, options.Branch)
	if err != nil {
		return fail(err)
	}
	result := CreateResult{Path: item.Path, Branch: options.Branch, Base: base, Agent: cfg.Agent, Config: sources, ReusedWorktree: true, ReusedWorkspace: item.OpenWorkspaceID != ""}
	if item.OpenWorkspaceID != "" {
		workspace, err := client.Workspace(item.OpenWorkspaceID)
		if err != nil {
			return fail(err)
		}
		if workspace.ID != item.OpenWorkspaceID || !workspace.LinkedWorktree || !samePath(workspace.CheckoutPath, item.Path) {
			return fail(fmt.Errorf("workspace %s is not associated with checkout %s", item.OpenWorkspaceID, item.Path))
		}
		panes, err := client.Panes(workspace.ID)
		if err != nil {
			return fail(err)
		}
		for _, pane := range panes {
			if pane.ID != "" && pane.WorkspaceID == workspace.ID {
				result.PaneID = pane.ID
				break
			}
		}
		if result.PaneID == "" {
			return fail(fmt.Errorf("reused workspace %s has no panes", workspace.ID))
		}
		result.WorkspaceID = workspace.ID
	} else {
		args := []string{"--cwd", source, "--path", item.Path, "--no-focus", "--json"}
		if options.Label != "" {
			args = append(args, "--label", options.Label)
		}
		opened, err := client.Open(args...)
		if err != nil {
			return fail(fmt.Errorf("open existing worktree: %w", err))
		}
		if opened.WorkspaceID == "" || opened.PaneID == "" || !samePath(opened.Path, item.Path) {
			return fail(errors.New("Herdr returned an invalid reused workspace"))
		}
		result.WorkspaceID, result.PaneID = opened.WorkspaceID, opened.PaneID
	}
	// Reuse must never enter creation's destructive rollback or preparation flow.
	result.Environment, err = prepareEnvironmentMode(item.Path, cfg, false, true)
	if err != nil {
		return fail(err)
	}
	if options.Focus {
		if _, err := client.Run("workspace", "focus", result.WorkspaceID); err != nil {
			return fail(fmt.Errorf("focus reused workspace: %w", err))
		}
	}
	return result, true, nil
}

func recordedBase(source, branch string) (string, error) {
	base, err := gitOutput(source, "config", "--get", "branch."+branch+".herdr-base")
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return "", nil
	}
	return base, err
}

func missingRecordRepairable(source string, item herdr.Worktree) (bool, error) {
	output, err := gitOutput(source, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return false, fmt.Errorf("inspect missing worktree record: %w", err)
	}
	path := item.Path
	if parent, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		path = filepath.Join(parent, filepath.Base(path))
	}
	for index, record := range strings.Split(output, "\x00\x00") {
		fields := strings.Split(record, "\x00")
		if index == 0 || len(fields) == 0 || fields[0] != "worktree "+path {
			continue
		}
		branchMatches := false
		for _, field := range fields[1:] {
			if field == "locked" || strings.HasPrefix(field, "locked ") {
				return false, nil
			}
			branchMatches = branchMatches || field == "branch refs/heads/"+item.Branch
		}
		return branchMatches, nil
	}
	return false, nil
}
