package pullrequest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
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

type Metadata struct {
	Number            int    `json:"number"`
	URL               string `json:"url"`
	Title             string `json:"title"`
	HeadRefName       string `json:"headRefName"`
	HeadRefOID        string `json:"headRefOid"`
	BaseRefName       string `json:"baseRefName"`
	IsCrossRepository bool   `json:"isCrossRepository"`
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
	output, branch, err := lookup(commands, options, "url")
	if err != nil {
		return Result{}, err
	}
	var result Result
	if err := json.Unmarshal(output, &result); err != nil {
		return Result{}, fmt.Errorf("decode GitHub pull request: %w", err)
	}
	if result.URL == "" {
		return Result{}, fmt.Errorf("GitHub returned no URL for the pull request associated with branch %q", branch)
	}
	return result, nil
}

func ResolveNumber(options Options) (int, error) {
	return resolveNumber(commandRunner{}, options)
}

func ResolveMetadata(options Options) (Metadata, error) {
	return resolveMetadata(commandRunner{}, options)
}

func resolveMetadata(commands runner, options Options) (Metadata, error) {
	requested, owner, repository, number, err := parsePullRequestURL(options.Branch)
	if err != nil {
		return Metadata{}, fmt.Errorf("resolve pull request metadata: %w", err)
	}
	if options.Repository != "" && !repositoryMatchesURL(options.Repository, requested.Hostname(), owner, repository) {
		return Metadata{}, fmt.Errorf("repository %q does not match pull request URL repository %s/%s", options.Repository, owner, repository)
	}
	if options.CWD == "" {
		options.CWD, err = os.Getwd()
		if err != nil {
			return Metadata{}, err
		}
	}

	arguments := []string{"pr", "view", options.Branch, "--json", "number,url,title,headRefName,headRefOid,baseRefName,isCrossRepository"}
	if options.Repository != "" {
		arguments = append(arguments, "--repo", options.Repository)
	}
	output, err := commands.Run(options.CWD, "gh", arguments...)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return Metadata{}, errors.New("GitHub CLI (gh) is required to resolve pull request metadata; install it and authenticate with gh auth login")
		}
		return Metadata{}, fmt.Errorf("query GitHub pull request metadata (ensure gh is authenticated with `gh auth login`): %w", err)
	}

	var response struct {
		Number            *int    `json:"number"`
		URL               *string `json:"url"`
		Title             *string `json:"title"`
		HeadRefName       *string `json:"headRefName"`
		HeadRefOID        *string `json:"headRefOid"`
		BaseRefName       *string `json:"baseRefName"`
		IsCrossRepository *bool   `json:"isCrossRepository"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return Metadata{}, fmt.Errorf("decode GitHub pull request metadata: %w", err)
	}
	if response.Number == nil || *response.Number <= 0 || response.URL == nil || *response.URL == "" || response.Title == nil || *response.Title == "" || response.HeadRefName == nil || *response.HeadRefName == "" || response.HeadRefOID == nil || *response.HeadRefOID == "" || response.BaseRefName == nil || *response.BaseRefName == "" || response.IsCrossRepository == nil {
		return Metadata{}, errors.New("GitHub returned incomplete pull request metadata; number, url, title, headRefName, headRefOid, baseRefName, and isCrossRepository are required")
	}
	canonical, canonicalOwner, canonicalRepository, canonicalNumber, err := parsePullRequestURL(*response.URL)
	if err != nil {
		return Metadata{}, fmt.Errorf("GitHub returned an invalid canonical pull request URL: %w", err)
	}
	if !strings.EqualFold(requested.Hostname(), canonical.Hostname()) || !strings.EqualFold(owner, canonicalOwner) || !strings.EqualFold(repository, canonicalRepository) || number != canonicalNumber || number != *response.Number {
		return Metadata{}, errors.New("GitHub returned pull request metadata for a different host, repository, or number")
	}

	return Metadata{
		Number:            *response.Number,
		URL:               *response.URL,
		Title:             *response.Title,
		HeadRefName:       *response.HeadRefName,
		HeadRefOID:        *response.HeadRefOID,
		BaseRefName:       *response.BaseRefName,
		IsCrossRepository: *response.IsCrossRepository,
	}, nil
}

func parsePullRequestURL(value string) (*url.URL, string, string, int, error) {
	if value == "" {
		return nil, "", "", 0, errors.New("Branch must contain a full pull request URL")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, "", "", 0, fmt.Errorf("invalid pull request URL %q: %w", value, err)
	}
	if parsed.Scheme != "https" || parsed.Hostname() == "" || strings.Contains(parsed.Host, ":") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return nil, "", "", 0, fmt.Errorf("pull request URL must be a full HTTPS GitHub URL without userinfo, port, query, or fragment: %q", value)
	}
	parts := strings.Split(parsed.Path, "/")
	if len(parts) != 5 || parts[0] != "" || parts[1] == "" || parts[2] == "" || parts[3] != "pull" || parts[4] == "" {
		return nil, "", "", 0, fmt.Errorf("pull request URL path must be exactly /OWNER/REPO/pull/NUMBER: %q", value)
	}
	number, err := strconv.Atoi(parts[4])
	if err != nil || number <= 0 || strconv.Itoa(number) != parts[4] {
		return nil, "", "", 0, fmt.Errorf("pull request URL has an invalid pull request number: %q", value)
	}
	return parsed, parts[1], parts[2], number, nil
}

func repositoryMatchesURL(value, host, owner, repository string) bool {
	parts := strings.Split(value, "/")
	if len(parts) == 2 {
		return strings.EqualFold(parts[0], owner) && strings.EqualFold(parts[1], repository)
	}
	return len(parts) == 3 && strings.EqualFold(parts[0], host) && strings.EqualFold(parts[1], owner) && strings.EqualFold(parts[2], repository)
}

func resolveNumber(commands runner, options Options) (int, error) {
	output, branch, err := lookup(commands, options, "number")
	if err != nil {
		return 0, err
	}
	var result struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return 0, fmt.Errorf("decode GitHub pull request: %w", err)
	}
	if result.Number <= 0 {
		return 0, fmt.Errorf("GitHub returned no number for the pull request associated with branch %q", branch)
	}
	return result.Number, nil
}

func lookup(commands runner, options Options, fields string) ([]byte, string, error) {
	if options.CWD == "" {
		var err error
		options.CWD, err = os.Getwd()
		if err != nil {
			return nil, "", err
		}
	}
	if _, err := commands.Run(options.CWD, "git", "rev-parse", "--is-inside-work-tree"); err != nil {
		return nil, "", fmt.Errorf("resolve Git worktree: %w", err)
	}
	if options.Branch == "" {
		output, err := commands.Run(options.CWD, "git", "branch", "--show-current")
		if err != nil {
			return nil, "", fmt.Errorf("resolve current branch: %w", err)
		}
		options.Branch = strings.TrimSpace(string(output))
		if options.Branch == "" {
			return nil, "", errors.New("cannot resolve a pull request from detached HEAD; supply a branch argument")
		}
	}

	if options.Repository == "" {
		output, err := commands.Run(options.CWD, "git", "remote")
		if err != nil {
			return nil, "", fmt.Errorf("list Git remotes: %w", err)
		}
		remotes := strings.Fields(string(output))
		switch len(remotes) {
		case 0:
			return nil, "", errors.New("no Git remotes found; add a GitHub remote or specify --repo OWNER/REPO")
		case 1:
		default:
			return nil, "", fmt.Errorf("multiple Git remotes make the GitHub repository ambiguous (%s); specify --repo OWNER/REPO", strings.Join(remotes, ", "))
		}
	}

	arguments := []string{"pr", "view", options.Branch, "--json", fields}
	if options.Repository != "" {
		arguments = append(arguments, "--repo", options.Repository)
	}
	output, err := commands.Run(options.CWD, "gh", arguments...)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, "", errors.New("GitHub CLI (gh) is required to resolve pull requests; install it and authenticate with gh auth login")
		}
		return nil, "", fmt.Errorf("query GitHub pull requests: %w", err)
	}
	return output, options.Branch, nil
}
