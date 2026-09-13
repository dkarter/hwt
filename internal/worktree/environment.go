package worktree

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dkarter/hwt/internal/config"
	"github.com/dkarter/hwt/internal/localdns"
	"github.com/dkarter/hwt/internal/urltemplate"
	"golang.org/x/sys/unix"
)

const environmentFileName = ".env.worktree"

type EnvironmentResult struct {
	Path      string            `json:"path"`
	Variables map[string]string `json:"variables"`
}

type portRegistry struct {
	Version   int                       `json:"version"`
	Worktrees map[string]portAllocation `json:"worktrees"`
}

type portAllocation struct {
	Ports map[string]int `json:"ports"`
}

func Environment(cwd string, refresh bool) (EnvironmentResult, error) {
	gitDir, commonDir, root, err := metadata(cwd)
	if err != nil {
		return EnvironmentResult{}, fmt.Errorf("resolve worktree environment: %w", err)
	}
	if samePath(gitDir, commonDir) {
		return EnvironmentResult{}, errors.New("worktree environment is only available in a linked worktree")
	}
	source, err := primaryWorktree(root)
	if err != nil {
		return EnvironmentResult{}, err
	}
	cfg, _, err := config.Load(source, commonDir)
	if err != nil {
		return EnvironmentResult{}, err
	}
	return prepareEnvironment(root, cfg, refresh)
}

func prepareEnvironment(root string, cfg config.Config, refresh bool) (EnvironmentResult, error) {
	branch, err := gitOutput(root, "branch", "--show-current")
	if err != nil {
		return EnvironmentResult{}, err
	}
	if err := excludeEnvironmentFile(root); err != nil {
		return EnvironmentResult{}, err
	}
	result := EnvironmentResult{Path: filepath.Join(root, environmentFileName)}
	_, err = reservePorts(root, cfg.Ports, refresh, func(ports map[string]int) error {
		previousEnvironment, previousEnvironmentErr := os.ReadFile(result.Path)
		previousEnvironmentExists := previousEnvironmentErr == nil
		if previousEnvironmentErr != nil && !errors.Is(previousEnvironmentErr, os.ErrNotExist) {
			return fmt.Errorf("read existing worktree environment: %w", previousEnvironmentErr)
		}
		var previousDNS *localdns.Entry
		if cfg.LocalDNS.Enabled {
			status, statusErr := localdns.Status(localDNSConfig(cfg))
			if statusErr != nil {
				return fmt.Errorf("inspect existing local DNS route: %w", statusErr)
			}
			canonical, canonicalErr := canonicalWorktreePath(root)
			if canonicalErr != nil {
				return canonicalErr
			}
			for _, entry := range status.Entries {
				if entry.Worktree == canonical {
					entryCopy := entry
					previousDNS = &entryCopy
					break
				}
			}
		}
		variables := map[string]string{
			"HWT_ENV_FILE":        result.Path,
			"HWT_WORKTREE_BRANCH": branch,
			"HWT_WORKTREE_PATH":   root,
		}
		for service, port := range ports {
			variables[config.PortEnvironmentName(service)] = strconv.Itoa(port)
		}
		generated := make(map[string]string, len(variables))
		for name, value := range variables {
			generated[name] = value
		}
		for name, value := range cfg.Environment.Variables {
			unknown := ""
			expanded := os.Expand(value, func(key string) string {
				resolved, exists := generated[key]
				if !exists {
					unknown = key
				}
				return resolved
			})
			if unknown != "" {
				return fmt.Errorf("environment variable %s references unknown generated variable %s", name, unknown)
			}
			variables[name] = expanded
		}
		if cfg.LocalDNS.Enabled {
			repository, err := primaryWorktree(root)
			if err != nil {
				return err
			}
			registration, err := localdns.Register(localDNSConfig(cfg), localdns.Registration{Worktree: root, Repository: repository, Services: ports})
			if err != nil {
				return fmt.Errorf("register local DNS routes: %w", err)
			}
			variables["HWT_WORKTREE_HOSTNAME"] = registration.Hostname
			for service, localURL := range registration.URLs {
				variables[config.URLEnvironmentName(service)] = localURL
			}
		} else {
			hostname := localhostHostname(root)
			variables["HWT_WORKTREE_HOSTNAME"] = hostname
			for service, port := range ports {
				serviceLabel := urltemplate.SanitizeBranch(service)
				variables[config.URLEnvironmentName(service)] = fmt.Sprintf("http://%s.%s:%d", serviceLabel, hostname, port)
			}
			if err := releaseLocalDNS(root, cfg); err != nil {
				return err
			}
		}
		if err := writeEnvironmentFile(result.Path, variables); err != nil {
			var rollbackErr error
			if cfg.LocalDNS.Enabled {
				if previousDNS == nil {
					rollbackErr = localdns.Release(root, localDNSConfig(cfg))
				} else {
					rollbackConfig := localDNSConfig(cfg)
					rollbackConfig.Domain = previousDNS.Domain
					_, rollbackErr = localdns.Register(rollbackConfig, localdns.Registration{Worktree: previousDNS.Worktree, Repository: previousDNS.Repository, Services: previousDNS.Services})
				}
			}
			restoreErr := restoreEnvironmentFile(result.Path, previousEnvironment, previousEnvironmentExists)
			return errors.Join(err, rollbackErr, restoreErr)
		}
		result.Variables = variables
		return nil
	})
	if err != nil {
		return EnvironmentResult{}, err
	}
	return result, nil
}

func localhostHostname(root string) string {
	label := urltemplate.SanitizeBranch(filepath.Base(root))
	if label == "" {
		label = "worktree"
	}
	return label + ".localhost"
}

func localDNSConfig(cfg config.Config) localdns.Config {
	return localdns.Config{Enabled: cfg.LocalDNS.Enabled, Domain: cfg.LocalDNS.Domain, Reload: cfg.LocalDNS.Reload}
}

func releaseLocalDNS(root string, cfg config.Config) error {
	status, err := localdns.Status(localDNSConfig(cfg))
	if err != nil {
		return fmt.Errorf("inspect local DNS routes: %w", err)
	}
	canonical, err := canonicalWorktreePath(root)
	if err != nil {
		return err
	}
	for _, entry := range status.Entries {
		if entry.Worktree != canonical {
			continue
		}
		localConfig := localDNSConfig(cfg)
		localConfig.Enabled = true
		localConfig.Domain = entry.Domain
		if err := localdns.Release(canonical, localConfig); err != nil {
			return fmt.Errorf("release local DNS routes: %w", err)
		}
		break
	}
	return nil
}

func restoreEnvironmentFile(path string, content []byte, existed bool) error {
	if existed {
		return os.WriteFile(path, content, 0o600)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func excludeEnvironmentFile(root string) error {
	tracked, err := gitOutput(root, "ls-files", "--error-unmatch", "--", environmentFileName)
	if err == nil && tracked != "" {
		return fmt.Errorf("refusing to overwrite tracked %s", environmentFileName)
	}
	_, commonDir, _, err := metadata(root)
	if err != nil {
		return err
	}
	exclude := filepath.Join(commonDir, "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		return fmt.Errorf("create Git exclude directory: %w", err)
	}
	file, err := os.OpenFile(exclude, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open Git exclude file: %w", err)
	}
	defer file.Close()
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		return fmt.Errorf("lock Git exclude file: %w", err)
	}
	defer unix.Flock(int(file.Fd()), unix.LOCK_UN) //nolint:errcheck
	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("read Git exclude file: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "/"+environmentFileName {
			return nil
		}
	}
	prefix := ""
	if len(data) > 0 && data[len(data)-1] != '\n' {
		prefix = "\n"
	}
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	_, err = fmt.Fprintf(file, "%s/%s\n", prefix, environmentFileName)
	return err
}

func writeEnvironmentFile(path string, variables map[string]string) error {
	names := make([]string, 0, len(variables))
	for name := range variables {
		names = append(names, name)
	}
	sort.Strings(names)
	var content strings.Builder
	content.WriteString("# Generated by hwt. Do not edit or commit.\n")
	for _, name := range names {
		fmt.Fprintf(&content, "%s=%s\n", name, dotenvQuote(variables[name]))
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".env.worktree.*")
	if err != nil {
		return fmt.Errorf("create worktree environment: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.WriteString(content.String()); err != nil {
		temporary.Close()
		return fmt.Errorf("write worktree environment: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish worktree environment: %w", err)
	}
	return nil
}

func dotenvQuote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "`", "\\`", "\n", `\n`, "\r", `\r`).Replace(value) + `"`
}

func reservePorts(root string, configured config.Ports, refresh bool, publish func(map[string]int) error) (map[string]int, error) {
	if len(configured.Services) == 0 {
		ports := map[string]int{}
		if publish != nil {
			if err := publish(ports); err != nil {
				return nil, err
			}
		}
		if err := releasePorts(root); err != nil {
			return nil, err
		}
		return ports, nil
	}
	dir, err := stateDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create hwt state directory: %w", err)
	}
	lock, err := os.OpenFile(filepath.Join(dir, "ports.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open port registry lock: %w", err)
	}
	defer lock.Close()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		return nil, fmt.Errorf("lock port registry: %w", err)
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN) //nolint:errcheck

	path := filepath.Join(dir, "ports.json")
	_, statErr := os.Stat(path)
	previousRegistryExists := statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect port registry: %w", statErr)
	}
	registry, err := readPortRegistry(path)
	if err != nil {
		return nil, err
	}
	previousRegistry := clonePortRegistry(registry)
	canonical, err := canonicalWorktreePath(root)
	if err != nil {
		return nil, err
	}
	if !refresh {
		if existing, ok := registry.Worktrees[canonical]; ok && allocationMatches(existing.Ports, configured) {
			if publish != nil {
				if err := publish(existing.Ports); err != nil {
					return nil, err
				}
			}
			return existing.Ports, nil
		}
	}
	for worktree := range registry.Worktrees {
		if _, err := os.Stat(worktree); errors.Is(err, os.ErrNotExist) {
			delete(registry.Worktrees, worktree)
		}
	}
	if refresh {
		delete(registry.Worktrees, canonical)
	}
	allocation := registry.Worktrees[canonical]
	if allocation.Ports == nil {
		allocation.Ports = map[string]int{}
	}
	requested := make(map[string]bool, len(configured.Services))
	for _, service := range configured.Services {
		requested[service] = true
	}
	for service := range allocation.Ports {
		if !requested[service] {
			delete(allocation.Ports, service)
		}
	}
	used := map[int]bool{}
	for worktree, existing := range registry.Worktrees {
		if worktree == canonical {
			continue
		}
		for _, port := range existing.Ports {
			used[port] = true
		}
	}
	for _, service := range configured.Services {
		if port := allocation.Ports[service]; port >= configured.Start && port <= configured.End && !used[port] {
			used[port] = true
			continue
		}
		port := 0
		for candidate := configured.Start; candidate <= configured.End; candidate++ {
			if !used[candidate] && portAvailable(candidate) {
				port = candidate
				break
			}
		}
		if port == 0 {
			return nil, fmt.Errorf("no available ports in configured range %d-%d", configured.Start, configured.End)
		}
		allocation.Ports[service] = port
		used[port] = true
	}
	if len(allocation.Ports) == 0 {
		delete(registry.Worktrees, canonical)
	} else {
		registry.Worktrees[canonical] = allocation
	}
	if err := writePortRegistry(path, registry); err != nil {
		return nil, err
	}
	if publish != nil {
		if err := publish(allocation.Ports); err != nil {
			var rollbackErr error
			if previousRegistryExists {
				rollbackErr = writePortRegistry(path, previousRegistry)
			} else if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				rollbackErr = removeErr
			}
			if rollbackErr != nil {
				return nil, errors.Join(err, fmt.Errorf("restore port registry: %w", rollbackErr))
			}
			return nil, err
		}
	}
	return allocation.Ports, nil
}

func clonePortRegistry(registry portRegistry) portRegistry {
	cloned := portRegistry{Version: registry.Version, Worktrees: make(map[string]portAllocation, len(registry.Worktrees))}
	for worktree, allocation := range registry.Worktrees {
		ports := make(map[string]int, len(allocation.Ports))
		for service, port := range allocation.Ports {
			ports[service] = port
		}
		cloned.Worktrees[worktree] = portAllocation{Ports: ports}
	}
	return cloned
}

func allocationMatches(ports map[string]int, configured config.Ports) bool {
	if len(ports) != len(configured.Services) {
		return false
	}
	for _, service := range configured.Services {
		port, exists := ports[service]
		if !exists || port < configured.Start || port > configured.End {
			return false
		}
	}
	return true
}

func canonicalWorktreePath(root string) (string, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	evaluated, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return evaluated, nil
	}
	return filepath.Clean(absolute), nil
}

func portAvailable(port int) bool {
	listener, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	return listener.Close() == nil
}

func stateDir() (string, error) {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "hwt"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "state", "hwt"), nil
}

func readPortRegistry(path string) (portRegistry, error) {
	registry := portRegistry{Version: 1, Worktrees: map[string]portAllocation{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return registry, nil
	}
	if err != nil {
		return registry, fmt.Errorf("read port registry: %w", err)
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		return registry, fmt.Errorf("decode port registry: %w", err)
	}
	if registry.Version != 1 || registry.Worktrees == nil {
		return registry, fmt.Errorf("unsupported port registry version %d", registry.Version)
	}
	return registry, nil
}

func writePortRegistry(path string, registry portRegistry) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".ports.*")
	if err != nil {
		return fmt.Errorf("create port registry: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(registry); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func releasePorts(root string) error {
	dir, err := stateDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "ports.json")
	lock, err := os.OpenFile(filepath.Join(dir, "ports.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN) //nolint:errcheck
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	registry, err := readPortRegistry(path)
	if err != nil {
		return err
	}
	canonical, err := canonicalWorktreePath(root)
	if err != nil {
		return err
	}
	delete(registry.Worktrees, canonical)
	return writePortRegistry(path, registry)
}

func RunWithEnvironment(result EnvironmentResult, command []string) error {
	if len(command) == 0 {
		return errors.New("command is required")
	}
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = filepath.Dir(result.Path)
	cmd.Env = environmentList(result.Variables)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func environmentList(variables map[string]string) []string {
	merged := map[string]string{}
	for _, entry := range os.Environ() {
		name, value, _ := strings.Cut(entry, "=")
		merged[name] = value
	}
	for name, value := range variables {
		merged[name] = value
	}
	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, name+"="+merged[name])
	}
	return result
}
