package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/dkarter/hwt/internal/worktree"
	"github.com/spf13/cobra"
)

type pluginContext struct {
	WorkspaceID    string `json:"workspace_id"`
	WorkspaceCWD   string `json:"workspace_cwd"`
	FocusedPaneCWD string `json:"focused_pane_cwd"`
}

func (a *app) herdrCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "herdr",
		Short: "Run interactive Herdr plugin workflows",
	}
	command.AddCommand(a.herdrCreateCommand(), a.herdrRemoveCommand())
	return command
}

func (a *app) herdrCreateCommand() *cobra.Command {
	noFocus := false
	command := &cobra.Command{
		Use:   "create",
		Short: "Interactively create a configured Herdr worktree",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			context, err := currentPluginContext()
			if err != nil {
				return err
			}
			cwd := context.FocusedPaneCWD
			if cwd == "" {
				cwd = context.WorkspaceCWD
			}
			root, err := repoRoot(cwd)
			if err != nil {
				return fmt.Errorf("current Herdr workspace is not in a Git repository: %w", err)
			}

			branch, ok, err := runTextPrompt(cmd.InOrStdin(), cmd.OutOrStdout(), "New Worktree", "Branch name", "feature/my-change")
			if err != nil || !ok {
				return err
			}
			branch = normalizeBranchInput(branch)
			if branch == "" {
				return nil
			}
			candidates, err := baseBranchCandidates(root)
			if err != nil {
				return err
			}
			base, ok, err := runBranchPicker(cmd.InOrStdin(), cmd.OutOrStdout(), candidates)
			if err != nil || !ok {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Creating %s from %s...\n", branch, base)
			result, err := worktree.Create(a.client(), worktree.CreateOptions{
				CWD:    root,
				Branch: branch,
				Base:   base,
			})
			if err != nil {
				return err
			}
			if !noFocus && os.Getenv("HWT_HERDR_NO_FOCUS") == "" {
				if _, err := a.client().Run("workspace", "focus", result.WorkspaceID); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Created %s at %s, but could not focus workspace %s: %v\n", result.Branch, result.Path, result.WorkspaceID, err)
					return nil
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created %s at %s\n", result.Branch, result.Path)
			return nil
		},
	}
	command.Flags().BoolVar(&noFocus, "no-focus", false, "do not focus the created workspace")
	return command
}

func (a *app) herdrRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove",
		Short: "Interactively remove the current Herdr worktree",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			context, err := currentPluginContext()
			if err != nil {
				return err
			}
			workspaceID := context.WorkspaceID
			if workspaceID == "" {
				workspaceID, err = a.client().CurrentWorkspaceID()
				if err != nil {
					return err
				}
			}
			workspace, err := a.client().Workspace(workspaceID)
			if err != nil {
				return err
			}
			if workspace.CheckoutPath == "" || !workspace.LinkedWorktree {
				return fmt.Errorf("current workspace is not a linked Herdr worktree")
			}
			label := workspace.Label
			if label == "" {
				label = workspaceID
			}

			confirmed, err := runConfirm(cmd.InOrStdin(), cmd.OutOrStdout(), fmt.Sprintf("Remove worktree %q?", label), workspace.CheckoutPath)
			if err != nil || !confirmed {
				return err
			}
			result, err := worktree.Remove(a.client(), worktree.RemoveOptions{WorkspaceID: workspaceID})
			if err == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", result.Path)
				return nil
			}
			if !worktree.CanForceRemove(err) {
				return err
			}

			question := "Force removal and discard uncommitted files?"
			if worktree.IsLocked(err) {
				question = "Force removal and override the worktree lock?"
			}
			confirmed, promptErr := runConfirm(cmd.InOrStdin(), cmd.OutOrStdout(), question, err.Error())
			if promptErr != nil || !confirmed {
				return promptErr
			}
			result, err = worktree.Remove(a.client(), worktree.RemoveOptions{WorkspaceID: workspaceID, Force: true})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", result.Path)
			return nil
		},
	}
}

func currentPluginContext() (pluginContext, error) {
	value := os.Getenv("HWT_HERDR_PLUGIN_CONTEXT_JSON")
	if value == "" {
		value = os.Getenv("HERDR_PLUGIN_CONTEXT_JSON")
	}
	if value == "" {
		return pluginContext{}, nil
	}
	var context pluginContext
	if err := json.Unmarshal([]byte(value), &context); err != nil {
		return pluginContext{}, fmt.Errorf("decode Herdr plugin context: %w", err)
	}
	return context, nil
}

func normalizeBranchInput(value string) string {
	return strings.Join(strings.Fields(value), "-")
}

func baseBranchCandidates(root string) ([]string, error) {
	current, err := gitCLIOutput(root, "branch", "--show-current")
	if err != nil {
		return nil, err
	}
	output, err := gitCLIOutput(root, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	locals := strings.Fields(output)
	localSet := make(map[string]bool, len(locals))
	for _, branch := range locals {
		localSet[branch] = true
	}

	main := ""
	if detected, detectErr := gitCLIOutput(root, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); detectErr == nil {
		detected = strings.TrimPrefix(detected, "origin/")
		if localSet[detected] {
			main = detected
		}
	}
	if main == "" {
		for _, candidate := range []string{"main", "master", "trunk", "develop"} {
			if localSet[candidate] {
				main = candidate
				break
			}
		}
	}
	if main == "" {
		main = current
	}

	ordered := make([]string, 0, len(locals))
	seen := make(map[string]bool, len(locals))
	for _, branch := range append([]string{main, current}, locals...) {
		if branch != "" && !seen[branch] {
			ordered = append(ordered, branch)
			seen[branch] = true
		}
	}
	return ordered, nil
}

func gitCLIOutput(root string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
}
