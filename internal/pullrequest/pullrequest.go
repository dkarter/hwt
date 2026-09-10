package pullrequest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/dkarter/hwt/internal/gitutil"
)

type Options struct {
	CWD        string
	Branch     string
	Repository string
}

type Result struct {
	URL string `json:"url"`
}

type runner interface {
	Run(cwd, name string, args ...string) ([]byte, error)
}

type commandRunner struct{}

func (commandRunner) Run(cwd, name string, args ...string) ([]byte, error) {
	command := exec.Command(name, args...)
	command.Dir = cwd
	command.Env = gitutil.Environment()
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %s: %w", strings.Join(append([]string{name}, args...), " "), strings.TrimSpace(string(output)), err)
	}
	return output, nil
}

func Resolve(options Options) (Result, error) {
	return resolve(commandRunner{}, options)
}

func resolve(commands runner, options Options) (Result, error) {
	if options.CWD == "" {
		var err error
		options.CWD, err = os.Getwd()
		if err != nil {
			return Result{}, err
		}
	}
	if _, err := commands.Run(options.CWD, "git", "rev-parse", "--is-inside-work-tree"); err != nil {
		return Result{}, fmt.Errorf("resolve Git worktree: %w", err)
	}
	if options.Branch == "" {
		output, err := commands.Run(options.CWD, "git", "branch", "--show-current")
		if err != nil {
			return Result{}, fmt.Errorf("resolve current branch: %w", err)
		}
		options.Branch = strings.TrimSpace(string(output))
		if options.Branch == "" {
			return Result{}, errors.New("cannot resolve a pull request from detached HEAD; supply a branch argument")
		}
	}

	if options.Repository == "" {
		output, err := commands.Run(options.CWD, "git", "remote")
		if err != nil {
			return Result{}, fmt.Errorf("list Git remotes: %w", err)
		}
		remotes := strings.Fields(string(output))
		switch len(remotes) {
		case 0:
			return Result{}, errors.New("no Git remotes found; add a GitHub remote or specify --repo OWNER/REPO")
		case 1:
		default:
			return Result{}, fmt.Errorf("multiple Git remotes make the GitHub repository ambiguous (%s); specify --repo OWNER/REPO", strings.Join(remotes, ", "))
		}
	}

	arguments := []string{"pr", "view", options.Branch, "--json", "url"}
	if options.Repository != "" {
		arguments = append(arguments, "--repo", options.Repository)
	}
	output, err := commands.Run(options.CWD, "gh", arguments...)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return Result{}, errors.New("GitHub CLI (gh) is required to resolve pull requests; install it and authenticate with gh auth login")
		}
		return Result{}, fmt.Errorf("query GitHub pull requests: %w", err)
	}
	var result Result
	if err := json.Unmarshal(output, &result); err != nil {
		return Result{}, fmt.Errorf("decode GitHub pull request: %w", err)
	}
	if result.URL == "" {
		return Result{}, fmt.Errorf("GitHub returned no URL for the pull request associated with branch %q", options.Branch)
	}
	return result, nil
}
