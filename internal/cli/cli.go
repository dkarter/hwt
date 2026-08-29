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
	"github.com/dkarter/hwt/internal/worktree"
	worktreeSchema "github.com/dkarter/hwt/schema"
	"github.com/dkarter/hwt/skills"
	"github.com/spf13/cobra"
)

type app struct {
	herdrBin string
	version  string
}

func New(version string) *cobra.Command {
	herdrBin := os.Getenv("HERDR_BIN_PATH")
	if herdrBin == "" {
		herdrBin = "herdr"
	}
	a := &app{herdrBin: herdrBin, version: version}
	root := &cobra.Command{
		Use:           "hwt",
		Short:         "Frictionless Herdr worktree orchestration",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&a.herdrBin, "herdr-bin", herdrBin, "path to the Herdr executable")
	root.AddCommand(a.createCommand(), copyCommand(), a.removeCommand(), a.listCommand(), a.configCommand(), a.pluginCommand(), a.herdrCommand(), schemaCommand(), skillCommand())
	return root
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
	parts := strings.Split(version, ".")
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
		Use:   "create --branch BRANCH",
		Short: "Create and configure a Herdr worktree workspace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
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
	flags.BoolVar(&jsonOutput, "json", false, "print machine-readable output")
	_ = command.MarkFlagRequired("branch")
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
