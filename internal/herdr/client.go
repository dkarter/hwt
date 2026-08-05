package herdr

import (
	"bytes"
	"encoding/json"
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
	data, err := c.Run(append([]string{"worktree", "create"}, args...)...)
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
		return Created{}, fmt.Errorf("decode Herdr worktree create response: %w", err)
	}
	created := Created{
		WorkspaceID: response.Result.Workspace.ID,
		PaneID:      response.Result.RootPane.ID,
		Path:        response.Result.Worktree.Path,
		Raw:         append(json.RawMessage(nil), data...),
	}
	if created.WorkspaceID == "" || created.PaneID == "" || created.Path == "" {
		return created, fmt.Errorf("Herdr returned an incomplete worktree create response")
	}
	return created, nil
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
