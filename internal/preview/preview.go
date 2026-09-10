package preview

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dkarter/hwt/internal/config"
	"github.com/dkarter/hwt/internal/gitutil"
	"github.com/dkarter/hwt/internal/previewurl"
)

type Options struct {
	CWD    string
	Branch string
}

type Result struct {
	URL string `json:"url"`
}

type runner interface {
	Run(cwd string, args ...string) ([]byte, error)
}

type commandRunner struct{}

func (commandRunner) Run(cwd string, args ...string) ([]byte, error) {
	command := exec.Command("git", args...)
	command.Dir = cwd
	command.Env = gitutil.Environment()
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(output)), err)
	}
	return output, nil
}

func Resolve(options Options) (Result, error) {
	return resolve(commandRunner{}, config.Load, options)
}

func resolve(commands runner, loadConfig func(string, ...string) (config.Config, config.Sources, error), options Options) (Result, error) {
	if options.CWD == "" {
		var err error
		options.CWD, err = os.Getwd()
		if err != nil {
			return Result{}, err
		}
	}
	topLevelOutput, err := commands.Run(options.CWD, "rev-parse", "--show-toplevel")
	if err != nil {
		return Result{}, fmt.Errorf("resolve Git worktree: %w", err)
	}
	topLevel := strings.TrimSpace(string(topLevelOutput))
	cfg, _, err := loadConfig(topLevel)
	if err != nil {
		return Result{}, err
	}
	if cfg.PreviewURL == "" {
		return Result{}, errors.New("preview_url is not configured; add it to .herdr-worktree.yaml or the global hwt config")
	}

	values := map[string]string{}
	if options.Branch == "" {
		branchOutput, err := commands.Run(topLevel, "branch", "--show-current")
		if err != nil {
			return Result{}, fmt.Errorf("resolve current branch: %w", err)
		}
		options.Branch = strings.TrimSpace(string(branchOutput))
		if options.Branch == "" {
			return Result{}, errors.New("cannot resolve a preview from detached HEAD; supply a branch argument")
		}
		values["worktree"] = filepath.Base(topLevel)
	} else if strings.Contains(cfg.PreviewURL, "{worktree}") {
		return Result{}, errors.New("preview_url uses {worktree}, which is unavailable for an explicit branch; omit the branch argument or remove that placeholder")
	}
	values["branch"] = options.Branch
	values["sanitized_branch"] = previewurl.SanitizeBranch(options.Branch)

	if strings.Contains(cfg.PreviewURL, "{repository}") {
		worktreesOutput, err := commands.Run(topLevel, "worktree", "list", "--porcelain", "-z")
		if err != nil {
			return Result{}, fmt.Errorf("resolve repository name: %w", err)
		}
		const prefix = "worktree "
		firstLine, _, _ := strings.Cut(string(worktreesOutput), "\x00")
		if !strings.HasPrefix(firstLine, prefix) || strings.TrimPrefix(firstLine, prefix) == "" {
			return Result{}, errors.New("cannot resolve repository name from the primary Git worktree")
		}
		values["repository"] = filepath.Base(filepath.Clean(strings.TrimPrefix(firstLine, prefix)))
	}
	resolved, err := previewurl.Expand(cfg.PreviewURL, values)
	if err != nil {
		return Result{}, fmt.Errorf("resolve preview_url: %w", err)
	}
	return Result{URL: resolved}, nil
}
