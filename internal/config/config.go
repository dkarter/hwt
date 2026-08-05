package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

const GlobalMarker = "<global>"
const DefaultWorktreeNaming = "full"
const DefaultCopyParallel = true
const DefaultCopyOnWrite = false

type Config struct {
	Agent          string   `json:"agent,omitempty" yaml:"agent,omitempty"`
	WorktreeDir    string   `json:"worktree_dir,omitempty" yaml:"worktree_dir,omitempty"`
	WorktreeNaming string   `json:"worktree_naming" yaml:"worktree_naming"`
	WorktreePrefix string   `json:"worktree_prefix,omitempty" yaml:"worktree_prefix,omitempty"`
	Files          Files    `json:"files" yaml:"files"`
	PostCreate     []string `json:"post_create,omitempty" yaml:"post_create,omitempty"`
}

type Files struct {
	Copy        []CopyEntry `json:"copy,omitempty" yaml:"copy,omitempty"`
	Parallel    bool        `json:"parallel" yaml:"parallel"`
	CopyOnWrite bool        `json:"copy_on_write" yaml:"copy_on_write"`
}

type CopyEntry struct {
	Path        string `json:"path" yaml:"path"`
	Parallel    bool   `json:"parallel" yaml:"parallel"`
	CopyOnWrite bool   `json:"copy_on_write" yaml:"copy_on_write"`
	Symlink     bool   `json:"symlink" yaml:"symlink"`
}

type Sources struct {
	Global  string `json:"global"`
	Project string `json:"project,omitempty"`
}

type rawConfig struct {
	Agent          *string   `yaml:"agent"`
	WorktreeDir    *string   `yaml:"worktree_dir"`
	WorktreeNaming *string   `yaml:"worktree_naming"`
	WorktreePrefix *string   `yaml:"worktree_prefix"`
	Files          *rawFiles `yaml:"files"`
	PostCreate     *[]string `yaml:"post_create"`
}

type rawFiles struct {
	Copy        *[]rawCopyEntry `yaml:"copy"`
	Parallel    *bool           `yaml:"parallel"`
	CopyOnWrite *bool           `yaml:"copy_on_write"`
}

type rawCopyEntry struct {
	Path        string `yaml:"path"`
	Parallel    *bool  `yaml:"parallel"`
	CopyOnWrite *bool  `yaml:"copy_on_write"`
	Symlink     *bool  `yaml:"symlink"`
}

func (entry *rawCopyEntry) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		return node.Decode(&entry.Path)
	}
	if node.Kind != yaml.MappingNode {
		return errors.New("copy entry must be a path string or object")
	}
	allowed := map[string]bool{"path": true, "parallel": true, "copy_on_write": true, "symlink": true}
	for index := 0; index < len(node.Content); index += 2 {
		if key := node.Content[index].Value; !allowed[key] {
			return fmt.Errorf("unknown copy entry field %q", key)
		}
	}
	type plain rawCopyEntry
	return node.Decode((*plain)(entry))
}

func GlobalPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir != "" {
		return filepath.Join(dir, "hwt", "config.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "hwt", "config.yaml"), nil
}

func ProjectPath(repoRoot string) (string, error) {
	yamlPath := filepath.Join(repoRoot, ".herdr-worktree.yaml")
	ymlPath := filepath.Join(repoRoot, ".herdr-worktree.yml")
	yamlExists := exists(yamlPath)
	ymlExists := exists(ymlPath)
	if yamlExists && ymlExists {
		return "", errors.New("both .herdr-worktree.yaml and .herdr-worktree.yml exist")
	}
	if yamlExists {
		return yamlPath, nil
	}
	if ymlExists {
		return ymlPath, nil
	}
	return "", nil
}

func DefaultProjectPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".herdr-worktree.yaml")
}

func Load(repoRoot string) (Config, Sources, error) {
	globalPath, err := GlobalPath()
	if err != nil {
		return Config{}, Sources{}, err
	}
	projectPath, err := ProjectPath(repoRoot)
	if err != nil {
		return Config{}, Sources{}, err
	}

	global, err := read(globalPath, false)
	if err != nil {
		return Config{}, Sources{}, err
	}
	if err := validateGlobal(global); err != nil {
		return Config{}, Sources{}, fmt.Errorf("validate %s: %w", globalPath, err)
	}
	project, err := read(projectPath, false)
	if err != nil {
		return Config{}, Sources{}, err
	}

	resolved := resolve(global, project)
	if err := Validate(resolved); err != nil {
		return Config{}, Sources{}, err
	}
	return resolved, Sources{Global: globalPath, Project: projectPath}, nil
}

func ValidateFile(path string) error {
	raw, err := read(path, true)
	if err != nil {
		return err
	}
	globalPath, globalPathErr := GlobalPath()
	if globalPathErr == nil && filepath.Clean(path) == filepath.Clean(globalPath) {
		if err := validateGlobal(raw); err != nil {
			return err
		}
	}
	return Validate(resolve(rawConfig{}, raw))
}

func Validate(cfg Config) error {
	if cfg.WorktreeNaming != "full" && cfg.WorktreeNaming != "basename" {
		return fmt.Errorf("worktree_naming must be full or basename, got %q", cfg.WorktreeNaming)
	}
	for _, item := range cfg.Files.Copy {
		if item.Path == GlobalMarker {
			return errors.New("files.copy contains unresolved <global> marker")
		}
		if err := validateRelativePath(item.Path); err != nil {
			return fmt.Errorf("files.copy entry %q: %w", item.Path, err)
		}
		if item.CopyOnWrite && item.Symlink {
			return fmt.Errorf("files.copy entry %q cannot enable both copy_on_write and symlink", item.Path)
		}
	}
	for _, hook := range cfg.PostCreate {
		if hook == GlobalMarker {
			return errors.New("post_create contains unresolved <global> marker")
		}
		if strings.TrimSpace(hook) == "" {
			return errors.New("post_create commands cannot be empty")
		}
	}
	return nil
}

func validateRelativePath(path string) error {
	if path == "" || path == "." {
		return errors.New("must name a repository-relative file or directory")
	}
	if filepath.IsAbs(path) {
		return errors.New("must be relative")
	}
	clean := filepath.Clean(path)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errors.New("must not escape the repository")
	}
	return nil
}

func read(path string, required bool) (rawConfig, error) {
	if path == "" {
		return rawConfig{}, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && !required {
		return rawConfig{}, nil
	}
	if err != nil {
		return rawConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var cfg rawConfig
	if err := decoder.Decode(&cfg); err != nil {
		return rawConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return rawConfig{}, fmt.Errorf("parse %s: multiple YAML documents are not supported", path)
		}
		return rawConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

func resolve(global, project rawConfig) Config {
	cfg := Config{WorktreeNaming: DefaultWorktreeNaming}
	cfg.Agent = scalar(global.Agent, project.Agent, "")
	cfg.WorktreeDir = scalar(global.WorktreeDir, project.WorktreeDir, "")
	cfg.WorktreeNaming = scalar(global.WorktreeNaming, project.WorktreeNaming, DefaultWorktreeNaming)
	cfg.WorktreePrefix = scalar(global.WorktreePrefix, project.WorktreePrefix, "")
	cfg.Files.Parallel = scalar(fileParallel(global.Files), fileParallel(project.Files), DefaultCopyParallel)
	cfg.Files.CopyOnWrite = scalar(fileCopyOnWrite(global.Files), fileCopyOnWrite(project.Files), DefaultCopyOnWrite)
	for _, entry := range copyList(fileCopy(global.Files), fileCopy(project.Files)) {
		cfg.Files.Copy = append(cfg.Files.Copy, CopyEntry{
			Path:        entry.Path,
			Parallel:    scalar(nil, entry.Parallel, cfg.Files.Parallel),
			CopyOnWrite: scalar(nil, entry.CopyOnWrite, cfg.Files.CopyOnWrite),
			Symlink:     scalar(nil, entry.Symlink, false),
		})
	}
	cfg.PostCreate = list(global.PostCreate, project.PostCreate)
	return cfg
}

func validateGlobal(cfg rawConfig) error {
	if entries := fileCopy(cfg.Files); entries != nil {
		for _, entry := range *entries {
			if entry.Path == GlobalMarker {
				return errors.New("<global> cannot be used in the global config")
			}
		}
	}
	if cfg.PostCreate != nil {
		for _, entry := range *cfg.PostCreate {
			if entry == GlobalMarker {
				return errors.New("<global> cannot be used in the global config")
			}
		}
	}
	return nil
}

func scalar[T any](global, project *T, fallback T) T {
	if project != nil {
		return *project
	}
	if global != nil {
		return *global
	}
	return fallback
}

func fileCopy(files *rawFiles) *[]rawCopyEntry {
	if files == nil {
		return nil
	}
	return files.Copy
}

func fileParallel(files *rawFiles) *bool {
	if files == nil {
		return nil
	}
	return files.Parallel
}

func fileCopyOnWrite(files *rawFiles) *bool {
	if files == nil {
		return nil
	}
	return files.CopyOnWrite
}

func copyList(global, project *[]rawCopyEntry) []rawCopyEntry {
	if project == nil {
		if global == nil {
			return nil
		}
		return append([]rawCopyEntry(nil), (*global)...)
	}
	result := make([]rawCopyEntry, 0, len(*project))
	for _, item := range *project {
		if item.Path == GlobalMarker {
			if global != nil {
				result = append(result, (*global)...)
			}
			continue
		}
		result = append(result, item)
	}
	return result
}

func list(global, project *[]string) []string {
	if project == nil {
		if global == nil {
			return nil
		}
		return append([]string(nil), (*global)...)
	}
	result := make([]string, 0, len(*project))
	for _, item := range *project {
		if item == GlobalMarker {
			if global != nil {
				result = append(result, (*global)...)
			}
			continue
		}
		result = append(result, item)
	}
	return result
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
