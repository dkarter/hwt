// Package localdns manages the HWT-owned dnsmasq and Caddy configuration used
// to expose worktree services under stable local hostnames. It does not install
// tools, edit system configuration, or manage services.
package localdns

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/dkarter/hwt/internal/urltemplate"
	"golang.org/x/sys/unix"
)

const (
	registryName = "registry.json"
	dnsmasqName  = "dnsmasq.conf"
	caddyName    = "Caddyfile"
	lockName     = ".lock"
)

var currentGOOS = func() string { return runtime.GOOS }

// Config controls local DNS state and the optional command used to reload the
// programs consuming the generated files.
type Config struct {
	Enabled bool     `json:"enabled"`
	Domain  string   `json:"domain"`
	Reload  []string `json:"reload,omitempty"`
}

// Registration describes one worktree and its HTTP services.
type Registration struct {
	Worktree   string         `json:"worktree"`
	Repository string         `json:"repository"`
	Services   map[string]int `json:"services"`
}

// Entry is the persisted, canonical representation of a registration.
type Entry struct {
	Worktree   string         `json:"worktree"`
	Repository string         `json:"repository"`
	Domain     string         `json:"domain"`
	Hostname   string         `json:"hostname"`
	Services   map[string]int `json:"services"`
}

// Paths identifies every hwt-owned local DNS file.
type Paths struct {
	StateDir  string `json:"state_dir"`
	Registry  string `json:"registry"`
	DNSMasq   string `json:"dnsmasq"`
	Caddyfile string `json:"caddyfile"`
}

// Result describes a registration and the URLs assigned to its services.
type Result struct {
	Hostname string            `json:"hostname"`
	URLs     map[string]string `json:"urls"`
	Paths    Paths             `json:"paths"`
}

// SetupResult includes paths and instructions for wiring the owned snippets
// into dnsmasq and Caddy. The package never edits system configuration.
type SetupResult struct {
	Paths          Paths  `json:"paths"`
	DNSMasqInclude string `json:"dnsmasq_include"`
	CaddyImport    string `json:"caddy_import"`
}

// StatusResult is a side-effect-free snapshot of local DNS state.
type StatusResult struct {
	Paths   Paths   `json:"paths"`
	Entries []Entry `json:"entries"`
}

type registryFile struct {
	Entries map[string]Entry `json:"entries"`
}

type snapshot struct {
	exists bool
	data   []byte
}

// Hostname returns the deterministic base hostname for existing repository and
// worktree directories without reading or changing local DNS state.
func Hostname(repository, worktree, domain string) (string, error) {
	if err := supportedPlatform(); err != nil {
		return "", err
	}
	domain, err := validateDomain(domain)
	if err != nil {
		return "", err
	}
	repository, err = canonicalPath(repository, "repository")
	if err != nil {
		return "", err
	}
	worktree, err = canonicalPath(worktree, "worktree")
	if err != nil {
		return "", err
	}
	return baseHostname(repository, worktree, domain), nil
}

// Register adds or replaces a worktree registration.
func Register(cfg Config, registration Registration) (Result, error) {
	paths, domain, err := prepare(cfg, true)
	if err != nil {
		return Result{}, err
	}
	worktree, err := canonicalPath(registration.Worktree, "worktree")
	if err != nil {
		return Result{}, err
	}
	repository, err := canonicalPath(registration.Repository, "repository")
	if err != nil {
		return Result{}, err
	}
	services, err := validateServices(registration.Services, domain)
	if err != nil {
		return Result{}, err
	}
	hostname := baseHostname(repository, worktree, domain)
	entry := Entry{Worktree: worktree, Repository: repository, Domain: domain, Hostname: hostname, Services: services}

	err = mutate(paths, cfg.Reload, domain, func(registry *registryFile) error {
		for owner, existing := range registry.Entries {
			if owner != worktree && existing.Hostname == hostname {
				return fmt.Errorf("hostname %q is already registered by worktree %q", hostname, owner)
			}
		}
		registry.Entries[worktree] = entry
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	urls := make(map[string]string, len(services))
	for service := range registration.Services {
		urls[service] = "http://" + sanitize(service, "") + "." + hostname
	}
	return Result{Hostname: hostname, URLs: urls, Paths: paths}, nil
}

// Release removes the registration whose canonical worktree path is root.
func Release(root string, cfg Config) error {
	paths, domain, err := prepare(cfg, true)
	if err != nil {
		return err
	}
	root, err = releasablePath(root)
	if err != nil {
		return err
	}
	return mutate(paths, cfg.Reload, domain, func(registry *registryFile) error {
		delete(registry.Entries, root)
		return nil
	})
}

// Setup creates empty owned state and generated snippets. It is idempotent.
func Setup(cfg Config) (SetupResult, error) {
	paths, domain, err := prepare(cfg, true)
	if err != nil {
		return SetupResult{}, err
	}
	if err := requireTools(); err != nil {
		return SetupResult{}, err
	}
	if err := mutate(paths, cfg.Reload, domain, func(*registryFile) error { return nil }); err != nil {
		return SetupResult{}, err
	}
	return SetupResult{
		Paths:          paths,
		DNSMasqInclude: "Add `conf-file=" + paths.DNSMasq + "` to your user-managed dnsmasq configuration.",
		CaddyImport:    "Add `import " + paths.Caddyfile + "` to your user-managed Caddy configuration.",
	}, nil
}

// Status reads current registrations without creating or changing files.
func Status(cfg Config) (StatusResult, error) {
	paths, _, err := prepare(cfg, false)
	if err != nil {
		return StatusResult{}, err
	}
	registry, err := readRegistry(paths.Registry)
	if err != nil {
		return StatusResult{}, err
	}
	entries := make([]Entry, 0, len(registry.Entries))
	for _, entry := range registry.Entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Worktree < entries[j].Worktree })
	return StatusResult{Paths: paths, Entries: entries}, nil
}

// Teardown removes only hwt-owned files. Existing registrations require force.
func Teardown(cfg Config, force bool) error {
	paths, _, err := prepare(cfg, false)
	if err != nil {
		return err
	}
	if _, err := os.Stat(paths.StateDir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect local DNS state directory: %w", err)
	}
	return withLock(paths, func() error {
		registry, err := readRegistry(paths.Registry)
		if err != nil {
			return err
		}
		if len(registry.Entries) != 0 && !force {
			return fmt.Errorf("refusing to tear down local DNS with %d registered worktree(s); release them or use force", len(registry.Entries))
		}
		files := []string{paths.Registry, paths.DNSMasq, paths.Caddyfile}
		before, err := capture(files)
		if err != nil {
			return err
		}
		changed := false
		for _, path := range files {
			if err := os.Remove(path); err == nil {
				changed = true
			} else if !errors.Is(err, os.ErrNotExist) {
				restore(files, before)
				return fmt.Errorf("remove %s: %w", path, err)
			}
		}
		if changed {
			if err := reload(cfg.Reload, paths); err != nil {
				if rollbackErr := restore(files, before); rollbackErr != nil {
					return fmt.Errorf("%w; additionally failed to restore state: %v", err, rollbackErr)
				}
				return err
			}
		}
		return nil
	})
}

func prepare(cfg Config, requireEnabled bool) (Paths, string, error) {
	if err := supportedPlatform(); err != nil {
		return Paths{}, "", err
	}
	if requireEnabled && !cfg.Enabled {
		return Paths{}, "", errors.New("local DNS is disabled; enable it in configuration before continuing")
	}
	domain, err := validateDomain(cfg.Domain)
	if err != nil {
		return Paths{}, "", err
	}
	stateRoot := os.Getenv("XDG_STATE_HOME")
	if stateRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, "", fmt.Errorf("resolve home directory for local DNS state: %w", err)
		}
		stateRoot = filepath.Join(home, ".local", "state")
	}
	stateDir := filepath.Join(stateRoot, "hwt", "local-dns")
	return Paths{StateDir: stateDir, Registry: filepath.Join(stateDir, registryName), DNSMasq: filepath.Join(stateDir, dnsmasqName), Caddyfile: filepath.Join(stateDir, caddyName)}, domain, nil
}

func supportedPlatform() error {
	if goos := currentGOOS(); goos != "darwin" && goos != "linux" {
		return fmt.Errorf("local DNS is unsupported on %s; only darwin and linux are supported", goos)
	}
	return nil
}

func validateDomain(value string) (string, error) {
	value = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
	if value == "" {
		return "", errors.New("local DNS domain is required")
	}
	if value == "localhost" || strings.ContainsAny(value, `/\\:`) {
		return "", fmt.Errorf("local DNS domain %q is invalid; use DNS labels such as hwt.test (not localhost or a path)", value)
	}
	if !strings.Contains(value, ".") {
		return "", fmt.Errorf("local DNS domain %q is invalid; use at least two labels such as hwt.test", value)
	}
	// Leave room for the maximum 63-byte generated base label and its dot.
	if len(value) > 189 {
		return "", fmt.Errorf("local DNS domain is %d bytes; maximum is 189 so generated hostnames remain within 253 bytes", len(value))
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("local DNS domain %q contains an invalid DNS label", value)
		}
		for _, character := range label {
			if !(character >= 'a' && character <= 'z') && !(character >= '0' && character <= '9') && character != '-' {
				return "", fmt.Errorf("local DNS domain %q contains an invalid DNS label", value)
			}
		}
	}
	return value, nil
}

func canonicalPath(path, kind string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("%s path is required", kind)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute %s path: %w", kind, err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("canonicalize %s path %q: %w", kind, path, err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("inspect %s path %q: %w", kind, path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s path %q is not a directory", kind, path)
	}
	return filepath.Clean(canonical), nil
}

func releasablePath(path string) (string, error) {
	if path == "" {
		return "", errors.New("worktree path is required")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("worktree path %q must be a previously canonical absolute path when releasing", path)
	}
	canonical, err := canonicalPath(path, "worktree")
	if err == nil {
		return canonical, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	// Worktree removal intentionally happens before registry release. At that
	// point symlinks cannot be resolved, so the caller's canonical path is key.
	return filepath.Clean(path), nil
}

func requireTools() error {
	var missing []string
	for _, tool := range []string{"caddy", "dnsmasq"} {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}
	if len(missing) != 0 {
		return fmt.Errorf("required local DNS tools are missing from PATH: %s; install them before setup (hwt does not install or start system services)", strings.Join(missing, ", "))
	}
	return nil
}

func sanitize(value, fallback string) string {
	value = strings.ToLower(value)
	var result strings.Builder
	hyphen := false
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			result.WriteRune(character)
			hyphen = false
		} else if result.Len() > 0 && !hyphen {
			result.WriteByte('-')
			hyphen = true
		}
	}
	clean := strings.Trim(result.String(), "-")
	if clean == "" {
		return fallback
	}
	return clean
}

func baseHostname(repository, worktree, domain string) string {
	repo := sanitize(filepath.Base(repository), "repo")
	work := sanitize(filepath.Base(worktree), "worktree")
	digest := sha256.Sum256([]byte(repository + "\x00" + worktree))
	suffix := hex.EncodeToString(digest[:])[:12]
	prefixBudget := 63 - len(suffix) - 1
	prefix := repo + "-" + work
	if len(prefix) > prefixBudget {
		prefix = strings.Trim(prefix[:prefixBudget], "-")
	}
	return prefix + "-" + suffix + "." + domain
}

func validateServices(input map[string]int, domain string) (map[string]int, error) {
	if len(input) == 0 {
		return nil, errors.New("at least one local DNS service is required")
	}
	result := make(map[string]int, len(input))
	for original, port := range input {
		name := sanitize(original, "")
		if name == "" || len(name) > 63 {
			return nil, fmt.Errorf("service name %q does not produce a valid DNS label of at most 63 bytes", original)
		}
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("service name %q conflicts with another service after sanitization as %q", original, name)
		}
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("service %q port %d is invalid; use 1 through 65535", original, port)
		}
		// One dot separates the service label from the maximum-length base label.
		if len(name)+1+63+1+len(domain) > 253 {
			return nil, fmt.Errorf("service hostname for %q would exceed the 253-byte DNS limit", original)
		}
		result[name] = port
	}
	return result, nil
}

func mutate(paths Paths, argv []string, domain string, update func(*registryFile) error) error {
	if err := os.MkdirAll(paths.StateDir, 0o700); err != nil {
		return fmt.Errorf("create local DNS state directory: %w", err)
	}
	return withLock(paths, func() error {
		registry, err := readRegistry(paths.Registry)
		if err != nil {
			return err
		}
		if err := update(&registry); err != nil {
			return err
		}
		registryBytes, dnsmasqBytes, caddyBytes, err := render(registry, domain)
		if err != nil {
			return err
		}
		files := []string{paths.Registry, paths.DNSMasq, paths.Caddyfile}
		contents := [][]byte{registryBytes, dnsmasqBytes, caddyBytes}
		before, err := capture(files)
		if err != nil {
			return err
		}
		changed := false
		for index, path := range files {
			if before[index].exists && bytes.Equal(before[index].data, contents[index]) {
				if err := os.Chmod(path, 0o600); err != nil {
					return fmt.Errorf("set permissions on %s: %w", path, err)
				}
				continue
			}
			changed = true
			if err := atomicWrite(path, contents[index]); err != nil {
				restore(files, before)
				return err
			}
		}
		if changed {
			if err := reload(argv, paths); err != nil {
				if rollbackErr := restore(files, before); rollbackErr != nil {
					return fmt.Errorf("%w; additionally failed to restore state: %v", err, rollbackErr)
				}
				return err
			}
		}
		return nil
	})
}

func withLock(paths Paths, fn func() error) error {
	file, err := os.OpenFile(filepath.Join(paths.StateDir, lockName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open local DNS state lock: %w", err)
	}
	defer file.Close()
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		return fmt.Errorf("lock local DNS state: %w", err)
	}
	defer unix.Flock(int(file.Fd()), unix.LOCK_UN)
	return fn()
}

func readRegistry(path string) (registryFile, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return registryFile{Entries: map[string]Entry{}}, nil
	}
	if err != nil {
		return registryFile{}, fmt.Errorf("read local DNS registry: %w", err)
	}
	var registry registryFile
	if err := json.Unmarshal(data, &registry); err != nil {
		return registryFile{}, fmt.Errorf("decode local DNS registry: %w", err)
	}
	if registry.Entries == nil {
		registry.Entries = map[string]Entry{}
	}
	return registry, nil
}

func render(registry registryFile, configuredDomain string) ([]byte, []byte, []byte, error) {
	registryBytes, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode local DNS registry: %w", err)
	}
	registryBytes = append(registryBytes, '\n')
	domains := map[string]bool{configuredDomain: true}
	entries := make([]Entry, 0, len(registry.Entries))
	for _, entry := range registry.Entries {
		domains[entry.Domain] = true
		entries = append(entries, entry)
	}
	sortedDomains := make([]string, 0, len(domains))
	for domain := range domains {
		sortedDomains = append(sortedDomains, domain)
	}
	sort.Strings(sortedDomains)
	var dnsmasq strings.Builder
	dnsmasq.WriteString("# Generated by hwt. Do not edit.\n")
	for _, domain := range sortedDomains {
		fmt.Fprintf(&dnsmasq, "address=/.%s/127.0.0.1\n", domain)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Hostname < entries[j].Hostname })
	var caddy strings.Builder
	caddy.WriteString("# Generated by hwt. Do not edit.\n")
	for _, entry := range entries {
		services := make([]string, 0, len(entry.Services))
		for service := range entry.Services {
			services = append(services, service)
		}
		sort.Strings(services)
		for _, service := range services {
			fmt.Fprintf(&caddy, "http://%s.%s {\n\treverse_proxy 127.0.0.1:%d\n}\n\n", service, entry.Hostname, entry.Services[service])
		}
	}
	return registryBytes, []byte(dnsmasq.String()), []byte(caddy.String()), nil
}

func atomicWrite(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".hwt-localdns-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("set permissions on temporary file for %s: %w", path, err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary file for %s: %w", path, err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync temporary file for %s: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file for %s: %w", path, err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("replace %s atomically: %w", path, err)
	}
	return nil
}

func capture(paths []string) ([]snapshot, error) {
	result := make([]snapshot, len(paths))
	for index, path := range paths {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("snapshot %s: %w", path, err)
		}
		result[index] = snapshot{exists: true, data: data}
	}
	return result, nil
}

func restore(paths []string, snapshots []snapshot) error {
	var failures []string
	for index, path := range paths {
		var err error
		if snapshots[index].exists {
			err = atomicWrite(path, snapshots[index].data)
		} else {
			err = os.Remove(path)
			if errors.Is(err, os.ErrNotExist) {
				err = nil
			}
		}
		if err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) != 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func reload(argv []string, paths Paths) error {
	if len(argv) == 0 {
		return nil
	}
	values := map[string]string{"caddyfile": paths.Caddyfile, "dnsmasq": paths.DNSMasq, "state_dir": paths.StateDir}
	expanded := make([]string, len(argv))
	for index, argument := range argv {
		var err error
		expanded[index], err = urltemplate.ExpandRaw(argument, values)
		if err != nil {
			return fmt.Errorf("expand local DNS reload argument %d: %w", index, err)
		}
	}
	command := exec.Command(expanded[0], expanded[1:]...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("reload executable %q was not found; install it or update local DNS reload configuration: %w", expanded[0], err)
		}
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return fmt.Errorf("local DNS reload command %q failed: %s: %w", strings.Join(expanded, " "), detail, err)
		}
		return fmt.Errorf("local DNS reload command %q failed: %w", strings.Join(expanded, " "), err)
	}
	return nil
}
