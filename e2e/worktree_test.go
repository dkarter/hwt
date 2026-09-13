package e2e

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestWT001_WT004_WT005_WT007_CLI003_CreateExplicitCopiesHooksAndJSON(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	herdr := s.fakeHerdr(repo)
	mustWrite(t, filepath.Join(repo, ".env.local"), "SOURCE=primary\n", 0o600)
	worktrees := filepath.Join(s.root, "worktrees")
	config := "worktree_dir: " + worktrees + "\nfiles:\n  copy: [.env.local]\nports:\n  start: 32100\n  end: 32110\n  services: [web]\nenvironment:\n  variables:\n    APP_URL: http://127.0.0.1:${HWT_PORT_WEB}\npost_create:\n  - 'printf hook-ok > hook.txt'\n"
	mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), config, 0o600)

	out := s.run(repo, "--herdr-bin", herdr, "create", "--branch", "feature/explicit", "--label", "E2E", "--json")
	result := decode(t, out)
	path := result["path"].(string)
	if result["branch"] != "feature/explicit" || result["workspace_id"] != "ws1" {
		t.Fatalf("unexpected create result: %#v", result)
	}
	for _, field := range []string{"pane_id", "base", "config", "environment"} {
		if result[field] == nil {
			t.Fatalf("structured create result lacks %s: %#v", field, result)
		}
	}
	if mustRead(t, filepath.Join(path, ".env.local")) != "SOURCE=primary\n" || mustRead(t, filepath.Join(path, "hook.txt")) != "hook-ok" {
		t.Fatal("copy or post-create hook did not run")
	}
	if !strings.Contains(mustRead(t, filepath.Join(path, ".env.worktree")), "APP_URL=\"http://127.0.0.1:") {
		t.Fatal("generated environment did not reach hook worktree")
	}
	if got := s.git(path, "config", "--get", "branch.feature/explicit.herdr-base"); got != "main" {
		t.Fatalf("recorded base = %q", got)
	}
	if strings.Contains(mustRead(t, filepath.Join(s.root, "herdr.log")), "workspace focus") {
		t.Fatal("create focused the workspace without --focus")
	}
}

func TestREV001_URL006_TicketCreatePersistsMappedMetadata(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	herdr := s.fakeHerdr(repo)
	lnrLog := filepath.Join(s.root, "lnr.args")
	s.env = append(s.env, "LNR_LOG="+lnrLog)
	s.tool("lnr", `printf '%s\n' "$@" > "$LNR_LOG"
printf '%s\n' '{"issueId":"RMS-96","branchName":"rms-96-just-a-test","title":"just a test","url":"https://linear.app/srms/issue/RMS-96/just-a-test"}'`)
	mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), "ticket_commands:\n  create:\n    command: [lnr, quick, '{input}', --json]\n    output:\n      branch: branchName\n      metadata:\n        identifier: issueId\n        title: title\n        url: url\nworktree_dir: "+filepath.Join(s.root, "worktrees")+"\nurls:\n  ticket: https://tickets.invalid/{ticket.identifier}/{branch}/{region}\nmetadata:\n  values:\n    region: west\n", 0o600)
	s.git(repo, "add", ".herdr-worktree.yaml")
	s.git(repo, "commit", "-m", "add hwt config")

	result := decode(t, s.run(repo, "--herdr-bin", herdr, "create", "--ticket=create", "just a test", "--json"))
	path := result["path"].(string)
	if result["branch"] != "rms-96-just-a-test" {
		t.Fatalf("ticket branch = %#v", result["branch"])
	}
	if got := mustRead(t, lnrLog); got != "quick\njust a test\n--json\n" {
		t.Fatalf("ticket argv = %q", got)
	}
	if got := strings.TrimSpace(s.run(path, "url", "ticket")); got != "https://tickets.invalid/RMS-96/rms-96-just-a-test/west" {
		t.Fatalf("ticket URL = %q", got)
	}
	metadata := mustRead(t, filepath.Join(s.git(path, "rev-parse", "--git-dir"), "hwt-ticket-metadata-v1.json"))
	for _, value := range []string{"RMS-96", "just a test", "https://linear.app/srms/issue/RMS-96/just-a-test"} {
		if !strings.Contains(metadata, value) {
			t.Fatalf("persisted ticket metadata lacks %q: %q", value, metadata)
		}
	}
}

func TestREV002_InvalidTicketResponseStopsBeforeCreate(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	herdr := s.fakeHerdr(repo)
	s.tool("lnr", `printf '%s\n' '{"issueId":"ABC-123"}'`)
	mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), "ticket_commands:\n  default:\n    command: [lnr, quick, '{input}', --json]\n", 0o600)

	_, stderr, err := s.command(repo, "--herdr-bin", herdr, "create", "--ticket", "invalid ticket")
	if err == nil || !strings.Contains(stderr, "branchName") {
		t.Fatalf("invalid ticket response = %q, %v", stderr, err)
	}
	if strings.Contains(mustRead(t, filepath.Join(s.root, "herdr.log")), "worktree create") {
		t.Fatal("invalid ticket response invoked Herdr create")
	}
}

func TestREV008_REV009_LiteralBranchCreationWithoutTicket(t *testing.T) {
	for _, test := range []struct {
		name   string
		config string
		args   []string
	}{
		{name: "no ticket commands", args: []string{"create", "investigation-cache", "--json"}},
		{name: "configured command remains opt in", config: "ticket_commands:\n  default:\n    command: [tickets, create, '{input}', --json]\n", args: []string{"create", "investigation-cache", "--json"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := newSandbox(t)
			repo := s.repo()
			herdr := s.fakeHerdr(repo)
			s.tool("tickets", "printf 'ticket command must not run\\n' >&2; exit 99")
			if test.config != "" {
				mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), test.config, 0o600)
			}

			args := append([]string{"--herdr-bin", herdr}, test.args...)
			result := decode(t, s.run(repo, args...))
			if result["branch"] != "investigation-cache" {
				t.Fatalf("literal branch = %#v", result)
			}
		})
	}
}

func TestREV010_TicketCommandCanSelectExistingTicketWithoutQuery(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	herdr := s.fakeHerdr(repo)
	lnrLog := filepath.Join(s.root, "lnr.args")
	s.env = append(s.env, "LNR_LOG="+lnrLog)
	s.tool("lnr", `printf '%s\n' "$@" > "$LNR_LOG"
printf '%s\n' '{"branchName":"rms-95-existing-ticket"}'`)
	mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), "ticket_commands:\n  default:\n    command: [lnr, issue, search, --json, '{input}']\n", 0o600)

	result := decode(t, s.run(repo, "--herdr-bin", herdr, "create", "--ticket", "--json"))
	if result["branch"] != "rms-95-existing-ticket" {
		t.Fatalf("selected ticket branch = %#v", result)
	}
	if got := mustRead(t, lnrLog); got != "issue\nsearch\n--json\n" {
		t.Fatalf("ticket search argv = %q", got)
	}
}

func TestWT004_WT006_CreateAtExplicitPathRollsBackAfterHookFailure(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	herdr := s.fakeHerdr(repo)
	path := filepath.Join(s.root, "worktrees", "broken")
	mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), "post_create:\n  - 'exit 17'\n", 0o600)

	_, stderr, err := s.command(repo, "--herdr-bin", herdr, "create", "--branch", "feature/broken", "--path", path, "--json")
	if err == nil || !strings.Contains(stderr, "post_create command") {
		t.Fatalf("hook failure = %q, %v", stderr, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("rollback left worktree at %s: %v", path, err)
	}
	requireContains(t, mustRead(t, filepath.Join(s.root, "herdr.log")), "worktree remove")
}

func TestWT002_WT003_CreateRejectsInvalidSelectionAndDetachedBase(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	herdr := s.fakeHerdr(repo)

	for _, args := range [][]string{{"create"}, {"create", "description", "--branch", "feature/both"}} {
		_, stderr, err := s.command(repo, args...)
		if err == nil || !strings.Contains(stderr, "provide exactly one branch name or --branch") {
			t.Fatalf("create %v = %q, %v", args, stderr, err)
		}
	}
	s.git(repo, "checkout", "--detach")
	_, stderr, err := s.command(repo, "--herdr-bin", herdr, "create", "--branch", "feature/detached")
	if err == nil || !strings.Contains(stderr, "--base is required from a detached HEAD") {
		t.Fatalf("detached create = %q, %v", stderr, err)
	}
	if data, readErr := os.ReadFile(filepath.Join(s.root, "herdr.log")); readErr == nil && strings.Contains(string(data), "worktree create") {
		t.Fatalf("rejected create invoked Herdr: %s", data)
	}
}

func TestWT008_WT009_CopyIsIdempotentAndUsesPluginContext(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	linked := s.linked(repo, "feature/copy")
	mustWrite(t, filepath.Join(repo, "private.env"), "first\n", 0o600)
	mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), "files:\n  copy: [private.env]\n", 0o600)
	s.env = append(s.env, `HWT_HERDR_PLUGIN_CONTEXT_JSON={"workspace_cwd":"`+linked+`"}`)

	type commandResult struct {
		stdout string
		stderr string
		err    error
	}
	results := make(chan commandResult, 2)
	for range 2 {
		go func() {
			stdout, stderr, err := s.command(s.root, "copy", "--json")
			results <- commandResult{stdout: stdout, stderr: stderr, err: err}
		}()
	}
	prepared := 0
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent copy: %s%s: %v", result.stdout, result.stderr, result.err)
		}
		if decoded := decode(t, result.stdout); decoded["already_prepared"] == false {
			prepared++
		}
	}
	if prepared != 1 || mustRead(t, filepath.Join(linked, "private.env")) != "first\n" {
		t.Fatalf("concurrent copies prepared %d times", prepared)
	}
	if _, err := os.Stat(filepath.Join(linked, ".env.worktree")); err != nil {
		t.Fatalf("copy did not generate the worktree environment: %v", err)
	}
	mustWrite(t, filepath.Join(repo, "private.env"), "second\n", 0o600)
	second := decode(t, s.run(s.root, "copy", "--json"))
	if second["already_prepared"] != true || mustRead(t, filepath.Join(linked, "private.env")) != "first\n" {
		t.Fatalf("second copy was not idempotent: %#v", second)
	}
}

func TestENV001_ENV002_ENV003_ENV004_EnvironmentStableRefreshAndRun(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	linked := s.linked(repo, "feature/env")
	other := s.linked(repo, "feature/env-other")
	mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), "ports:\n  start: 32200\n  end: 32220\n  services: [web, assets]\nenvironment:\n  variables:\n    WEB_ORIGIN: http://localhost:${HWT_PORT_WEB}\n", 0o600)

	type environmentResult struct {
		stdout string
		stderr string
		err    error
	}
	results := make(chan environmentResult, 2)
	for _, path := range []string{linked, other} {
		go func() {
			stdout, stderr, err := s.command(path, "env", "--json")
			results <- environmentResult{stdout: stdout, stderr: stderr, err: err}
		}()
	}
	outputs := make([]map[string]any, 0, 2)
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent environment: %s%s: %v", result.stdout, result.stderr, result.err)
		}
		outputs = append(outputs, decode(t, result.stdout))
	}
	first, otherResult := outputs[0], outputs[1]
	if first["variables"].(map[string]any)["HWT_WORKTREE_BRANCH"] != "feature/env" {
		first, otherResult = otherResult, first
	}
	canonicalLinked, err := filepath.EvalSymlinks(linked)
	if err != nil {
		t.Fatal(err)
	}
	second := decode(t, s.run(linked, "env", "--json"))
	a := first["variables"].(map[string]any)
	b := second["variables"].(map[string]any)
	if a["HWT_PORT_WEB"] != b["HWT_PORT_WEB"] || a["HWT_PORT_ASSETS"] != b["HWT_PORT_ASSETS"] {
		t.Fatalf("ports changed without refresh: %#v %#v", a, b)
	}
	c := otherResult["variables"].(map[string]any)
	if a["HWT_PORT_WEB"] == c["HWT_PORT_WEB"] || a["HWT_PORT_ASSETS"] == c["HWT_PORT_ASSETS"] {
		t.Fatalf("worktrees received conflicting ports: %#v %#v", a, c)
	}
	refreshed := decode(t, s.run(linked, "env", "--refresh", "--json"))["variables"].(map[string]any)
	if refreshed["HWT_PORT_WEB"] == c["HWT_PORT_WEB"] || refreshed["HWT_PORT_ASSETS"] == c["HWT_PORT_ASSETS"] {
		t.Fatalf("refresh conflicted with another worktree: other=%#v refreshed=%#v", c, refreshed)
	}
	if mustRead(t, filepath.Join(linked, ".env.worktree")) == "" {
		t.Fatal("refresh did not republish the environment")
	}
	info, err := os.Stat(filepath.Join(linked, ".env.worktree"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("environment mode = %v, %v", info, err)
	}
	expectedEnvironment, err := filepath.EvalSymlinks(filepath.Join(linked, ".env.worktree"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(s.run(linked, "env")); got != expectedEnvironment {
		t.Fatalf("plain environment path = %q", got)
	}
	s.git(linked, "check-ignore", ".env.worktree")
	capture := filepath.Join(s.root, "run.env")
	s.env = append(s.env, "CAPTURE="+capture)
	s.tool("capture-env", `printf '%s|%s|%s' "$PWD" "$HWT_WORKTREE_BRANCH" "$WEB_ORIGIN" > "$CAPTURE"`)
	s.run(linked, "env", "--", "capture-env")
	parts := strings.Split(mustRead(t, capture), "|")
	if len(parts) != 3 || parts[0] != canonicalLinked || parts[1] != "feature/env" || !strings.HasPrefix(parts[2], "http://localhost:") {
		t.Fatalf("run environment = %q", strings.Join(parts, "|"))
	}
	if port, err := strconv.Atoi(refreshed["HWT_PORT_WEB"].(string)); err != nil || port < 32200 || port > 32220 {
		t.Fatalf("port outside configured range: %#v", refreshed)
	}
}

func TestENV005_EnvironmentRejectsUnsafeInputs(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	linked := s.linked(repo, "feature/unsafe-env")
	mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), "environment:\n  variables:\n    BAD: ${UNKNOWN_GENERATED}\n", 0o600)

	_, stderr, err := s.command(linked, "env")
	if err == nil || !strings.Contains(stderr, "UNKNOWN_GENERATED") {
		t.Fatalf("unsafe environment = %q, %v", stderr, err)
	}
}

func TestWT011_WT012_SafeAndForcedRemoval(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	herdr := s.fakeHerdr(repo)
	state := filepath.Join(s.root, "herdr-worktree")
	clean := s.linked(repo, "feature/remove-clean")
	mustWrite(t, state, clean+"\n", 0o600)
	result := decode(t, s.run(repo, "--herdr-bin", herdr, "rm", "--workspace", "ws1", "--json"))
	if result["path"] != clean {
		t.Fatalf("safe removal result = %#v", result)
	}

	linked := s.linked(repo, "feature/remove-forced")
	mustWrite(t, state, linked+"\n", 0o600)

	mustWrite(t, filepath.Join(linked, "dirty.txt"), "dirty\n", 0o600)
	_, stderr, err := s.command(repo, "--herdr-bin", herdr, "remove", "--workspace", "ws1", "--json")
	if err == nil || !strings.Contains(stderr, "uncommitted or untracked") {
		t.Fatalf("unsafe remove = %q, %v", stderr, err)
	}
	result = decode(t, s.run(repo, "--herdr-bin", herdr, "remove", "--workspace", "ws1", "--force", "--json"))
	if result["workspace_id"] != "ws1" || result["path"] != linked {
		t.Fatalf("forced removal result = %#v", result)
	}
	if _, err := os.Stat(linked); !os.IsNotExist(err) {
		t.Fatalf("removed checkout remains: %v", err)
	}
}
