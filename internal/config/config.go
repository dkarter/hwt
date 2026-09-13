package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dkarter/hwt/internal/gitutil"
	"github.com/dkarter/hwt/internal/urltemplate"
	"go.yaml.in/yaml/v3"
)

const GlobalMarker = "<global>"
const DefaultWorktreeNaming = "full"
const DefaultCopyParallel = true
const DefaultCopyOnWrite = false
const DefaultLocalDNSDomain = "hwt.test"

var defaultTicketCommand = []string{"lnr", "quick", "--json"}
var defaultReviewCommand = []string{"tuicr"}
var defaultURLs = map[string]string{"pr": "https://{pr_host}/{pr_owner}/{pr_repository}/pull/{pr_number}"}
var serviceNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)
var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var dnsLabelPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

type Config struct {
	Agent          string            `json:"agent,omitempty" yaml:"agent,omitempty"`
	TicketCommand  []string          `json:"ticket_command" yaml:"ticket_command"`
	ReviewCommand  []string          `json:"review_command" yaml:"review_command"`
	WorktreeDir    string            `json:"worktree_dir,omitempty" yaml:"worktree_dir,omitempty"`
	WorktreeNaming string            `json:"worktree_naming" yaml:"worktree_naming"`
	WorktreePrefix string            `json:"worktree_prefix,omitempty" yaml:"worktree_prefix,omitempty"`
	Files          Files             `json:"files" yaml:"files"`
	PostCreate     []string          `json:"post_create,omitempty" yaml:"post_create,omitempty"`
	Ports          Ports             `json:"ports" yaml:"ports"`
	Environment    Environment       `json:"environment" yaml:"environment"`
	LocalDNS       LocalDNS          `json:"local_dns" yaml:"local_dns"`
	URLs           map[string]string `json:"urls,omitempty" yaml:"urls,omitempty"`
	Metadata       Metadata          `json:"metadata" yaml:"metadata"`
}

type Ports struct {
	Start    int      `json:"start" yaml:"start"`
	End      int      `json:"end" yaml:"end"`
	Services []string `json:"services,omitempty" yaml:"services,omitempty"`
}

type Environment struct {
	Variables map[string]string `json:"variables,omitempty" yaml:"variables,omitempty"`
}

type LocalDNS struct {
	Enabled bool     `json:"enabled" yaml:"enabled"`
	Domain  string   `json:"domain" yaml:"domain"`
	Reload  []string `json:"reload,omitempty" yaml:"reload,omitempty"`
}

type Metadata struct {
	Values   map[string]string   `json:"values,omitempty" yaml:"values,omitempty"`
	Commands map[string][]string `json:"commands,omitempty" yaml:"commands,omitempty"`
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
	Global    string `json:"global"`
	GitCommon string `json:"git_common,omitempty"`
	Project   string `json:"project,omitempty"`
}

type rawConfig struct {
	Agent          *string            `yaml:"agent"`
	TicketCommand  *[]string          `yaml:"ticket_command"`
	ReviewCommand  *[]string          `yaml:"review_command"`
	WorktreeDir    *string            `yaml:"worktree_dir"`
	WorktreeNaming *string            `yaml:"worktree_naming"`
	WorktreePrefix *string            `yaml:"worktree_prefix"`
	Files          *rawFiles          `yaml:"files"`
	PostCreate     *[]string          `yaml:"post_create"`
	Ports          *rawPorts          `yaml:"ports"`
	Environment    *rawEnvironment    `yaml:"environment"`
	LocalDNS       *rawLocalDNS       `yaml:"local_dns"`
	URLs           *map[string]string `yaml:"urls"`
	Metadata       *rawMetadata       `yaml:"metadata"`
}

type rawMetadata struct {
	Values   *map[string]string   `yaml:"values"`
	Commands *map[string][]string `yaml:"commands"`
}

type rawPorts struct {
	Start    *int      `yaml:"start"`
	End      *int      `yaml:"end"`
	Services *[]string `yaml:"services"`
}

type rawEnvironment struct {
	Variables *map[string]string `yaml:"variables"`
}

type rawLocalDNS struct {
	Enabled *bool     `yaml:"enabled"`
	Domain  *string   `yaml:"domain"`
	Reload  *[]string `yaml:"reload"`
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
	return configPath(repoRoot, ".herdr-worktree")
}

func GitCommonPath(repoRoot string) (string, error) {
	dir, err := gitCommonDir(repoRoot)
	if err != nil || dir == "" {
		return "", err
	}
	path, err := findGitCommonPath(dir)
	if err != nil || path != "" {
		return path, err
	}
	return filepath.Join(dir, "hwt", "config.yaml"), nil
}

func FindGitCommonPath(repoRoot string) (string, error) {
	dir, err := gitCommonDir(repoRoot)
	if err != nil || dir == "" {
		return "", err
	}
	return findGitCommonPath(dir)
}

func findGitCommonPath(dir string) (string, error) {
	path, err := configPath(filepath.Join(dir, "hwt"), "config")
	return path, err
}

func configPath(dir, name string) (string, error) {
	yamlPath := filepath.Join(dir, name+".yaml")
	ymlPath := filepath.Join(dir, name+".yml")
	yamlExists := exists(yamlPath)
	ymlExists := exists(ymlPath)
	if yamlExists && ymlExists {
		return "", fmt.Errorf("both %s and %s exist", yamlPath, ymlPath)
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

func Load(repoRoot string, commonDirs ...string) (Config, Sources, error) {
	globalPath, err := GlobalPath()
	if err != nil {
		return Config{}, Sources{}, err
	}
	projectPath, err := ProjectPath(repoRoot)
	if err != nil {
		return Config{}, Sources{}, err
	}
	gitCommonPath := ""
	if projectPath == "" {
		if len(commonDirs) > 0 {
			gitCommonPath, err = findGitCommonPath(commonDirs[0])
		} else {
			gitCommonPath, err = FindGitCommonPath(repoRoot)
		}
		if err != nil {
			return Config{}, Sources{}, err
		}
	}

	global, err := read(globalPath, false)
	if err != nil {
		return Config{}, Sources{}, err
	}
	if err := validateGlobal(global); err != nil {
		return Config{}, Sources{}, fmt.Errorf("validate %s: %w", globalPath, err)
	}
	repositoryPath := projectPath
	if repositoryPath == "" {
		repositoryPath = gitCommonPath
	}
	repository, err := read(repositoryPath, false)
	if err != nil {
		return Config{}, Sources{}, err
	}

	resolved := resolve(global, repository)
	if err := Validate(resolved); err != nil {
		return Config{}, Sources{}, err
	}
	return resolved, Sources{Global: globalPath, GitCommon: gitCommonPath, Project: projectPath}, nil
}

func gitCommonDir(repoRoot string) (string, error) {
	if _, err := os.Lstat(filepath.Join(repoRoot, ".git")); errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("inspect Git metadata: %w", err)
	}
	command := exec.Command("git", "-C", repoRoot, "rev-parse", "--path-format=absolute", "--git-common-dir")
	command.Env = gitutil.Environment()
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("resolve Git common directory: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
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
	if err := validateArgv("ticket_command", cfg.TicketCommand); err != nil {
		return err
	}
	if err := validateArgv("review_command", cfg.ReviewCommand); err != nil {
		return err
	}
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
		cleanPath := filepath.Clean(item.Path)
		if cleanPath == ".env.worktree" || strings.HasPrefix(cleanPath, ".env.worktree"+string(filepath.Separator)) {
			return errors.New("files.copy cannot copy the generated .env.worktree file")
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
	if cfg.Ports.Start < 1024 || cfg.Ports.End > 65535 || cfg.Ports.Start > cfg.Ports.End {
		return fmt.Errorf("ports range must be between 1024 and 65535, got %d-%d", cfg.Ports.Start, cfg.Ports.End)
	}
	serviceNames := map[string]string{}
	for _, service := range cfg.Ports.Services {
		if !validServiceName(service) {
			return fmt.Errorf("port service %q must start with a letter and contain only letters, numbers, underscores, or hyphens", service)
		}
		if len(service) > 63 {
			return fmt.Errorf("port service %q exceeds the 63-byte DNS label limit", service)
		}
		environmentName := PortEnvironmentName(service)
		if previous, exists := serviceNames[environmentName]; exists {
			return fmt.Errorf("port services %q and %q produce the same environment variable", previous, service)
		}
		serviceNames[environmentName] = service
	}
	for name := range cfg.Environment.Variables {
		if !validEnvironmentName(name) {
			return fmt.Errorf("environment variable name %q is invalid", name)
		}
		if strings.HasPrefix(name, "HWT_") {
			return fmt.Errorf("environment variable %q uses the reserved HWT_ prefix", name)
		}
	}
	if cfg.LocalDNS.Domain == "" {
		return errors.New("local_dns.domain cannot be empty")
	}
	if err := validateLocalDomain(cfg.LocalDNS.Domain); err != nil {
		return fmt.Errorf("local_dns.domain: %w", err)
	}
	if cfg.LocalDNS.Enabled && len(cfg.Ports.Services) == 0 {
		return errors.New("local_dns.enabled requires at least one ports.services entry")
	}
	if cfg.LocalDNS.Enabled {
		for _, service := range cfg.Ports.Services {
			if len(service)+1+63+1+len(cfg.LocalDNS.Domain) > 253 {
				return fmt.Errorf("local DNS hostname for service %q exceeds 253 bytes", service)
			}
		}
	}
	for _, argument := range cfg.LocalDNS.Reload {
		if argument == "" {
			return errors.New("local_dns.reload entries cannot be empty")
		}
		placeholders, err := urltemplate.PlaceholdersRaw(argument)
		if err != nil {
			return fmt.Errorf("local_dns.reload: %w", err)
		}
		for _, placeholder := range placeholders {
			if placeholder != "caddyfile" && placeholder != "dnsmasq" && placeholder != "state_dir" {
				return fmt.Errorf("local_dns.reload uses unsupported placeholder {%s}", placeholder)
			}
		}
	}
	for name, template := range cfg.URLs {
		if !validServiceName(name) {
			return fmt.Errorf("URL name %q must start with a letter and contain only letters, numbers, underscores, or hyphens", name)
		}
		if err := urltemplate.ValidateTemplate(template); err != nil {
			return fmt.Errorf("urls.%s: %w", name, err)
		}
	}
	reserved := map[string]bool{"repository": true, "branch": true, "sanitized_branch": true, "worktree": true, "hostname": true, "pr_host": true, "pr_owner": true, "pr_repository": true, "pr_number": true}
	for name := range cfg.Metadata.Values {
		if !urltemplate.ValidPlaceholder(name) {
			return fmt.Errorf("metadata value name %q must be a dot-separated identifier", name)
		}
		if reserved[name] {
			return fmt.Errorf("metadata value name %q is reserved", name)
		}
	}
	for name, command := range cfg.Metadata.Commands {
		if !validServiceName(name) {
			return fmt.Errorf("metadata command name %q must start with a letter and contain only letters, numbers, underscores, or hyphens", name)
		}
		if reserved[name] {
			return fmt.Errorf("metadata command name %q is reserved", name)
		}
		if len(command) == 0 || command[0] == "" {
			return fmt.Errorf("metadata command %q must contain an executable", name)
		}
		for _, argument := range command {
			if argument == "" {
				return fmt.Errorf("metadata command %q entries cannot be empty", name)
			}
		}
		for index, argument := range command[1:] {
			placeholders, err := urltemplate.PlaceholdersRaw(argument)
			if err != nil {
				return fmt.Errorf("metadata command %q argument %d: %w", name, index+1, err)
			}
			for _, placeholder := range placeholders {
				if placeholder != "repository" && placeholder != "branch" && placeholder != "sanitized_branch" && placeholder != "worktree" && placeholder != "hostname" {
					return fmt.Errorf("metadata command %q argument %d uses unsupported placeholder {%s}; only repository, branch, sanitized_branch, worktree, and hostname are available", name, index+1, placeholder)
				}
			}
		}
	}
	return nil
}

func validateArgv(name string, command []string) error {
	if len(command) == 0 {
		return fmt.Errorf("%s must contain an executable", name)
	}
	for _, argument := range command {
		if argument == "" {
			return fmt.Errorf("%s entries cannot be empty", name)
		}
	}
	return nil
}

func validServiceName(value string) bool {
	return serviceNamePattern.MatchString(value)
}

func validEnvironmentName(value string) bool {
	return environmentNamePattern.MatchString(value)
}

func PortEnvironmentName(value string) string {
	return "HWT_PORT_" + strings.ToUpper(strings.ReplaceAll(value, "-", "_"))
}

func URLEnvironmentName(value string) string {
	return "HWT_URL_" + strings.ToUpper(strings.ReplaceAll(value, "-", "_"))
}

func validateLocalDomain(value string) error {
	if len(value) > 189 || strings.ContainsAny(value, `/\:`) {
		return errors.New("must be a DNS name of at most 189 bytes, not a path or address")
	}
	labels := strings.Split(value, ".")
	if len(labels) < 2 || strings.EqualFold(value, "localhost") {
		return errors.New("must contain at least two DNS labels and cannot be localhost")
	}
	for _, label := range labels {
		if !dnsLabelPattern.MatchString(label) {
			return fmt.Errorf("label %q must be 1-63 letters, numbers, or interior hyphens", label)
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
	cfg := Config{WorktreeNaming: DefaultWorktreeNaming, Ports: Ports{Start: 20000, End: 39999}}
	cfg.Agent = scalar(global.Agent, project.Agent, "")
	ticketCommand := global.TicketCommand
	if project.TicketCommand != nil {
		ticketCommand = project.TicketCommand
	}
	if ticketCommand == nil {
		cfg.TicketCommand = append([]string(nil), defaultTicketCommand...)
	} else {
		cfg.TicketCommand = append([]string(nil), (*ticketCommand)...)
	}
	reviewCommand := global.ReviewCommand
	if project.ReviewCommand != nil {
		reviewCommand = project.ReviewCommand
	}
	if reviewCommand == nil {
		cfg.ReviewCommand = append([]string(nil), defaultReviewCommand...)
	} else {
		cfg.ReviewCommand = append([]string(nil), (*reviewCommand)...)
	}
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
	cfg.Ports.Start = scalar(portStart(global.Ports), portStart(project.Ports), 20000)
	cfg.Ports.End = scalar(portEnd(global.Ports), portEnd(project.Ports), 39999)
	cfg.Ports.Services = list(portServices(global.Ports), portServices(project.Ports))
	cfg.Environment.Variables = stringMap(environmentVariables(global.Environment), environmentVariables(project.Environment))
	cfg.LocalDNS.Enabled = scalar(localDNSEnabled(global.LocalDNS), localDNSEnabled(project.LocalDNS), false)
	cfg.LocalDNS.Domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(scalar(localDNSDomain(global.LocalDNS), localDNSDomain(project.LocalDNS), DefaultLocalDNSDomain)), "."))
	reload := localDNSReload(global.LocalDNS)
	if project.LocalDNS != nil && project.LocalDNS.Reload != nil {
		reload = project.LocalDNS.Reload
	}
	if reload != nil {
		cfg.LocalDNS.Reload = append([]string(nil), (*reload)...)
	}
	cfg.URLs = mergeStringMaps(&defaultURLs, global.URLs, project.URLs)
	cfg.Metadata.Values = mergeStringMaps(metadataValues(global.Metadata), metadataValues(project.Metadata))
	cfg.Metadata.Commands = mergeCommandMaps(metadataCommands(global.Metadata), metadataCommands(project.Metadata))
	return cfg
}

func metadataValues(metadata *rawMetadata) *map[string]string {
	if metadata == nil {
		return nil
	}
	return metadata.Values
}

func metadataCommands(metadata *rawMetadata) *map[string][]string {
	if metadata == nil {
		return nil
	}
	return metadata.Commands
}

func mergeStringMaps(maps ...*map[string]string) map[string]string {
	result := map[string]string{}
	for _, values := range maps {
		if values == nil {
			continue
		}
		for key, value := range *values {
			result[key] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func mergeCommandMaps(global, project *map[string][]string) map[string][]string {
	result := map[string][]string{}
	for _, source := range []*map[string][]string{global, project} {
		if source != nil {
			for key, value := range *source {
				result[key] = append([]string(nil), value...)
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func portStart(ports *rawPorts) *int {
	if ports == nil {
		return nil
	}
	return ports.Start
}
func portEnd(ports *rawPorts) *int {
	if ports == nil {
		return nil
	}
	return ports.End
}
func portServices(ports *rawPorts) *[]string {
	if ports == nil {
		return nil
	}
	return ports.Services
}
func environmentVariables(environment *rawEnvironment) *map[string]string {
	if environment == nil {
		return nil
	}
	return environment.Variables
}

func localDNSEnabled(localDNS *rawLocalDNS) *bool {
	if localDNS == nil {
		return nil
	}
	return localDNS.Enabled
}

func localDNSDomain(localDNS *rawLocalDNS) *string {
	if localDNS == nil {
		return nil
	}
	return localDNS.Domain
}

func localDNSReload(localDNS *rawLocalDNS) *[]string {
	if localDNS == nil {
		return nil
	}
	return localDNS.Reload
}

func stringMap(global, project *map[string]string) map[string]string {
	selected := global
	if project != nil {
		selected = project
	}
	if selected == nil {
		return nil
	}
	result := make(map[string]string, len(*selected))
	for key, value := range *selected {
		result[key] = value
	}
	return result
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
	if services := portServices(cfg.Ports); services != nil {
		for _, service := range *services {
			if service == GlobalMarker {
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
