package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMETA001_RepositoryAndInvalidFixtureValidation(t *testing.T) {
	command := exec.Command(specLinksBinary, "validate")
	command.Dir = repoRoot
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("repository validation failed: %s: %v", output, err)
	}
	requireContains(t, string(output), "validated 66 scenarios")

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "specs", "sample", "spec.md"), `
#### Scenario: Duplicate {#XY-001}
#### Scenario: Duplicate again {#XY-001}
#### Scenario: Missing
#### Scenario: Malformed {#lower-002}
#### Scenario: Uncovered {#LONGNAME-003}
`, 0o600)
	mustWrite(t, filepath.Join(root, "e2e", "sample_test.go"), `package e2e
import "testing"
func TestZZ999Unknown(t *testing.T) {}
func TestWithoutID(t *testing.T) {}
`, 0o600)
	command = exec.Command(specLinksBinary, "validate", "--spec-root", filepath.Join(root, "specs"), "--e2e-root", filepath.Join(root, "e2e"))
	output, err = command.CombinedOutput()
	if err == nil {
		t.Fatalf("invalid fixture passed validation: %s", output)
	}
	for _, code := range []string{"duplicate-scenario-id", "missing-scenario-id", "malformed-scenario-id", "unknown-test-reference", "unlinked-e2e-test", "scenario-without-test"} {
		requireContains(t, string(output), "["+code+"]")
	}
}

func TestMETA002_FocusedRunnerListsScenarioCapabilityAndSpec(t *testing.T) {
	for _, args := range [][]string{
		{"run", "--scenario", "META-002", "--list"},
		{"run", "--capability", "spec-test-linkage", "--list"},
		{"run", "--spec", "spec-test-linkage/spec.md", "--list"},
	} {
		command := exec.Command(specLinksBinary, args...)
		command.Dir = repoRoot
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("spec-links %s: %s: %v", strings.Join(args, " "), output, err)
		}
		requireContains(t, string(output), "TestMETA002_FocusedRunnerListsScenarioCapabilityAndSpec")
	}
}

func TestDIST003_DIST004_DIST005_DevelopmentReleaseValidation(t *testing.T) {
	command := exec.Command(filepath.Join(repoRoot, "scripts", "test-development-release.sh"))
	command.Dir = repoRoot
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("development release validation: %s: %v", output, err)
	}
	requireContains(t, string(output), "development release checks passed")
}

func TestHDR004_HDR005_HDR006_HDR007_HDR008_LiveHerdrPlugin(t *testing.T) {
	script := mustRead(t, filepath.Join(repoRoot, "scripts", "test-herdr-plugin-e2e.sh"))
	for _, forbidden := range []string{"herdr plugin install", "herdr plugin update", "herdr plugin link", "herdr plugin unlink", "herdr plugin uninstall"} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("live tier contains unsafe active-session command %q", forbidden)
		}
	}
	manifest := mustRead(t, filepath.Join(repoRoot, "plugins", "herdr", "herdr-plugin.toml"))
	for _, expected := range []string{`on = "worktree.created"`, `command = ["./run-hwt", "copy"]`, `id = "new"`, `id = "remove"`} {
		requireContains(t, manifest, expected)
	}
	if os.Getenv("HWT_E2E_LIVE") != "1" {
		t.Skip("set HWT_E2E_LIVE=1 to run the interactive Herdr E2E test")
	}
	command := exec.Command(filepath.Join(repoRoot, "scripts", "test-herdr-plugin-e2e.sh"))
	command.Dir = repoRoot
	command.Env = os.Environ()
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("live Herdr plugin E2E: %s: %v", output, err)
	}
	requireContains(t, string(output), "Herdr plugin end-to-end test passed")
}
