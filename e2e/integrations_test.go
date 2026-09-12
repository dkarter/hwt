package e2e

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestURL001_URL003_URL004_URL007_NamedURLsPreviewMetadataAndPRTemplate(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	s.git(repo, "remote", "add", "origin", filepath.Join(s.root, "offline-origin.git"))
	metaLog := filepath.Join(s.root, "meta.args")
	s.env = append(s.env, "META_LOG="+metaLog)
	s.tool("metadata", `printf '%s\n' "$@" > "$META_LOG"
printf '%s\n' '{"slug":"blue team"}'`)
	s.tool("gh", `printf '%s\n' '{"number":42,"url":"https://github.com/acme/app/pull/42"}'`)
	mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), `urls:
  preview: https://{sanitized_branch}.preview.invalid/{region}/{deploy.slug}
  pull: https://reviews.invalid/{repository}/{pr_number}
metadata:
  values:
    region: us west
  commands:
    deploy: [metadata, --branch, '{branch}', --json]
`, 0o600)

	preview := decode(t, s.run(repo, "url", "preview", "Feature/URL Test", "--json"))
	if preview["url"] != "https://feature-url-test.preview.invalid/us%20west/blue%20team" {
		t.Fatalf("preview URL = %#v", preview)
	}
	if got := mustRead(t, metaLog); got != "--branch\nFeature/URL Test\n--json\n" {
		t.Fatalf("metadata argv = %q", got)
	}
	pull := strings.TrimSpace(s.run(repo, "url", "pull", "feature/pr", "--repo", "acme/app"))
	if pull != "https://reviews.invalid/repo/42" {
		t.Fatalf("PR template URL = %q", pull)
	}
}

func TestURL008_URL009_ListAndCompleteConfiguredURLs(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	s.git(repo, "remote", "add", "origin", filepath.Join(s.root, "offline-origin.git"))
	s.tool("gh", `printf '%s\n' '{"url":"https://github.com/acme/app/pull/12"}'`)
	mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), "urls:\n  preview: https://preview.invalid/{branch}\n  ticket: https://tickets.invalid/{branch}\n", 0o600)

	listed := s.run(repo, "url", "--json")
	for _, expected := range []string{`"name": "pr"`, `"url": "https://github.com/acme/app/pull/12"`, `"name": "preview"`, `"name": "ticket"`} {
		requireContains(t, listed, expected)
	}
	completion := s.run(repo, "__complete", "url", "")
	for _, name := range []string{"pr", "preview", "ticket"} {
		requireContains(t, completion, name+"\n")
	}
	help := s.run(repo, "--help")
	if strings.Contains(help, "\n  pr ") || strings.Contains(help, "\n  preview ") {
		t.Fatalf("legacy root URL commands remain in help:\n%s", help)
	}
}

func TestREV003_PullRequestJSONAndFailure(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	s.git(repo, "remote", "add", "origin", filepath.Join(s.root, "origin.git"))
	s.tool("gh", `if [ "${GH_FAIL:-}" = 1 ]; then printf 'not found\n' >&2; exit 1; fi
printf '%s\n' '{"url":"https://github.com/acme/app/pull/9"}'`)
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	openLog := filepath.Join(s.root, "open.log")
	s.env = append(s.env, "OPEN_LOG="+openLog)
	s.tool(opener, `printf '%s\n' "$1" > "$OPEN_LOG"`)
	if got := decode(t, s.run(repo, "url", "pr", "--json"))["url"]; got != "https://github.com/acme/app/pull/9" {
		t.Fatalf("PR URL = %#v", got)
	}
	requireContains(t, s.run(repo, "url", "pr", "--open"), "Opened https://github.com/acme/app/pull/9")
	if strings.TrimSpace(mustRead(t, openLog)) != "https://github.com/acme/app/pull/9" {
		t.Fatalf("opened URL = %q", mustRead(t, openLog))
	}
	s.env = append(s.env, "GH_FAIL=1")
	_, stderr, err := s.command(repo, "url", "pr", "--json")
	if err == nil || !strings.Contains(stderr, "query GitHub pull requests") || !strings.Contains(stderr, "not found") {
		t.Fatalf("PR failure = %q, %v", stderr, err)
	}
}

func TestURL001_URL002_URL003_URL005_ExplicitOpeningAndUnavailableValues(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	openLog := filepath.Join(s.root, "open.log")
	s.env = append(s.env, "OPEN_LOG="+openLog)
	s.tool(opener, `printf '%s\n' "$1" > "$OPEN_LOG"`)
	mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), `urls:
  web: https://example.invalid/{sanitized_branch}
  preview: https://preview.invalid/{sanitized_branch}
  database: postgres://database.invalid/app
  local: https://example.invalid/{worktree}
`, 0o600)

	if got := strings.TrimSpace(s.run(repo, "url", "web", "Feature/Open")); got != "https://example.invalid/feature-open" {
		t.Fatalf("plain URL = %q", got)
	}
	if got := decode(t, s.run(repo, "url", "web", "Feature/Open", "--json"))["name"]; got != "web" {
		t.Fatalf("JSON URL = %#v", got)
	}
	requireContains(t, s.run(repo, "url", "web", "Feature/Open", "--open"), "Opened")
	if strings.TrimSpace(mustRead(t, openLog)) != "https://example.invalid/feature-open" {
		t.Fatalf("opened URL = %q", mustRead(t, openLog))
	}
	if got := decode(t, s.run(repo, "url", "preview", "Feature/Open", "--json"))["url"]; got != "https://preview.invalid/feature-open" {
		t.Fatalf("preview JSON = %#v", got)
	}
	requireContains(t, s.run(repo, "url", "preview", "Feature/Open", "--open"), "Opened https://preview.invalid/feature-open")
	for _, args := range [][]string{{"url", "database", "--open"}, {"url", "local", "other-branch"}} {
		_, stderr, err := s.command(repo, args...)
		if err == nil {
			t.Fatalf("unsafe URL %v succeeded", args)
		}
		if !strings.Contains(stderr, "refusing to open non-browser") && !strings.Contains(stderr, "unavailable") {
			t.Fatalf("unsafe URL %v = %q", args, stderr)
		}
	}
}

func TestENV006_ENV007_ENV008_SetupStatusRefreshAndTeardown(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("local DNS is supported only on darwin and linux")
	}
	s := newSandbox(t)
	repo := s.repo()
	linked := s.linked(repo, "feature/dns")
	s.tool("caddy", "exit 0")
	s.tool("dnsmasq", "exit 0")
	mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), "ports:\n  start: 32300\n  end: 32310\n  services: [web]\nlocal_dns:\n  enabled: true\n  domain: e2e.test\n", 0o600)

	setup := decode(t, s.run(repo, "dns", "setup", "--json"))
	paths := setup["paths"].(map[string]any)
	if _, err := os.Stat(paths["dnsmasq"].(string)); err != nil {
		t.Fatalf("dnsmasq snippet: %v", err)
	}
	refresh := decode(t, s.run(linked, "dns", "refresh", "--json"))
	variables := refresh["variables"].(map[string]any)
	if !strings.HasSuffix(variables["HWT_WORKTREE_HOSTNAME"].(string), ".e2e.test") {
		t.Fatalf("hostname = %#v", variables)
	}
	status := decode(t, s.run(repo, "dns", "status", "--json"))
	if len(status["entries"].([]any)) != 1 {
		t.Fatalf("DNS status = %#v", status)
	}
	_, stderr, err := s.command(repo, "dns", "teardown", "--json")
	if err == nil || !strings.Contains(stderr, "registered worktree") {
		t.Fatalf("non-force teardown = %q, %v", stderr, err)
	}
	removed := decode(t, s.run(repo, "dns", "teardown", "--force", "--json"))
	if removed["removed"] != true {
		t.Fatalf("teardown = %#v", removed)
	}
}

func TestHDR001_HDR002_WT010_PluginForwardingAndList(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	herdr := s.fakeHerdr(repo)
	if got := s.run(repo, "--herdr-bin", herdr, "list"); !strings.Contains(got, "source_checkout_path") {
		t.Fatalf("list output = %q", got)
	}
	s.run(repo, "--herdr-bin", herdr, "plugin", "install")
	s.run(repo, "--herdr-bin", herdr, "plugin", "update")
	s.run(repo, "--herdr-bin", herdr, "plugin", "uninstall")
	log := mustRead(t, filepath.Join(s.root, "herdr.log"))
	for _, expected := range []string{
		"plugin install dkarter/hwt/plugins/herdr --ref v1.2.3 --yes",
		"plugin uninstall hwt.worktrees",
	} {
		requireContains(t, log, expected)
	}
}

func TestHDR003_DevelopmentPluginUsesDefaultRevision(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	herdr := s.fakeHerdr(repo)
	stdout, stderr, err := s.commandBinary(devHWTBinary, repo, "--herdr-bin", herdr, "plugin", "install")
	if err != nil {
		t.Fatalf("development plugin install: %s%s: %v", stdout, stderr, err)
	}
	log := mustRead(t, filepath.Join(s.root, "herdr.log"))
	requireContains(t, log, "plugin install dkarter/hwt/plugins/herdr --yes")
	if strings.Contains(log, "--ref") {
		t.Fatalf("development plugin unexpectedly pinned a ref: %s", log)
	}
}

func TestREV005_REV007_ReviewLocalBranchCreatesWorkspaceAndLaunchesTool(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	s.git(repo, "branch", "feature/review")
	herdr := s.fakeHerdr(repo)
	s.tool("review-tool", "exit 0")
	mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), "review_command: [review-tool, --local]\nworktree_dir: "+filepath.Join(s.root, "reviews")+"\n", 0o600)

	result := decode(t, s.run(repo, "--herdr-bin", herdr, "review", "feature/review", "--json"))
	if result["workspace_id"] != "ws1" || result["reused"] != false {
		t.Fatalf("review result = %#v", result)
	}
	launch := result["review_command"].(map[string]any)
	if launch["status"] != "launched" {
		t.Fatalf("review launch = %#v", launch)
	}
	log := mustRead(t, filepath.Join(s.root, "herdr.log"))
	requireContains(t, log, "pane run pane1 'review-tool' '--local'")
	path := result["path"].(string)
	if got := s.git(path, "branch", "--show-current"); !strings.HasPrefix(got, "hwt/review/feature-review-") {
		t.Fatalf("review branch = %q", got)
	}
}

func TestREV004_ReviewForkPullRequestByExactCommit(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	remote := filepath.Join(s.root, "origin.git")
	s.git(repo, "clone", "--bare", repo, remote)
	head := s.git(repo, "rev-parse", "HEAD")
	s.git(remote, "update-ref", "refs/pull/7/head", head)
	s.git(repo, "remote", "add", "origin", remote)
	herdr := s.fakeHerdr(repo)
	s.tool("review-tool", "exit 0")
	s.tool("gh", `printf '%s\n' '{"number":7,"url":"https://github.com/acme/app/pull/7","title":"Fork change","headRefName":"feature/fork","headRefOid":"`+head+`","baseRefName":"main","isCrossRepository":true}'`)
	mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), "review_command: [review-tool]\nworktree_dir: "+filepath.Join(s.root, "reviews")+"\n", 0o600)

	result := decode(t, s.run(repo, "--herdr-bin", herdr, "review", "https://github.com/acme/app/pull/7", "--repo", "acme/app", "--remote", "origin", "--json"))
	identity := result["identity"].(map[string]any)
	if result["commit"] != head || identity["fork"] != true || identity["remote"] != "origin" {
		t.Fatalf("pull request review result = %#v", result)
	}
	if got := s.git(repo, "branch", "--show-current"); got != "main" {
		t.Fatalf("source checkout moved to %q", got)
	}
}

func TestREV006_ExactReviewWorkspaceIsReused(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	s.git(repo, "branch", "feature/reuse")
	herdr := s.fakeHerdr(repo)
	s.tool("review-tool", "exit 0")
	mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), "review_command: [review-tool]\nworktree_dir: "+filepath.Join(s.root, "reviews")+"\n", 0o600)

	first := decode(t, s.run(repo, "--herdr-bin", herdr, "review", "feature/reuse", "--json"))
	second := decode(t, s.run(repo, "--herdr-bin", herdr, "review", "feature/reuse", "--json"))
	if first["reused"] != false || second["reused"] != true {
		t.Fatalf("review reuse: first=%#v second=%#v", first, second)
	}
	if second["workspace_id"] != "ws1" || second["review_command"].(map[string]any)["status"] != "launched" {
		t.Fatalf("reused review result = %#v", second)
	}
}
