package worktree

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/dkarter/hwt/internal/config"
	"github.com/dkarter/hwt/internal/gitutil"
	"github.com/dkarter/hwt/internal/herdr"
)

var invalidSlug = regexp.MustCompile(`[^A-Za-z0-9._-]`)
var errCloneUnavailable = errors.New("copy-on-write cloning is unavailable")

const maxParallelCopies = 8

type Client interface {
	Run(args ...string) ([]byte, error)
	SourceCheckout(cwd string) (string, error)
	Create(args ...string) (herdr.Created, error)
	Workspace(id string) (herdr.Workspace, error)
	CurrentWorkspaceID() (string, error)
}

type CreateOptions struct {
	CWD    string
	Branch string
	Input  string
	Base   string
	Path   string
	Label  string
	Focus  bool
	Ticket string
}

type CreateResult struct {
	WorkspaceID string            `json:"workspace_id"`
	PaneID      string            `json:"pane_id"`
	Path        string            `json:"path"`
	Branch      string            `json:"branch"`
	Base        string            `json:"base"`
	Agent       string            `json:"agent,omitempty"`
	Copied      []string          `json:"copied,omitempty"`
	Config      config.Sources    `json:"config"`
	Environment EnvironmentResult `json:"environment"`
}

type ticketResult struct {
	BranchName string            `json:"branchName"`
	Metadata   map[string]string `json:"metadata"`
}

func Create(client Client, options CreateOptions) (CreateResult, error) {
	repoRoot, err := gitOutput(options.CWD, "rev-parse", "--show-toplevel")
	if err != nil {
		return CreateResult{}, fmt.Errorf("resolve repository root: %w", err)
	}
	hasInput := strings.TrimSpace(options.Input) != ""
	var generated ticketResult
	if options.Ticket != "" && options.Branch != "" {
		return CreateResult{}, errors.New("--ticket cannot be combined with --branch")
	}
	if options.Ticket == "" && ((options.Branch != "" && options.Input != "") || (options.Branch == "" && !hasInput)) {
		return CreateResult{}, errors.New("provide exactly one branch name or --branch")
	}
	if options.Base == "" {
		options.Base, err = gitOutput(repoRoot, "branch", "--show-current")
		if err != nil {
			return CreateResult{}, fmt.Errorf("resolve current branch: %w", err)
		}
		if options.Base == "" {
			return CreateResult{}, errors.New("--base is required from a detached HEAD")
		}
	}

	sourceCheckout, err := client.SourceCheckout(repoRoot)
	if err != nil {
		return CreateResult{}, err
	}
	cfg, sources, err := config.Load(sourceCheckout)
	if err != nil {
		return CreateResult{}, err
	}
	ticketCommand, commandExists := cfg.TicketCommands[options.Ticket]
	if options.Ticket != "" && !commandExists {
		if options.Ticket == "default" {
			return CreateResult{}, errors.New("--ticket requires ticket_commands.default to be configured")
		}
		return CreateResult{}, fmt.Errorf("--ticket=%s requires ticket_commands.%s to be configured", options.Ticket, options.Ticket)
	}
	generatedBranch := options.Ticket != ""
	if generatedBranch {
		if _, err := gitOutput(repoRoot, "rev-parse", "--verify", "--quiet", options.Base+"^{commit}"); err != nil {
			return CreateResult{}, fmt.Errorf("base ref %q does not resolve to a commit: %w", options.Base, err)
		}
		generated, err = runTicketCommand(sourceCheckout, ticketCommand, options.Input)
		if err != nil {
			return CreateResult{}, err
		}
		options.Branch = generated.BranchName
	} else if hasInput {
		options.Branch = NormalizeBranchName(options.Input)
	}
	if err := gitRun(repoRoot, "check-ref-format", "--branch", options.Branch); err != nil {
		if generatedBranch {
			return CreateResult{}, fmt.Errorf("invalid branch %q returned by ticket command: %w", options.Branch, err)
		}
		return CreateResult{}, fmt.Errorf("invalid branch %q: %w", options.Branch, err)
	}
	path, err := worktreePath(sourceCheckout, options.Path, options.Branch, cfg)
	if err != nil {
		return CreateResult{}, err
	}
	if generatedBranch {
		if branchExists, err := gitBranchExists(repoRoot, options.Branch); err != nil {
			return CreateResult{}, err
		} else if branchExists {
			return CreateResult{}, fmt.Errorf("ticket command returned branch %q, but that local branch already exists", options.Branch)
		}
		if path != "" {
			if _, err := os.Lstat(path); err == nil {
				return CreateResult{}, fmt.Errorf("ticket command returned branch %q, but worktree path %s already exists", options.Branch, path)
			} else if !errors.Is(err, os.ErrNotExist) {
				return CreateResult{}, fmt.Errorf("inspect worktree path %s: %w", path, err)
			}
		}
	}
	args := []string{"--cwd", sourceCheckout, "--branch", options.Branch, "--base", options.Base}
	if path != "" {
		args = append(args, "--path", path)
	}
	if options.Label != "" {
		args = append(args, "--label", options.Label)
	}
	if options.Focus {
		args = append(args, "--focus")
	} else {
		args = append(args, "--no-focus")
	}
	args = append(args, "--json")

	created, err := client.Create(args...)
	if err != nil {
		if created.WorkspaceID != "" {
			_, cleanupErr := client.Run("worktree", "remove", "--workspace", created.WorkspaceID, "--force", "--json")
			return CreateResult{}, errors.Join(err, cleanupErr)
		}
		return CreateResult{}, err
	}
	rollback := func(cause error) (CreateResult, error) {
		allocationPath, pathErr := canonicalWorktreePath(created.Path)
		_, cleanupErr := client.Run("worktree", "remove", "--workspace", created.WorkspaceID, "--force", "--json")
		if cleanupErr != nil {
			return CreateResult{}, errors.Join(cause, pathErr, fmt.Errorf("rollback worktree: %w", cleanupErr))
		}
		if pathErr != nil {
			return CreateResult{}, errors.Join(cause, pathErr)
		}
		localDNSErr := releaseLocalDNS(allocationPath, cfg)
		allocationErr := releasePorts(allocationPath)
		return CreateResult{}, errors.Join(cause, localDNSErr, allocationErr)
	}
	if err := writeTicketMetadata(created.Path, generated.Metadata); err != nil {
		return rollback(err)
	}

	copyResult, err := prepareConfiguredFiles(sourceCheckout, created.Path, cfg)
	if err != nil {
		return rollback(err)
	}
	environment, err := prepareEnvironment(created.Path, cfg, false)
	if err != nil {
		return rollback(err)
	}
	if err := runHooks("post_create", created.Path, cfg.PostCreate, environment.Variables); err != nil {
		return rollback(err)
	}
	if err := gitRun(created.Path, "config", "--local", "branch."+options.Branch+".herdr-base", options.Base); err != nil {
		return rollback(fmt.Errorf("record Herdr base branch: %w", err))
	}

	return CreateResult{
		WorkspaceID: created.WorkspaceID,
		PaneID:      created.PaneID,
		Path:        created.Path,
		Branch:      options.Branch,
		Base:        options.Base,
		Agent:       cfg.Agent,
		Copied:      copyResult.Copied,
		Config:      sources,
		Environment: environment,
	}, nil
}

func NormalizeBranchName(value string) string {
	return strings.Join(strings.Fields(value), "-")
}

func gitBranchExists(cwd, branch string) (bool, error) {
	cmd := exec.Command("git", "-C", cwd, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Env = gitEnvironment()
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("check for existing branch %q: %w", branch, err)
}

func EncodeResult(w io.Writer, result any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func worktreePath(repoRoot, override, branch string, cfg config.Config) (string, error) {
	if override != "" {
		return absoluteFrom(repoRoot, override)
	}
	if cfg.WorktreeDir == "" {
		return "", nil
	}
	dir := expandHome(cfg.WorktreeDir)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(repoRoot, dir)
	}
	name := branch
	if cfg.WorktreeNaming == "basename" {
		parts := strings.Split(branch, "/")
		name = parts[len(parts)-1]
	}
	slug := invalidSlug.ReplaceAllString(name, "-")
	return filepath.Clean(filepath.Join(dir, cfg.WorktreePrefix+slug)), nil
}

func absoluteFrom(root, path string) (string, error) {
	path = expandHome(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	return filepath.Abs(path)
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~"+string(filepath.Separator)) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	return path
}

func copyConfigured(sourceRoot, destinationRoot string, entries []config.CopyEntry) ([]string, error) {
	return copyConfiguredWith(sourceRoot, destinationRoot, entries, copyPathConfigured)
}

func copyConfiguredWith(sourceRoot, destinationRoot string, entries []config.CopyEntry, copy func(config.CopyEntry, string, string) error) ([]string, error) {
	copied := make([]bool, len(entries))
	errs := make([]error, len(entries))
	copyEntry := func(index int) {
		entry := entries[index]
		if err := rejectSymlinkParents(sourceRoot, entry.Path); err != nil {
			errs[index] = fmt.Errorf("inspect copy source %s: %w", entry.Path, err)
			return
		}
		if err := rejectSymlinkParents(destinationRoot, entry.Path); err != nil {
			errs[index] = fmt.Errorf("inspect copy destination %s: %w", entry.Path, err)
			return
		}
		source := filepath.Join(sourceRoot, filepath.Clean(entry.Path))
		if _, err := os.Lstat(source); errors.Is(err, os.ErrNotExist) {
			return
		} else if err != nil {
			errs[index] = fmt.Errorf("inspect copy source %s: %w", entry.Path, err)
			return
		}
		destination := filepath.Join(destinationRoot, filepath.Clean(entry.Path))
		if err := copy(entry, source, destination); err != nil {
			errs[index] = fmt.Errorf("copy %s: %w", entry.Path, err)
			return
		}
		copied[index] = true
	}

	runParallel := func(indexes []int) error {
		workerCount := min(len(indexes), maxParallelCopies)
		jobs := make(chan int)
		var wait sync.WaitGroup
		for range workerCount {
			wait.Go(func() {
				for index := range jobs {
					copyEntry(index)
				}
			})
		}
		for _, index := range indexes {
			jobs <- index
		}
		close(jobs)
		wait.Wait()
		return errors.Join(errs...)
	}

	var batch []int
	for index, entry := range entries {
		if !entry.Parallel || overlapsBatch(entry.Path, entries, batch) {
			if err := runParallel(batch); err != nil {
				return nil, err
			}
			batch = batch[:0]
		}
		if entry.Parallel {
			batch = append(batch, index)
			continue
		}
		copyEntry(index)
		if errs[index] != nil {
			return nil, errs[index]
		}
	}
	if err := runParallel(batch); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(entries))
	for index, entry := range entries {
		if copied[index] {
			result = append(result, entry.Path)
		}
	}
	return result, nil
}

func overlapsBatch(path string, entries []config.CopyEntry, batch []int) bool {
	path = filepath.Clean(path)
	for _, index := range batch {
		other := filepath.Clean(entries[index].Path)
		if path == other || strings.HasPrefix(path, other+string(filepath.Separator)) || strings.HasPrefix(other, path+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func copyPathConfigured(entry config.CopyEntry, source, destination string) error {
	if entry.Symlink {
		return replaceWithSymlink(source, destination)
	}
	if entry.CopyOnWrite {
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		info, err := os.Lstat(source)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return copyPath(source, destination)
		}
		if err := clonePath(source, destination); err == nil {
			return nil
		} else if !errors.Is(err, errCloneUnavailable) {
			return fmt.Errorf("clone: %w", err)
		}
	}
	return copyPath(source, destination)
}

func replaceWithSymlink(target, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	return os.Symlink(target, destination)
}

func copyPath(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(source)
		if err != nil {
			return err
		}
		return replaceWithSymlink(target, destination)
	}
	if destinationInfo, err := os.Lstat(destination); err == nil && destinationInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("destination is a symlink")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if info.IsDir() {
		if err := os.MkdirAll(destination, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyPath(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported file type %s", info.Mode().Type())
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	return errors.Join(copyErr, closeErr)
}

func rejectSymlinkParents(root, relative string) error {
	parts := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	current := root
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("parent path %s is a symlink", current)
		}
	}
	return nil
}

func runHooks(name, cwd string, hooks []string, environment map[string]string) error {
	for _, hook := range hooks {
		cmd := exec.Command("/bin/sh", "-lc", hook)
		cmd.Dir = cwd
		cmd.Env = environmentList(environment)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s command %q: %w", name, hook, err)
		}
	}
	return nil
}

func gitOutput(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	cmd.Env = gitEnvironment()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
}

func gitRun(cwd string, args ...string) error {
	_, err := gitOutput(cwd, args...)
	return err
}

func gitEnvironment() []string {
	return gitutil.Environment()
}
