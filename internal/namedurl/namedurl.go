package namedurl

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dkarter/hwt/internal/config"
	"github.com/dkarter/hwt/internal/gitutil"
	"github.com/dkarter/hwt/internal/localdns"
	"github.com/dkarter/hwt/internal/metadatajson"
	"github.com/dkarter/hwt/internal/pullrequest"
	"github.com/dkarter/hwt/internal/urltemplate"
	"github.com/dkarter/hwt/internal/worktree"
)

type Options struct {
	CWD        string
	Name       string
	Branch     string
	Repository string
}

type Result struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type runner interface {
	Run(cwd, name string, args ...string) ([]byte, error)
}

type commandRunner struct{}

func (commandRunner) Run(cwd, name string, args ...string) ([]byte, error) {
	command := exec.Command(name, args...)
	command.Dir = cwd
	if name == "git" {
		command.Env = gitutil.Environment()
	}
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			return nil, fmt.Errorf("%s: %s: %w", name, detail, err)
		}
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return stdout.Bytes(), nil
}

type dependencies struct {
	commands   runner
	loadConfig func(string, ...string) (config.Config, config.Sources, error)
	readTicket func(string) (map[string]string, error)
	resolvePR  func(pullrequest.Options) (int, error)
}

func Resolve(options Options) (Result, error) {
	return resolve(dependencies{
		commands:   commandRunner{},
		loadConfig: config.Load,
		readTicket: worktree.ReadTicketMetadata,
		resolvePR:  pullrequest.ResolveNumber,
	}, options)
}

func resolve(deps dependencies, options Options) (Result, error) {
	if options.Name == "" {
		return Result{}, errors.New("URL name is required")
	}
	if options.CWD == "" {
		var err error
		options.CWD, err = os.Getwd()
		if err != nil {
			return Result{}, err
		}
	}
	topLevel, err := git(deps.commands, options.CWD, "rev-parse", "--show-toplevel")
	if err != nil {
		return Result{}, fmt.Errorf("resolve Git worktree: %w", err)
	}
	cfg, _, err := deps.loadConfig(topLevel)
	if err != nil {
		return Result{}, err
	}
	template, exists := cfg.URLs[options.Name]
	if !exists {
		return Result{}, fmt.Errorf("URL %q is not configured under urls", options.Name)
	}
	placeholders, err := urltemplate.Placeholders(template)
	if err != nil {
		return Result{}, fmt.Errorf("resolve URL %q: %w", options.Name, err)
	}

	explicitBranch := options.Branch != ""
	if explicitBranch && contains(placeholders, "worktree") {
		return Result{}, errors.New("{worktree} is unavailable for an explicit branch; omit the branch argument or remove that placeholder")
	}
	if explicitBranch && contains(placeholders, "hostname") {
		return Result{}, errors.New("{hostname} is unavailable for an explicit branch; omit the branch argument or remove that placeholder")
	}
	if explicitBranch && hasPrefix(placeholders, "ticket.") {
		return Result{}, errors.New("ticket metadata is unavailable for an explicit branch because it belongs to a checked-out worktree")
	}
	if !explicitBranch {
		options.Branch, err = git(deps.commands, topLevel, "branch", "--show-current")
		if err != nil {
			return Result{}, fmt.Errorf("resolve current branch: %w", err)
		}
		if options.Branch == "" {
			return Result{}, errors.New("cannot resolve a URL from detached HEAD; supply a branch argument")
		}
	}

	values := make(map[string]string, len(cfg.Metadata.Values)+8)
	for key, value := range cfg.Metadata.Values {
		values[key] = value
	}
	if !explicitBranch && hasPrefix(placeholders, "ticket.") {
		ticket, err := deps.readTicket(topLevel)
		if err != nil {
			return Result{}, err
		}
		for key, value := range ticket {
			values["ticket."+key] = value
		}
	}
	builtins := map[string]string{
		"branch":           options.Branch,
		"sanitized_branch": urltemplate.SanitizeBranch(options.Branch),
	}
	if !explicitBranch {
		builtins["worktree"] = filepath.Base(topLevel)
	}
	for key, value := range builtins {
		values[key] = value
	}

	commandNames := requiredCommands(placeholders, cfg.Metadata.Commands)
	needsRepository := contains(placeholders, "repository") || contains(placeholders, "hostname")
	needsHostname := contains(placeholders, "hostname")
	for _, name := range commandNames {
		for _, argument := range cfg.Metadata.Commands[name][1:] {
			names, _ := urltemplate.PlaceholdersRaw(argument)
			if explicitBranch && contains(names, "worktree") {
				return Result{}, fmt.Errorf("metadata command %q requires {worktree}, which is unavailable for an explicit branch", name)
			}
			if explicitBranch && contains(names, "hostname") {
				return Result{}, fmt.Errorf("metadata command %q requires {hostname}, which is unavailable for an explicit branch", name)
			}
			needsRepository = needsRepository || contains(names, "repository")
			needsRepository = needsRepository || contains(names, "hostname")
			needsHostname = needsHostname || contains(names, "hostname")
		}
	}
	if needsRepository {
		primary, err := primaryWorktree(deps.commands, topLevel)
		if err != nil {
			return Result{}, err
		}
		builtins["repository"] = filepath.Base(primary)
		values["repository"] = builtins["repository"]
		if needsHostname {
			if !cfg.LocalDNS.Enabled {
				return Result{}, errors.New("{hostname} requires local_dns.enabled")
			}
			hostname, err := localdns.Hostname(primary, topLevel, cfg.LocalDNS.Domain)
			if err != nil {
				return Result{}, fmt.Errorf("resolve {hostname}: %w", err)
			}
			builtins["hostname"] = hostname
			values["hostname"] = hostname
		}
	}
	for _, name := range commandNames {
		output, err := runMetadataCommand(deps.commands, topLevel, cfg.Metadata.Commands[name], builtins)
		if err != nil {
			return Result{}, fmt.Errorf("resolve metadata command %q: %w", name, err)
		}
		for key, value := range output {
			values[name+"."+key] = value
		}
	}
	if contains(placeholders, "pr_number") {
		number, err := deps.resolvePR(pullrequest.Options{CWD: topLevel, Branch: options.Branch, Repository: options.Repository})
		if err != nil {
			return Result{}, fmt.Errorf("resolve {pr_number} for branch %q: %w", options.Branch, err)
		}
		values["pr_number"] = strconv.Itoa(number)
	}

	resolved, err := urltemplate.Expand(template, values)
	if err != nil {
		return Result{}, fmt.Errorf("resolve URL %q: %w", options.Name, err)
	}
	return Result{Name: options.Name, URL: resolved}, nil
}

func BrowserURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("parse URL for browser opening: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("refusing to open non-browser URL scheme %q; only http and https URLs can be opened", parsed.Scheme)
	}
	return nil
}

func primaryWorktree(commands runner, cwd string) (string, error) {
	output, err := commands.Run(cwd, "git", "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return "", fmt.Errorf("list Git worktrees: %w", err)
	}
	first, _, _ := strings.Cut(string(output), "\x00")
	if !strings.HasPrefix(first, "worktree ") {
		return "", errors.New("Git did not report a primary worktree")
	}
	return strings.TrimPrefix(first, "worktree "), nil
}

func git(commands runner, cwd string, args ...string) (string, error) {
	output, err := commands.Run(cwd, "git", args...)
	return strings.TrimSpace(string(output)), err
}

func requiredCommands(placeholders []string, commands map[string][]string) []string {
	var result []string
	seen := map[string]bool{}
	for _, placeholder := range placeholders {
		root, _, nested := strings.Cut(placeholder, ".")
		if nested && commands[root] != nil && !seen[root] {
			seen[root] = true
			result = append(result, root)
		}
	}
	return result
}

func runMetadataCommand(commands runner, cwd string, command []string, builtins map[string]string) (map[string]string, error) {
	arguments := make([]string, len(command)-1)
	for index, argument := range command[1:] {
		expanded, err := urltemplate.ExpandRaw(argument, builtins)
		if err != nil {
			return nil, fmt.Errorf("expand argument %d: %w", index+1, err)
		}
		arguments[index] = expanded
	}
	output, err := commands.Run(cwd, command[0], arguments...)
	if err != nil {
		return nil, err
	}
	values, err := metadatajson.DecodeObject(output)
	if err != nil {
		return nil, fmt.Errorf("decode JSON object: %w", err)
	}
	for key := range values {
		if !urltemplate.ValidPlaceholder(key) {
			return nil, fmt.Errorf("JSON output key %q is not a valid metadata identifier", key)
		}
	}
	return values, nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func hasPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}
