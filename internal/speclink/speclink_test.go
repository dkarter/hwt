package speclink

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestLoadReportsAllLinkFailures(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "specs/alpha/spec.md", `
#### Scenario: Valid but uncovered {#AAA-001}
#### Scenario: Duplicate {#AAA-001}
#### Scenario: Missing
#### Scenario: Malformed {#AAA-02}
#### Scenario: Extra malformed {#AAA-002} {#BAD}
#### Scenario: External <!-- spec-links:ignore -->
`)
	writeFixture(t, root, "e2e/links_test.go", `package e2e
import "testing"
func TestBBB001Unknown(t *testing.T) {}
func TestHasNoLink(t *testing.T) {}
`)

	_, err := Load(filepath.Join(root, "specs"), filepath.Join(root, "e2e"))
	var issues Issues
	if !errors.As(err, &issues) {
		t.Fatalf("expected Issues, got %v", err)
	}
	var codes []string
	for _, issue := range issues {
		codes = append(codes, issue.Code)
	}
	sort.Strings(codes)
	want := []string{"duplicate-scenario-id", "malformed-scenario-id", "malformed-scenario-id", "missing-scenario-id", "scenario-without-test", "scenario-without-test", "unknown-test-reference", "unlinked-e2e-test"}
	if !reflect.DeepEqual(codes, want) {
		t.Fatalf("issue codes = %v, want %v", codes, want)
	}
}

func TestLoadAcceptsTwoThroughEightLetterPrefixes(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "specs/prefixes/spec.md", `
#### Scenario: Short {#WT-001}
#### Scenario: Long {#ABCDEFGH-002}
`)
	writeFixture(t, root, "e2e/links_test.go", `package e2e
import "testing"
func TestWT001Short(t *testing.T) {}
func TestABCDEFGH002Long(t *testing.T) {}
`)

	index, err := Load(filepath.Join(root, "specs"), filepath.Join(root, "e2e"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := index.Tests[1].IDs, []string{"WT-001"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("short prefix IDs = %v, want %v", got, want)
	}
}

func TestSelectAndTestRegex(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "specs/worktrees/spec.md", `
#### Scenario: Create {#WTR-001}
#### Scenario: Remove {#WTR-002}
`)
	writeFixture(t, root, "specs/review/spec.md", `
#### Scenario: Open review {#REV-001}
`)
	writeFixture(t, root, "e2e/links_test.go", `package e2e
import "testing"
func TestWTR001Create(t *testing.T) {}
func TestWTR002REV001SharedFlow(t *testing.T) {}
`)

	index, err := Load(filepath.Join(root, "specs"), filepath.Join(root, "e2e"))
	if err != nil {
		t.Fatal(err)
	}
	tests, err := index.Select("", "worktrees", "")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := TestRegex(tests), `^(?:TestWTR001Create|TestWTR002REV001SharedFlow)$`; got != want {
		t.Fatalf("regex = %q, want %q", got, want)
	}
	tests, err = index.Select("", "", "review/spec.md")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := TestRegex(tests), `^(?:TestWTR002REV001SharedFlow)$`; got != want {
		t.Fatalf("regex = %q, want %q", got, want)
	}
	if _, err := index.Select("", "", "spec.md"); err == nil {
		t.Fatal("ambiguous spec basename unexpectedly selected multiple capabilities")
	}
}

func writeFixture(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
