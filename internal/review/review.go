package review

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dkarter/hwt/internal/config"
	"github.com/dkarter/hwt/internal/gitutil"
	"github.com/dkarter/hwt/internal/herdr"
	"github.com/dkarter/hwt/internal/pullrequest"
	"github.com/dkarter/hwt/internal/worktree"
)

var unsafeBranchName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type Client interface {
	worktree.Client
	Open(args ...string) (herdr.Created, error)
	Worktrees(cwd string) ([]herdr.Worktree, error)
	Panes(workspaceID string) ([]herdr.Pane, error)
	ProcessInfo(paneID string) ([]herdr.Process, error)
	Split(paneID, cwd string) (herdr.Pane, error)
}

type Options struct {
	CWD        string
	Selector   string
	Repository string
	Remote     string
	Focus      bool
}

type Identity struct {
	Kind       string `json:"kind"`
	Selector   string `json:"selector"`
	URL        string `json:"url,omitempty"`
	Number     int    `json:"number,omitempty"`
	Title      string `json:"title,omitempty"`
	HeadBranch string `json:"head_branch,omitempty"`
	BaseBranch string `json:"base_branch,omitempty"`
	Fork       bool   `json:"fork,omitempty"`
	Remote     string `json:"remote,omitempty"`
}

type Launch struct {
	Status  string   `json:"status"`
	Command []string `json:"command"`
	Error   string   `json:"error,omitempty"`
}

type Result struct {
	Identity    Identity `json:"identity"`
	Commit      string   `json:"commit"`
	Branch      string   `json:"branch"`
	Path        string   `json:"path,omitempty"`
	WorkspaceID string   `json:"workspace_id,omitempty"`
	PaneID      string   `json:"pane_id,omitempty"`
	Reused      bool     `json:"reused"`
	Launch      Launch   `json:"review_command"`
}

type dependencies struct {
	resolvePR func(pullrequest.Options) (pullrequest.Metadata, error)
	runGit    func(string, ...string) (string, error)
	findTool  func(string, string) error
}

func Run(client Client, options Options) (Result, error) {
	return run(client, options, dependencies{pullrequest.ResolveMetadata, gitOutput, findTool})
}

func run(client Client, options Options, deps dependencies) (Result, error) {
	if strings.TrimSpace(options.Selector) == "" {
		return Result{}, errors.New("a pull request URL, number, or branch reference is required")
	}
	if options.CWD == "" {
		var err error
		options.CWD, err = os.Getwd()
		if err != nil {
			return Result{}, err
		}
	}
	repoRoot, err := deps.runGit(options.CWD, "rev-parse", "--show-toplevel")
	if err != nil {
		return Result{}, fmt.Errorf("resolve repository root: %w", err)
	}
	source, err := client.SourceCheckout(repoRoot)
	if err != nil {
		return Result{}, fmt.Errorf("resolve primary checkout through Herdr: %w", err)
	}
	cfg, _, err := config.Load(source)
	if err != nil {
		return Result{}, err
	}
	if reviewCommandUsesPRURL(cfg.ReviewCommand) && !pullrequest.IsSelector(options.Selector) {
		return Result{}, errors.New("review_command uses {pr_url}, but the review target is not a pull request URL or number")
	}

	identity, commit, base, reviewBranch, err := resolveTarget(options, source, deps)
	if err != nil {
		return Result{}, err
	}
	reviewCommand, err := expandReviewCommand(cfg.ReviewCommand, identity)
	if err != nil {
		return Result{}, err
	}
	result := Result{Identity: identity, Commit: commit, Branch: reviewBranch, Launch: Launch{Status: "not_started", Command: reviewCommand}}

	ref := "refs/heads/" + reviewBranch
	if current, resolveErr := deps.runGit(source, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); resolveErr == nil {
		if current != commit {
			return result, fmt.Errorf("review branch %q points to %s, not requested commit %s; remove or rename the conflicting branch", reviewBranch, current, commit)
		}
	} else {
		objectFormat, formatErr := deps.runGit(source, "rev-parse", "--show-object-format")
		if formatErr != nil {
			return result, fmt.Errorf("resolve repository object format: %w", formatErr)
		}
		zeroOID := strings.Repeat("0", 40)
		if objectFormat == "sha256" {
			zeroOID = strings.Repeat("0", 64)
		}
		if _, createErr := deps.runGit(source, "update-ref", ref, commit, zeroOID); createErr != nil {
			return result, fmt.Errorf("create review branch %q at %s: %w", reviewBranch, commit, createErr)
		}
	}

	items, err := client.Worktrees(source)
	if err != nil {
		return result, fmt.Errorf("inspect Herdr worktrees: %w", err)
	}
	for _, item := range items {
		if item.Branch != reviewBranch {
			continue
		}
		if item.Detached {
			return result, fmt.Errorf("worktree %s for review branch %q is detached", item.Path, reviewBranch)
		}
		actual, headErr := deps.runGit(item.Path, "rev-parse", "HEAD^{commit}")
		if headErr != nil {
			return result, fmt.Errorf("verify existing review worktree %s: %w", item.Path, headErr)
		}
		if actual != commit {
			return result, fmt.Errorf("existing review worktree %s is at %s, not requested commit %s; refusing to reuse it", item.Path, actual, commit)
		}
		if !item.Linked {
			return result, fmt.Errorf("review branch %q is checked out in the primary checkout; switch it before creating a review workspace", reviewBranch)
		}
		managedBase, managedErr := deps.runGit(source, "config", "--get", "branch."+reviewBranch+".herdr-base")
		if managedErr != nil || managedBase == "" {
			return result, fmt.Errorf("worktree %s uses the review branch but is not marked as HWT-managed; remove the conflict or choose another branch", item.Path)
		}
		if managedBase != base {
			return result, fmt.Errorf("existing review worktree %s uses base %s, not requested base %s; refusing to reuse it", item.Path, managedBase, base)
		}
		result.Path = item.Path
		result.Reused = true
		if item.OpenWorkspaceID != "" {
			result.WorkspaceID = item.OpenWorkspaceID
			panes, panesErr := client.Panes(item.OpenWorkspaceID)
			if panesErr != nil {
				return result, fmt.Errorf("inspect reused Herdr workspace %s: %w", item.OpenWorkspaceID, panesErr)
			}
			if len(panes) == 0 {
				return result, fmt.Errorf("reused Herdr workspace %s has no panes", item.OpenWorkspaceID)
			}
			idlePane := ""
			reviewPane, _ := deps.runGit(source, "config", "--get", "branch."+reviewBranch+".hwt-review-pane")
			for _, pane := range panes {
				processes, processErr := client.ProcessInfo(pane.ID)
				if processErr != nil {
					return result, fmt.Errorf("inspect pane %s in reused workspace: %w", pane.ID, processErr)
				}
				if pane.ID == reviewPane && len(processes) > 0 {
					result.PaneID = pane.ID
					result.Launch.Status = "already_open"
					if options.Focus {
						if _, focusErr := client.Run("workspace", "focus", item.OpenWorkspaceID); focusErr != nil {
							return result, fmt.Errorf("focus reused Herdr workspace %s: %w", item.OpenWorkspaceID, focusErr)
						}
					}
					return result, nil
				}
				if len(processes) == 0 && (idlePane == "" || pane.ID == reviewPane) {
					idlePane = pane.ID
				}
			}
			if idlePane == "" {
				pane, splitErr := client.Split(panes[0].ID, item.Path)
				if splitErr != nil {
					return result, fmt.Errorf("create a review pane in reused workspace %s: %w", item.OpenWorkspaceID, splitErr)
				}
				idlePane = pane.ID
			}
			result.PaneID = idlePane
			if options.Focus {
				if _, focusErr := client.Run("workspace", "focus", item.OpenWorkspaceID); focusErr != nil {
					return result, fmt.Errorf("focus reused Herdr workspace %s: %w", item.OpenWorkspaceID, focusErr)
				}
			}
			return launch(client, result, reviewCommand, deps.findTool, deps.runGit)
		}

		opened, openErr := client.Open("--cwd", source, "--path", item.Path, focusFlag(options.Focus), "--json")
		if openErr != nil {
			return result, fmt.Errorf("open existing review worktree in Herdr: %w", openErr)
		}
		result.WorkspaceID, result.PaneID, result.Path = opened.WorkspaceID, opened.PaneID, opened.Path
		return launch(client, result, reviewCommand, deps.findTool, deps.runGit)
	}

	created, err := worktree.Create(client, worktree.CreateOptions{CWD: source, Branch: reviewBranch, Base: base, Label: reviewLabel(identity), Focus: options.Focus})
	if err != nil {
		return result, fmt.Errorf("create review worktree: %w", err)
	}
	result.WorkspaceID, result.PaneID, result.Path = created.WorkspaceID, created.PaneID, created.Path
	return launch(client, result, reviewCommand, deps.findTool, deps.runGit)
}

func expandReviewCommand(command []string, identity Identity) ([]string, error) {
	expanded := append([]string(nil), command...)
	for index, argument := range expanded {
		if argument != "{pr_url}" {
			continue
		}
		if identity.URL == "" {
			return nil, errors.New("review_command uses {pr_url}, but the review target is not a pull request URL or number")
		}
		expanded[index] = identity.URL
	}
	return expanded, nil
}

func reviewCommandUsesPRURL(command []string) bool {
	for _, argument := range command {
		if argument == "{pr_url}" {
			return true
		}
	}
	return false
}

func launch(client Client, result Result, command []string, locate func(string, string) error, git func(string, ...string) (string, error)) (Result, error) {
	if err := locate(result.Path, command[0]); err != nil {
		result.Launch.Status = "failed"
		result.Launch.Error = err.Error()
		return result, fmt.Errorf("review workspace %s is ready at %s, but the review command could not start: %w", result.WorkspaceID, result.Path, err)
	}
	quoted := make([]string, len(command))
	for index, argument := range command {
		quoted[index] = shellQuote(argument)
	}
	if _, err := git(result.Path, "config", "--local", "branch."+result.Branch+".hwt-review-pane", result.PaneID); err != nil {
		result.Launch.Status = "failed"
		result.Launch.Error = err.Error()
		return result, fmt.Errorf("review workspace %s is ready at %s, but HWT could not record its review pane: %w", result.WorkspaceID, result.Path, err)
	}
	if _, err := client.Run("pane", "run", result.PaneID, strings.Join(quoted, " ")); err != nil {
		result.Launch.Status = "failed"
		result.Launch.Error = err.Error()
		return result, fmt.Errorf("review workspace %s is ready at %s, but Herdr could not launch the review command: %w", result.WorkspaceID, result.Path, err)
	}
	result.Launch.Status = "launched"
	return result, nil
}

func resolveTarget(options Options, source string, deps dependencies) (Identity, string, string, string, error) {
	if pullrequest.IsSelector(options.Selector) {
		metadata, err := deps.resolvePR(pullrequest.Options{CWD: source, Branch: options.Selector, Repository: options.Repository})
		if err != nil {
			return Identity{}, "", "", "", err
		}
		remote, err := selectPRRemote(source, metadata.URL, options.Remote, deps.runGit)
		if err != nil {
			return Identity{}, "", "", "", err
		}
		headRef := fmt.Sprintf("refs/hwt/reviews/pr-%d/head", metadata.Number)
		baseRef := fmt.Sprintf("refs/hwt/reviews/pr-%d/base", metadata.Number)
		if _, err := deps.runGit(source, "fetch", "--no-tags", remote,
			fmt.Sprintf("+refs/pull/%d/head:%s", metadata.Number, headRef),
			fmt.Sprintf("+refs/heads/%s:%s", metadata.BaseRefName, baseRef)); err != nil {
			return Identity{}, "", "", "", fmt.Errorf("fetch pull request #%d from remote %q: %w", metadata.Number, remote, err)
		}
		commit, err := deps.runGit(source, "rev-parse", "--verify", headRef+"^{commit}")
		if err != nil {
			return Identity{}, "", "", "", fmt.Errorf("resolve fetched pull request head: %w", err)
		}
		if !strings.EqualFold(commit, metadata.HeadRefOID) {
			return Identity{}, "", "", "", fmt.Errorf("fetched pull request head SHA %s does not match GitHub metadata %s", commit, metadata.HeadRefOID)
		}
		baseCommit, err := deps.runGit(source, "rev-parse", "--verify", baseRef+"^{commit}")
		if err != nil {
			return Identity{}, "", "", "", fmt.Errorf("resolve fetched pull request base: %w", err)
		}
		identity := Identity{Kind: "pull_request", Selector: options.Selector, URL: metadata.URL, Number: metadata.Number, Title: metadata.Title, HeadBranch: metadata.HeadRefName, BaseBranch: metadata.BaseRefName, Fork: metadata.IsCrossRepository, Remote: remote}
		return identity, commit, baseCommit, fmt.Sprintf("hwt/review/pr-%d", metadata.Number), nil
	}
	if options.Repository != "" {
		return Identity{}, "", "", "", errors.New("--repo is only valid with a pull request URL or number")
	}
	return resolveBranch(options, source, deps.runGit)
}

func resolveBranch(options Options, source string, git func(string, ...string) (string, error)) (Identity, string, string, string, error) {
	if _, err := git(source, "check-ref-format", "--branch", options.Selector); err != nil {
		return Identity{}, "", "", "", fmt.Errorf("invalid branch reference %q: %w", options.Selector, err)
	}
	base, err := git(source, "branch", "--show-current")
	if err != nil {
		return Identity{}, "", "", "", fmt.Errorf("resolve current branch: %w", err)
	}
	if base == "" {
		return Identity{}, "", "", "", errors.New("cannot choose a review base from detached HEAD; switch to a base branch")
	}
	baseCommit, err := git(source, "rev-parse", "--verify", "refs/heads/"+base+"^{commit}")
	if err != nil {
		return Identity{}, "", "", "", fmt.Errorf("resolve review base branch %q: %w", base, err)
	}
	remotes, err := remoteNames(source, git)
	if err != nil {
		return Identity{}, "", "", "", err
	}
	remoteSelector := options.Remote != ""
	for _, remote := range remotes {
		remoteSelector = remoteSelector || strings.HasPrefix(options.Selector, remote+"/")
	}
	if commit, localErr := git(source, "rev-parse", "--verify", "refs/heads/"+options.Selector+"^{commit}"); localErr == nil && !remoteSelector {
		identity := Identity{Kind: "branch", Selector: options.Selector, HeadBranch: options.Selector}
		return identity, commit, baseCommit, branchName(options.Selector), nil
	}
	remote, branch, err := selectBranchRemote(options.Selector, options.Remote, remotes, source, git)
	if err != nil {
		return Identity{}, "", "", "", err
	}
	fetched := "refs/hwt/reviews/branches/" + shortHash(remote+"/"+branch)
	if _, err := git(source, "fetch", "--no-tags", remote, "+refs/heads/"+branch+":"+fetched); err != nil {
		return Identity{}, "", "", "", fmt.Errorf("fetch branch %q from remote %q: %w", branch, remote, err)
	}
	commit, err := git(source, "rev-parse", "--verify", fetched+"^{commit}")
	if err != nil {
		return Identity{}, "", "", "", fmt.Errorf("resolve fetched branch %q: %w", branch, err)
	}
	identity := Identity{Kind: "branch", Selector: options.Selector, HeadBranch: branch, Remote: remote}
	return identity, commit, baseCommit, branchName(remote + "/" + branch), nil
}

func selectPRRemote(cwd, prURL, requested string, git func(string, ...string) (string, error)) (string, error) {
	parsed, _ := url.Parse(prURL)
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	remotes, err := remoteNames(cwd, git)
	if err != nil {
		return "", err
	}
	if requested != "" {
		for _, remote := range remotes {
			if remote == requested {
				value, getErr := git(cwd, "remote", "get-url", remote)
				if getErr != nil || (!remoteMatches(value, parsed.Hostname(), parts[0], parts[1]) && !isLocalRemote(value)) {
					return "", fmt.Errorf("remote %q does not match pull request repository %s/%s on %s", remote, parts[0], parts[1], parsed.Hostname())
				}
				return remote, nil
			}
		}
		return "", fmt.Errorf("Git remote %q does not exist", requested)
	}
	var matches []string
	for _, remote := range remotes {
		value, getErr := git(cwd, "remote", "get-url", remote)
		if getErr == nil && remoteMatches(value, parsed.Hostname(), parts[0], parts[1]) {
			matches = append(matches, remote)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if !strings.EqualFold(parsed.Hostname(), "github.com") && len(matches) == 0 {
		return "", fmt.Errorf("unsupported GitHub Enterprise host %q; configure a matching Git remote or specify --remote", parsed.Hostname())
	}
	if len(remotes) == 0 {
		return "", errors.New("no Git remotes found; add the pull request base repository as a remote")
	}
	return "", fmt.Errorf("cannot identify the pull request base remote among %s; specify --remote", strings.Join(remotes, ", "))
}

func selectBranchRemote(selector, requested string, remotes []string, cwd string, git func(string, ...string) (string, error)) (string, string, error) {
	if requested != "" {
		for _, remote := range remotes {
			if remote != requested && strings.HasPrefix(selector, remote+"/") {
				return "", "", fmt.Errorf("branch reference selects remote %q but --remote selects %q", remote, requested)
			}
		}
		for _, remote := range remotes {
			if remote == requested {
				return remote, strings.TrimPrefix(selector, remote+"/"), nil
			}
		}
		return "", "", fmt.Errorf("Git remote %q does not exist", requested)
	}
	for _, remote := range remotes {
		if strings.HasPrefix(selector, remote+"/") {
			return remote, strings.TrimPrefix(selector, remote+"/"), nil
		}
	}
	var matches []string
	for _, remote := range remotes {
		if _, err := git(cwd, "rev-parse", "--verify", "refs/remotes/"+remote+"/"+selector+"^{commit}"); err == nil {
			matches = append(matches, remote)
		}
	}
	if len(matches) == 1 {
		return matches[0], selector, nil
	}
	if len(matches) > 1 {
		return "", "", fmt.Errorf("branch %q exists on multiple remotes (%s); use REMOTE/BRANCH or --remote", selector, strings.Join(matches, ", "))
	}
	if len(remotes) == 1 {
		return remotes[0], selector, nil
	}
	if len(remotes) == 0 {
		return "", "", fmt.Errorf("branch %q is not local and no Git remotes are configured", selector)
	}
	return "", "", fmt.Errorf("branch %q is not already fetched and the remote is ambiguous (%s); use REMOTE/BRANCH or --remote", selector, strings.Join(remotes, ", "))
}

func remoteNames(cwd string, git func(string, ...string) (string, error)) ([]string, error) {
	output, err := git(cwd, "remote")
	if err != nil {
		return nil, fmt.Errorf("list Git remotes: %w", err)
	}
	remotes := strings.Fields(output)
	sort.Strings(remotes)
	return remotes, nil
}

func remoteMatches(value, host, owner, repository string) bool {
	value = strings.TrimSuffix(value, ".git")
	if parsed, err := url.Parse(value); err == nil && parsed.Hostname() != "" {
		return strings.EqualFold(parsed.Hostname(), host) && strings.EqualFold(strings.Trim(parsed.Path, "/"), owner+"/"+repository)
	}
	prefix := "git@" + host + ":"
	return strings.HasPrefix(strings.ToLower(value), strings.ToLower(prefix)) && strings.EqualFold(strings.TrimPrefix(value, prefix), owner+"/"+repository)
}

func isLocalRemote(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "file" || (parsed.Scheme == "" && !strings.Contains(value, ":")))
}

func reviewLabel(identity Identity) string {
	if identity.Number > 0 {
		return fmt.Sprintf("Review #%d: %s", identity.Number, identity.Title)
	}
	return "Review " + identity.HeadBranch
}

func branchName(identity string) string {
	slug := strings.Trim(unsafeBranchName.ReplaceAllString(identity, "-"), "-.")
	if len(slug) > 40 {
		slug = slug[:40]
	}
	if slug == "" {
		slug = "branch"
	}
	return "hwt/review/" + slug + "-" + shortHash(identity)
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:4])
}

func focusFlag(focus bool) string {
	if focus {
		return "--focus"
	}
	return "--no-focus"
}

func findTool(cwd, name string) error {
	if strings.ContainsRune(name, filepath.Separator) {
		path := name
		if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("find review command %q: %w", name, err)
		}
		if info.IsDir() || info.Mode()&0o111 == 0 {
			return fmt.Errorf("review command %q is not executable", name)
		}
		return nil
	}
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("find review command %q: %w", name, err)
	}
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func gitOutput(cwd string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	command.Env = gitutil.Environment()
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
}
