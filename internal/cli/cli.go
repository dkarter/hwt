package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dkarter/hwt/internal/config"
	"github.com/dkarter/hwt/internal/herdr"
	"github.com/dkarter/hwt/internal/localdns"
	"github.com/dkarter/hwt/internal/namedurl"
	"github.com/dkarter/hwt/internal/review"
	"github.com/dkarter/hwt/internal/urlopen"
	"github.com/dkarter/hwt/internal/worktree"
	worktreeSchema "github.com/dkarter/hwt/schema"
	"github.com/dkarter/hwt/skills"
	"github.com/spf13/cobra"
)

type app struct {
	herdrBin   string
	version    string
	resolveURL func(namedurl.Options) (namedurl.Result, error)
	openURL    func(string) error
}

func New(version string) *cobra.Command {
	return newCommand(version, namedurl.Resolve, urlopen.Open)
}

func newCommand(version string, resolveURL func(namedurl.Options) (namedurl.Result, error), openURL func(string) error) *cobra.Command {
	herdrBin := os.Getenv("HERDR_BIN_PATH")
	if herdrBin == "" {
		herdrBin = "herdr"
	}
	a := &app{herdrBin: herdrBin, version: version, resolveURL: resolveURL, openURL: openURL}
	root := &cobra.Command{
		Use:           "hwt",
		Short:         "Frictionless Herdr worktree orchestration",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&a.herdrBin, "herdr-bin", herdrBin, "path to the Herdr executable")
	root.AddCommand(a.createCommand(), a.reviewCommand(), copyCommand(), environmentCommand(), a.removeCommand(), a.listCommand(), a.urlCommand(), dnsCommand(), a.configCommand(), a.pluginCommand(), a.herdrCommand(), schemaCommand(), skillCommand())
	return root
}

func (a *app) reviewCommand() *cobra.Command {
	options := review.Options{}
	jsonOutput := false
	command := &cobra.Command{
		Use:   "review <pull-request-url|number|branch>",
		Short: "Open a pull request or branch in a dedicated review workspace",
		Long: "Fetch a GitHub pull request URL, pull request number, or branch without changing the primary checkout, create or reuse an exact-commit Herdr worktree, and launch the configured review command.\n\n" +
			"Pull request URLs must use HTTPS and the /OWNER/REPO/pull/NUMBER form. A positive decimal selector is a pull request number. Branches may be local names, REMOTE/BRANCH references, or unfetched names when the repository has one remote.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Selector = args[0]
			result, err := review.Run(a.client(), options)
			if jsonOutput && (err == nil || result.Path != "") {
				if encodeErr := worktree.EncodeResult(cmd.OutOrStdout(), result); encodeErr != nil {
					return errors.Join(err, encodeErr)
				}
			} else if result.Path != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Review workspace ready at %s (workspace %s, commit %s)\n", result.Path, result.WorkspaceID, result.Commit)
				if err == nil {
					fmt.Fprintf(cmd.OutOrStdout(), "Review command: %s\n", result.Launch.Status)
				}
			}
			return err
		},
	}
	flags := command.Flags()
	flags.StringVar(&options.CWD, "cwd", "", "repository path (defaults to the current directory)")
	flags.StringVarP(&options.Repository, "repo", "R", "", "GitHub repository in [HOST/]OWNER/REPO format (pull requests only)")
	flags.StringVar(&options.Remote, "remote", "", "Git remote to fetch when it cannot be selected unambiguously")
	flags.BoolVar(&options.Focus, "focus", false, "focus the review workspace")
	flags.BoolVar(&jsonOutput, "json", false, "print machine-readable workspace and launch details")
	return command
}

func (a *app) urlCommand() *cobra.Command {
	options := namedurl.Options{}
	jsonOutput := false
	open := false
	command := &cobra.Command{
		Use:   "url [name] [branch]",
		Short: "Resolve a configured named URL",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 2 {
				return fmt.Errorf("accepts at most 2 args, received %d", len(args))
			}
			if len(args) == 0 && !jsonOutput {
				return errors.New("URL name is required unless --json lists all URLs")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				results, err := namedurl.ResolveAll(options)
				if err != nil {
					return err
				}
				return worktree.EncodeResult(cmd.OutOrStdout(), results)
			}
			options.Name = args[0]
			if len(args) == 2 {
				options.Branch = args[1]
			}
			result, err := a.resolveURL(options)
			if err != nil {
				return err
			}
			if jsonOutput {
				return worktree.EncodeResult(cmd.OutOrStdout(), result)
			}
			if open {
				if err := namedurl.BrowserURL(result.URL); err != nil {
					return err
				}
				if err := a.openURL(result.URL); err != nil {
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Opened %s\n", result.URL)
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), result.URL)
			return err
		},
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			names, err := namedurl.Names(options.CWD)
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return names, cobra.ShellCompDirectiveNoFileComp
		},
	}
	command.Flags().StringVar(&options.CWD, "cwd", "", "repository path (defaults to the current directory)")
	command.Flags().StringVarP(&options.Repository, "repo", "R", "", "GitHub repository in [HOST/]OWNER/REPO format for pull request placeholders")
	command.Flags().BoolVar(&jsonOutput, "json", false, "print machine-readable output")
	command.Flags().BoolVar(&open, "open", false, "open an http or https URL in the default browser")
	command.MarkFlagsMutuallyExclusive("json", "open")
	return command
}

func environmentCommand() *cobra.Command {
	cwd := ""
	refresh := false
	jsonOutput := false
	command := &cobra.Command{
		Use:   "env [-- COMMAND...]",
		Short: "Generate or use the current worktree environment",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cwd == "" {
				var err error
				cwd, err = os.Getwd()
				if err != nil {
					return err
				}
			}
			result, err := worktree.Environment(cwd, refresh)
			if err != nil {
				return err
			}
			if len(args) > 0 {
				return worktree.RunWithEnvironment(result, args)
			}
			if jsonOutput {
				return worktree.EncodeResult(cmd.OutOrStdout(), result)
			}
			fmt.Fprintln(cmd.OutOrStdout(), result.Path)
			return nil
		},
	}
	command.Flags().StringVar(&cwd, "cwd", "", "worktree path (defaults to the current directory)")
	command.Flags().BoolVar(&refresh, "refresh", false, "replace this worktree's port allocation")
	command.Flags().BoolVar(&jsonOutput, "json", false, "print machine-readable output")
	return command
}

func dnsCommand() *cobra.Command {
	command := &cobra.Command{Use: "dns", Short: "Manage HWT-owned local DNS and Caddy snippets"}
	command.AddCommand(dnsSetupCommand(), dnsStatusCommand(), dnsRefreshCommand(), dnsTeardownCommand())
	return command
}

func dnsSetupCommand() *cobra.Command {
	cwd := ""
	jsonOutput := false
	command := &cobra.Command{
		Use:   "setup",
		Short: "Generate local DNS and Caddy snippets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadDNSConfig(cwd)
			if err != nil {
				return err
			}
			result, err := localdns.Setup(cfg)
			if err != nil {
				return err
			}
			if jsonOutput {
				return worktree.EncodeResult(cmd.OutOrStdout(), result)
			}
			fmt.Fprintln(cmd.OutOrStdout(), result.DNSMasqInclude)
			fmt.Fprintln(cmd.OutOrStdout(), result.CaddyImport)
			return nil
		},
	}
	command.Flags().StringVar(&cwd, "cwd", "", "repository path (defaults to the current directory)")
	command.Flags().BoolVar(&jsonOutput, "json", false, "print machine-readable output")
	return command
}

func dnsStatusCommand() *cobra.Command {
	cwd := ""
	jsonOutput := false
	command := &cobra.Command{
		Use:   "status",
		Short: "Inspect local DNS registrations and snippet paths",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadDNSConfig(cwd)
			if err != nil {
				return err
			}
			result, err := localdns.Status(cfg)
			if err != nil {
				return err
			}
			if jsonOutput {
				return worktree.EncodeResult(cmd.OutOrStdout(), result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Caddy: %s\ndnsmasq: %s\nRegistered worktrees: %d\n", result.Paths.Caddyfile, result.Paths.DNSMasq, len(result.Entries))
			return nil
		},
	}
	command.Flags().StringVar(&cwd, "cwd", "", "repository path (defaults to the current directory)")
	command.Flags().BoolVar(&jsonOutput, "json", false, "print machine-readable output")
	return command
}

func dnsRefreshCommand() *cobra.Command {
	cwd := ""
	jsonOutput := false
	command := &cobra.Command{
		Use:   "refresh",
		Short: "Reconcile the current worktree route without replacing ports",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cwd == "" {
				var err error
				cwd, err = os.Getwd()
				if err != nil {
					return err
				}
			}
			result, err := worktree.Environment(cwd, false)
			if err != nil {
				return err
			}
			if jsonOutput {
				return worktree.EncodeResult(cmd.OutOrStdout(), result)
			}
			fmt.Fprintln(cmd.OutOrStdout(), result.Variables["HWT_WORKTREE_HOSTNAME"])
			return nil
		},
	}
	command.Flags().StringVar(&cwd, "cwd", "", "worktree path (defaults to the current directory)")
	command.Flags().BoolVar(&jsonOutput, "json", false, "print machine-readable output")
	return command
}

func dnsTeardownCommand() *cobra.Command {
	cwd := ""
	force := false
	jsonOutput := false
	command := &cobra.Command{
		Use:   "teardown",
		Short: "Remove HWT-owned local DNS state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadDNSConfig(cwd)
			if err != nil {
				return err
			}
			if err := localdns.Teardown(cfg, force); err != nil {
				return err
			}
			if jsonOutput {
				return worktree.EncodeResult(cmd.OutOrStdout(), map[string]bool{"removed": true})
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Removed HWT-owned generated local DNS files")
			return nil
		},
	}
	command.Flags().StringVar(&cwd, "cwd", "", "repository path (defaults to the current directory)")
	command.Flags().BoolVar(&force, "force", false, "remove state even when worktrees are registered")
	command.Flags().BoolVar(&jsonOutput, "json", false, "print machine-readable output")
	return command
}

func loadDNSConfig(cwd string) (localdns.Config, error) {
	root, err := repoRoot(cwd)
	if err != nil {
		return localdns.Config{}, err
	}
	cfg, _, err := config.Load(root)
	if err != nil {
		return localdns.Config{}, err
	}
	return localdns.Config{Enabled: cfg.LocalDNS.Enabled, Domain: cfg.LocalDNS.Domain, Reload: cfg.LocalDNS.Reload}, nil
}

func (a *app) pluginCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "plugin",
		Short: "Manage the HWT plugin for Herdr",
	}
	command.AddCommand(
		a.pluginInstallCommand("install", "Install the HWT plugin for Herdr"),
		a.pluginInstallCommand("update", "Update the HWT plugin for Herdr"),
		a.pluginUninstallCommand(),
	)
	return command
}

func (a *app) pluginInstallCommand(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			arguments := []string{"plugin", "install", "dkarter/hwt/plugins/herdr"}
			if ref := releaseRef(a.version); ref != "" {
				arguments = append(arguments, "--ref", ref)
			}
			arguments = append(arguments, "--yes")
			output, err := a.client().Run(arguments...)
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(output)
			return err
		},
	}
}

func releaseRef(version string) string {
	version = strings.TrimPrefix(version, "v")
	core := version
	if candidate, prerelease, found := strings.Cut(version, "-"); found {
		if !strings.HasPrefix(prerelease, "dev.") {
			return ""
		}
		core = candidate
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return ""
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return ""
		}
	}
	return "v" + version
}

func (a *app) pluginUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall the HWT plugin from Herdr",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			output, err := a.client().Run("plugin", "uninstall", "hwt.worktrees")
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(output)
			return err
		},
	}
}

func copyCommand() *cobra.Command {
	options := worktree.CopyOptions{}
	jsonOutput := false
	command := &cobra.Command{
		Use:   "copy",
		Short: "Copy configured files into a new worktree once",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if options.CWD == "" {
				context, err := currentPluginContext()
				if err != nil {
					return err
				}
				options.CWD = context.WorkspaceCWD
			}
			if options.CWD == "" {
				var err error
				options.CWD, err = os.Getwd()
				if err != nil {
					return err
				}
			}
			result, err := worktree.Copy(options)
			if err != nil {
				return err
			}
			if jsonOutput {
				return worktree.EncodeResult(cmd.OutOrStdout(), result)
			}
			if result.AlreadyPrepared {
				fmt.Fprintf(cmd.OutOrStdout(), "Configured files already prepared at %s\n", result.Path)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Copied configured files from %s to %s\n", result.Source, result.Path)
			return nil
		},
	}
	command.Flags().StringVar(&options.CWD, "cwd", "", "worktree path (defaults to the Herdr event worktree or current directory)")
	command.Flags().BoolVar(&jsonOutput, "json", false, "print machine-readable output")
	return command
}

func (a *app) client() herdr.Client {
	return herdr.Client{Binary: a.herdrBin}
}

func (a *app) createCommand() *cobra.Command {
	options := worktree.CreateOptions{}
	jsonOutput := false
	command := &cobra.Command{
		Use:   "create [branch-or-ticket-input]",
		Short: "Create and configure a Herdr worktree workspace",
		Long: "Create a branch from a literal positional value by default. With --ticket, HWT runs ticket_commands.default; use --ticket=NAME to select another configured command.\n\n" +
			"Ticket input is optional for commands that provide an interactive picker. The --branch flag cannot be combined with --ticket or a positional value.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				options.Input = args[0]
			}
			if options.Ticket != "" && options.Branch != "" {
				return errors.New("--ticket cannot be combined with --branch")
			}
			if options.Ticket == "" && ((options.Branch != "" && len(args) != 0) || (options.Branch == "" && strings.TrimSpace(options.Input) == "")) {
				return errors.New("provide exactly one branch name or --branch")
			}
			if options.CWD == "" {
				var err error
				options.CWD, err = os.Getwd()
				if err != nil {
					return err
				}
			}
			result, err := worktree.Create(a.client(), options)
			if err != nil {
				return err
			}
			if jsonOutput {
				return worktree.EncodeResult(cmd.OutOrStdout(), result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created %s at %s (workspace %s)\n", result.Branch, result.Path, result.WorkspaceID)
			return nil
		},
	}
	flags := command.Flags()
	flags.StringVarP(&options.Branch, "branch", "b", "", "branch to create")
	flags.StringVar(&options.Base, "base", "", "base ref (defaults to the current branch)")
	flags.StringVar(&options.CWD, "cwd", "", "repository path (defaults to the current directory)")
	flags.StringVar(&options.Path, "path", "", "override the configured worktree path")
	flags.StringVar(&options.Label, "label", "", "Herdr workspace label")
	flags.BoolVar(&options.Focus, "focus", false, "focus the new workspace")
	flags.StringVar(&options.Ticket, "ticket", "", "derive the branch with a named ticket command")
	flags.Lookup("ticket").NoOptDefVal = "default"
	flags.BoolVar(&jsonOutput, "json", false, "print machine-readable output")
	return command
}

func (a *app) removeCommand() *cobra.Command {
	options := worktree.RemoveOptions{}
	jsonOutput := false
	command := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "Quickly remove a Herdr worktree workspace",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := worktree.Remove(a.client(), options)
			if err != nil {
				return err
			}
			if jsonOutput {
				return worktree.EncodeResult(cmd.OutOrStdout(), result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed workspace %s at %s\n", result.WorkspaceID, result.Path)
			return nil
		},
	}
	flags := command.Flags()
	flags.StringVarP(&options.WorkspaceID, "workspace", "w", "", "workspace ID (defaults to the current workspace)")
	flags.BoolVarP(&options.Force, "force", "f", false, "remove a dirty or locked worktree")
	flags.BoolVar(&jsonOutput, "json", false, "print machine-readable output")
	return command
}

func (a *app) listCommand() *cobra.Command {
	cwd := ""
	command := &cobra.Command{
		Use:   "list",
		Short: "List Herdr worktrees",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cwd == "" {
				var err error
				cwd, err = os.Getwd()
				if err != nil {
					return err
				}
			}
			data, err := a.client().Run("worktree", "list", "--cwd", cwd, "--json")
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(data)
			return err
		},
	}
	command.Flags().StringVar(&cwd, "cwd", "", "repository path (defaults to the current directory)")
	return command
}

func (a *app) configCommand() *cobra.Command {
	command := &cobra.Command{Use: "config", Short: "Inspect and validate hwt configuration"}
	command.AddCommand(configPathCommand(), configShowCommand(), configValidateCommand(), configInitCommand())
	return command
}

func configPathCommand() *cobra.Command {
	global := false
	gitCommon := false
	command := &cobra.Command{
		Use:   "path",
		Short: "Print the global or repository config path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if global {
				path, err := config.GlobalPath()
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), path)
				return nil
			}
			root, err := repoRoot("")
			if err != nil {
				return err
			}
			var path string
			if gitCommon {
				path, err = config.GitCommonPath(root)
			} else {
				path, err = config.ProjectPath(root)
				if err == nil && path == "" {
					path, err = config.FindGitCommonPath(root)
				}
			}
			if err != nil {
				return err
			}
			if path == "" {
				path = config.DefaultProjectPath(root)
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
	command.Flags().BoolVar(&global, "global", false, "print the XDG global config path")
	command.Flags().BoolVar(&gitCommon, "git-common", false, "print the shared Git-local config path")
	command.MarkFlagsMutuallyExclusive("global", "git-common")
	return command
}

func configShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the resolved configuration as JSON",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := repoRoot("")
			if err != nil {
				return err
			}
			cfg, sources, err := config.Load(root)
			if err != nil {
				return err
			}
			return worktree.EncodeResult(cmd.OutOrStdout(), struct {
				Config  config.Config  `json:"config"`
				Sources config.Sources `json:"sources"`
			}{cfg, sources})
		},
	}
}

func configValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [path]",
		Short: "Validate one config file or the resolved repository configuration",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				if err := config.ValidateFile(args[0]); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s is valid\n", args[0])
				return nil
			}
			root, err := repoRoot("")
			if err != nil {
				return err
			}
			_, _, err = config.Load(root)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "configuration is valid")
			return nil
		},
	}
}

func configInitCommand() *cobra.Command {
	global := false
	gitCommon := false
	command := &cobra.Command{
		Use:   "init",
		Short: "Create a configuration file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var path string
			var err error
			if global {
				path, err = config.GlobalPath()
			} else {
				var root string
				root, err = repoRoot("")
				if err == nil && gitCommon {
					path, err = config.GitCommonPath(root)
				} else {
					path = config.DefaultProjectPath(root)
				}
			}
			if err != nil {
				return err
			}
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("refusing to overwrite %s", path)
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			content := "# yaml-language-server: $schema=https://raw.githubusercontent.com/dkarter/hwt/main/schema/herdr-worktree.schema.json\nworktree_naming: " + config.DefaultWorktreeNaming + "\nfiles:\n  copy: []\n"
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
	command.Flags().BoolVar(&global, "global", false, "initialize the XDG global config")
	command.Flags().BoolVar(&gitCommon, "git-common", false, "initialize the shared Git-local config")
	command.MarkFlagsMutuallyExclusive("global", "git-common")
	return command
}

func schemaCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Print the managed JSON schema",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var value any
			if err := json.Unmarshal(worktreeSchema.JSON, &value); err != nil {
				return err
			}
			return worktree.EncodeResult(cmd.OutOrStdout(), value)
		},
	}
}

func skillCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "skill",
		Short: "Print the hwt skill for AI agent discovery",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := cmd.OutOrStdout().Write(skills.Usage)
			return err
		},
	}
	command.AddCommand(&cobra.Command{
		Use:   "config",
		Short: "Print the on-demand project configuration reference",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := cmd.OutOrStdout().Write(skills.ProjectConfig)
			return err
		},
	})
	return command
}

func repoRoot(cwd string) (string, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	command := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel")
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
}
