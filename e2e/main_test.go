package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var (
	hwtBinary       string
	devHWTBinary    string
	specLinksBinary string
	repoRoot        string
	gitBinary       string
)

func TestMain(m *testing.M) {
	if runtime.GOOS == "windows" {
		fmt.Fprintln(os.Stderr, "e2e suite requires POSIX process semantics")
		os.Exit(0)
	}
	var err error
	gitBinary, err = exec.LookPath("git")
	if err != nil {
		fmt.Fprintln(os.Stderr, "git is required:", err)
		os.Exit(1)
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	repoRoot = filepath.Dir(cwd)
	buildDir, err := os.MkdirTemp("", "hwt-e2e-build-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	hwtBinary = filepath.Join(buildDir, "hwt")
	devHWTBinary = filepath.Join(buildDir, "hwt-dev")
	specLinksBinary = filepath.Join(buildDir, "spec-links")
	goBinary, err := exec.LookPath("go")
	if err != nil {
		fmt.Fprintln(os.Stderr, "go is required:", err)
		os.Exit(1)
	}
	moduleCacheCommand := exec.Command(goBinary, "env", "GOMODCACHE")
	moduleCacheCommand.Env = []string{"HOME=" + os.Getenv("HOME"), "GOTOOLCHAIN=local", "PATH=" + filepath.Dir(goBinary)}
	moduleCache, err := moduleCacheCommand.Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve Go module cache:", err)
		os.Exit(1)
	}
	build := exec.Command(goBinary, "build", "-ldflags", "-X main.version=1.2.3", "-o", hwtBinary, "./cmd/hwt")
	build.Dir = repoRoot
	build.Env = []string{
		"HOME=" + buildDir,
		"GOCACHE=" + filepath.Join(buildDir, "go-cache"),
		"GOMODCACHE=" + strings.TrimSpace(string(moduleCache)),
		"GOTOOLCHAIN=local",
		"GOPROXY=off",
		"CGO_ENABLED=0",
		"PATH=" + filepath.Dir(goBinary),
		"TMPDIR=" + buildDir,
	}
	if output, buildErr := build.CombinedOutput(); buildErr != nil {
		fmt.Fprintf(os.Stderr, "build hwt: %s: %v\n", output, buildErr)
		_ = os.RemoveAll(buildDir)
		os.Exit(1)
	}
	for output, target := range map[string]string{devHWTBinary: "./cmd/hwt", specLinksBinary: "./cmd/spec-links"} {
		command := exec.Command(goBinary, "build", "-o", output, target)
		command.Dir = repoRoot
		command.Env = build.Env
		if result, buildErr := command.CombinedOutput(); buildErr != nil {
			fmt.Fprintf(os.Stderr, "build %s: %s: %v\n", target, result, buildErr)
			_ = os.RemoveAll(buildDir)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(buildDir)
	os.Exit(code)
}

type sandbox struct {
	t      *testing.T
	root   string
	home   string
	bin    string
	state  string
	config string
	env    []string
}

func newSandbox(t *testing.T) *sandbox {
	t.Helper()
	root := t.TempDir()
	s := &sandbox{
		t:      t,
		root:   root,
		home:   filepath.Join(root, "home"),
		bin:    filepath.Join(root, "bin"),
		state:  filepath.Join(root, "state"),
		config: filepath.Join(root, "config"),
	}
	for _, dir := range []string{s.home, s.bin, s.state, s.config, filepath.Join(root, "cache")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(gitBinary, filepath.Join(s.bin, "git")); err != nil {
		t.Fatal(err)
	}
	global := filepath.Join(root, "gitconfig")
	mustWrite(t, global, "[user]\n\tname = HWT E2E\n\temail = hwt-e2e@example.invalid\n[init]\n\tdefaultBranch = main\n", 0o600)
	s.env = []string{
		"HOME=" + s.home,
		"XDG_CONFIG_HOME=" + s.config,
		"XDG_STATE_HOME=" + s.state,
		"XDG_CACHE_HOME=" + filepath.Join(root, "cache"),
		"GIT_CONFIG_GLOBAL=" + global,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"PATH=" + s.bin,
		"LC_ALL=C",
		"LANG=C",
		"TMPDIR=" + root,
	}
	return s
}

func (s *sandbox) command(cwd string, args ...string) (string, string, error) {
	s.t.Helper()
	return s.commandBinary(hwtBinary, cwd, args...)
}

func (s *sandbox) commandBinary(binary, cwd string, args ...string) (string, string, error) {
	s.t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = cwd
	cmd.Env = append([]string(nil), s.env...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func (s *sandbox) run(cwd string, args ...string) string {
	s.t.Helper()
	stdout, stderr, err := s.command(cwd, args...)
	if err != nil {
		s.t.Fatalf("hwt %s: %s%s: %v", strings.Join(args, " "), stdout, stderr, err)
	}
	return stdout
}

func (s *sandbox) git(cwd string, args ...string) string {
	s.t.Helper()
	cmd := exec.Command(gitBinary, append([]string{"-C", cwd}, args...)...)
	cmd.Env = append([]string(nil), s.env...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.t.Fatalf("git %s: %s: %v", strings.Join(args, " "), output, err)
	}
	return strings.TrimSpace(string(output))
}

func (s *sandbox) repo() string {
	s.t.Helper()
	repo := filepath.Join(s.root, "repo")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		s.t.Fatal(err)
	}
	s.git(repo, "init", "-b", "main")
	mustWrite(s.t, filepath.Join(repo, "README"), "initial\n", 0o600)
	s.git(repo, "add", "README")
	s.git(repo, "commit", "-m", "initial")
	return repo
}

func (s *sandbox) linked(repo, branch string) string {
	s.t.Helper()
	path := filepath.Join(s.root, strings.NewReplacer("/", "-", " ", "-").Replace(branch))
	s.git(repo, "worktree", "add", "-b", branch, path, "main")
	return path
}

func (s *sandbox) tool(name, body string) string {
	s.t.Helper()
	path := filepath.Join(s.bin, name)
	mustWrite(s.t, path, "#!/bin/sh\nset -eu\n"+body+"\n", 0o700)
	return path
}

func (s *sandbox) fakeHerdr(repo string) string {
	s.t.Helper()
	state := filepath.Join(s.root, "herdr-worktree")
	log := filepath.Join(s.root, "herdr.log")
	s.env = append(s.env, "HWT_FAKE_SOURCE="+repo, "HWT_FAKE_STATE="+state, "HWT_FAKE_LOG="+log)
	return s.tool("herdr", `
printf '%s\n' "$*" >> "$HWT_FAKE_LOG"
if [ "$1 $2" = "worktree list" ]; then
  if [ -f "$HWT_FAKE_STATE" ]; then
    IFS= read -r path < "$HWT_FAKE_STATE"
    branch=$(git -C "$path" branch --show-current)
    printf '{"result":{"source":{"source_checkout_path":"%s"},"worktrees":[{"branch":"%s","path":"%s","is_linked_worktree":true,"open_workspace_id":"ws1"}]}}\n' "$HWT_FAKE_SOURCE" "$branch" "$path"
  else
    printf '{"result":{"source":{"source_checkout_path":"%s"},"worktrees":[]}}\n' "$HWT_FAKE_SOURCE"
  fi
	  exit 0
fi
if [ "$1 $2" = "worktree create" ]; then
  shift 2; branch=; base=; path=
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --branch) branch=$2; shift 2;;
      --base) base=$2; shift 2;;
      --path) path=$2; shift 2;;
      *) shift;;
    esac
  done
  [ -n "$path" ] || path="$HWT_FAKE_SOURCE-wt-${branch##*/}"
  if git -C "$HWT_FAKE_SOURCE" show-ref --verify --quiet "refs/heads/$branch"; then
    git -C "$HWT_FAKE_SOURCE" worktree add "$path" "$branch" >/dev/null
  else
    git -C "$HWT_FAKE_SOURCE" worktree add -b "$branch" "$path" "$base" >/dev/null
  fi
  printf '%s\n' "$path" > "$HWT_FAKE_STATE"
  printf '{"result":{"workspace":{"workspace_id":"ws1"},"root_pane":{"pane_id":"pane1"},"worktree":{"path":"%s"}}}\n' "$path"
  exit 0
fi
if [ "$1 $2" = "worktree remove" ]; then
  if [ -f "$HWT_FAKE_STATE" ]; then IFS= read -r path < "$HWT_FAKE_STATE"; git -C "$HWT_FAKE_SOURCE" worktree remove --force "$path" >/dev/null; fi
  printf '{"result":{"removed":true}}\n'; exit 0
fi
if [ "$1 $2" = "workspace get" ]; then
  IFS= read -r path < "$HWT_FAKE_STATE"
  printf '{"result":{"workspace":{"workspace_id":"ws1","label":"fixture","worktree":{"checkout_path":"%s","is_linked_worktree":true}}}}\n' "$path"
  exit 0
fi
if [ "$1 $2" = "pane list" ]; then
  printf '{"result":{"panes":[{"pane_id":"pane1","workspace_id":"ws1"}]}}\n'; exit 0
fi
if [ "$1 $2" = "pane process-info" ]; then
  printf '{"result":{"process_info":{"foreground_processes":[]}}}\n'; exit 0
fi
if [ "$1 $2" = "pane run" ] || [ "$1" = "plugin" ] || [ "$1 $2" = "workspace focus" ] || [ "$1 $2" = "workspace close" ]; then
  printf 'ok\n'; exit 0
fi
printf 'unexpected fake herdr command: %s\n' "$*" >&2
exit 64`)
}

func mustWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func decode(t *testing.T, value string) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		t.Fatalf("decode JSON %q: %v", value, err)
	}
	return result
}

func requireContains(t *testing.T, value, fragment string) {
	t.Helper()
	if !strings.Contains(value, fragment) {
		t.Fatalf("%q does not contain %q", value, fragment)
	}
}
