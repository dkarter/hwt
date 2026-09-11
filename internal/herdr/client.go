package herdr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Client struct {
	Binary string
}

type Created struct {
	WorkspaceID string
	PaneID      string
	Path        string
	Raw         json.RawMessage
}

type Workspace struct {
	ID             string
	Label          string
	CheckoutPath   string
	LinkedWorktree bool
}

type Worktree struct {
	Branch          string
	Path            string
	Linked          bool
	Detached        bool
	OpenWorkspaceID string
}

type Pane struct {
	ID          string
	WorkspaceID string
}

type Process struct {
	Name  string
	Argv0 string
	Argv  []string
}

func (c Client) Run(args ...string) ([]byte, error) {
	cmd := exec.Command(c.Binary, args...)
	cmd.Stdin = os.Stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		return nil, fmt.Errorf("%s %s failed: %s: %w", c.Binary, strings.Join(args, " "), message, err)
	}
	return stdout.Bytes(), nil
}

func (c Client) SourceCheckout(cwd string) (string, error) {
	data, err := c.Run("worktree", "list", "--cwd", cwd, "--json")
	if err != nil {
		return "", err
	}
	var response struct {
		Result struct {
			Source struct {
				Path string `json:"source_checkout_path"`
			} `json:"source"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return "", fmt.Errorf("decode Herdr worktree list response: %w", err)
	}
	if response.Result.Source.Path == "" {
		return "", fmt.Errorf("Herdr did not return a source checkout path")
	}
	return response.Result.Source.Path, nil
}

func (c Client) Create(args ...string) (Created, error) {
	return c.created("create", args...)
}

func (c Client) Open(args ...string) (Created, error) {
	return c.created("open", args...)
}

func (c Client) created(action string, args ...string) (Created, error) {
	data, err := c.Run(append([]string{"worktree", action}, args...)...)
	if err != nil {
		return Created{}, err
	}
	var response struct {
		Result struct {
			Workspace struct {
				ID string `json:"workspace_id"`
			} `json:"workspace"`
			RootPane struct {
				ID string `json:"pane_id"`
			} `json:"root_pane"`
			Worktree struct {
				Path string `json:"path"`
			} `json:"worktree"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return Created{}, fmt.Errorf("decode Herdr worktree %s response: %w", action, err)
	}
	created := Created{
		WorkspaceID: response.Result.Workspace.ID,
		PaneID:      response.Result.RootPane.ID,
		Path:        response.Result.Worktree.Path,
		Raw:         append(json.RawMessage(nil), data...),
	}
	if created.WorkspaceID == "" || created.PaneID == "" || created.Path == "" {
		return created, fmt.Errorf("Herdr returned an incomplete worktree %s response", action)
	}
	return created, nil
}

func (c Client) Worktrees(cwd string) ([]Worktree, error) {
	data, err := c.Run("worktree", "list", "--cwd", cwd, "--json")
	if err != nil {
		return nil, err
	}
	var response struct {
		Result struct {
			Worktrees []struct {
				Branch          string `json:"branch"`
				Path            string `json:"path"`
				Linked          bool   `json:"is_linked_worktree"`
				Detached        bool   `json:"is_detached"`
				OpenWorkspaceID string `json:"open_workspace_id"`
			} `json:"worktrees"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("decode Herdr worktree list response: %w", err)
	}
	result := make([]Worktree, 0, len(response.Result.Worktrees))
	for _, item := range response.Result.Worktrees {
		result = append(result, Worktree{
			Branch:          item.Branch,
			Path:            item.Path,
			Linked:          item.Linked,
			Detached:        item.Detached,
			OpenWorkspaceID: item.OpenWorkspaceID,
		})
	}
	return result, nil
}

func (c Client) Panes(workspaceID string) ([]Pane, error) {
	data, err := c.Run("pane", "list", "--workspace", workspaceID)
	if err != nil {
		return nil, err
	}
	var response struct {
		Result struct {
			Panes []struct {
				ID          string `json:"pane_id"`
				WorkspaceID string `json:"workspace_id"`
			} `json:"panes"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("decode Herdr pane list response: %w", err)
	}
	result := make([]Pane, 0, len(response.Result.Panes))
	for _, item := range response.Result.Panes {
		result = append(result, Pane{ID: item.ID, WorkspaceID: item.WorkspaceID})
	}
	return result, nil
}

func (c Client) ProcessInfo(paneID string) ([]Process, error) {
	data, err := c.Run("pane", "process-info", "--pane", paneID)
	if err != nil {
		return nil, err
	}
	var response struct {
		Result struct {
			Info struct {
				Processes []struct {
					Name  string   `json:"name"`
					Argv0 string   `json:"argv0"`
					Argv  []string `json:"argv"`
				} `json:"foreground_processes"`
			} `json:"process_info"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("decode Herdr pane process response: %w", err)
	}
	result := make([]Process, 0, len(response.Result.Info.Processes))
	for _, item := range response.Result.Info.Processes {
		result = append(result, Process{Name: item.Name, Argv0: item.Argv0, Argv: append([]string(nil), item.Argv...)})
	}
	return result, nil
}

func (c Client) Split(paneID, cwd string) (Pane, error) {
	data, err := c.Run("pane", "split", paneID, "--direction", "right", "--cwd", cwd, "--no-focus")
	if err != nil {
		return Pane{}, err
	}
	var response struct {
		Result struct {
			Pane struct {
				ID          string `json:"pane_id"`
				WorkspaceID string `json:"workspace_id"`
			} `json:"pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return Pane{}, fmt.Errorf("decode Herdr pane split response: %w", err)
	}
	if response.Result.Pane.ID == "" {
		return Pane{}, errors.New("Herdr returned an incomplete pane split response")
	}
	return Pane{ID: response.Result.Pane.ID, WorkspaceID: response.Result.Pane.WorkspaceID}, nil
}

func (c Client) Workspace(id string) (Workspace, error) {
	data, err := c.Run("workspace", "get", id)
	if err != nil {
		return Workspace{}, err
	}
	var response struct {
		Result struct {
			Workspace struct {
				ID       string `json:"workspace_id"`
				Label    string `json:"label"`
				Worktree struct {
					CheckoutPath string `json:"checkout_path"`
					Linked       bool   `json:"is_linked_worktree"`
				} `json:"worktree"`
			} `json:"workspace"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return Workspace{}, fmt.Errorf("decode Herdr workspace response: %w", err)
	}
	return Workspace{
		ID:             response.Result.Workspace.ID,
		Label:          response.Result.Workspace.Label,
		CheckoutPath:   response.Result.Workspace.Worktree.CheckoutPath,
		LinkedWorktree: response.Result.Workspace.Worktree.Linked,
	}, nil
}

func (c Client) CurrentWorkspaceID() (string, error) {
	if id := os.Getenv("HERDR_WORKSPACE_ID"); id != "" {
		return id, nil
	}
	data, err := c.Run("pane", "current", "--current")
	if err != nil {
		return "", err
	}
	var response struct {
		Result struct {
			Pane struct {
				WorkspaceID string `json:"workspace_id"`
			} `json:"pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return "", fmt.Errorf("decode Herdr current pane response: %w", err)
	}
	if response.Result.Pane.WorkspaceID == "" {
		return "", fmt.Errorf("Herdr did not return the current workspace ID")
	}
	return response.Result.Pane.WorkspaceID, nil
}
