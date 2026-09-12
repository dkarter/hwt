// Package speclink links OpenSpec scenarios to end-to-end Go tests.
package speclink

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	scenarioHeading = regexp.MustCompile(`^\s*#{1,6}\s+Scenario:\s*(.*?)\s*$`)
	scenarioID      = regexp.MustCompile(`\{#([A-Z]{2,8}-[0-9]{3})\}`)
	testID          = regexp.MustCompile(`[A-Z]{2,8}[0-9]{3}`)
)

const ignoreMarker = "<!-- spec-links:ignore -->"

// Scenario is an active OpenSpec scenario.
type Scenario struct {
	ID         string
	Name       string
	Capability string
	Path       string
	Line       int
}

// Test is a top-level Go end-to-end test and its linked scenario IDs.
type Test struct {
	Name string
	Path string
	Line int
	IDs  []string
}

// Issue is one validation failure.
type Issue struct {
	Code    string
	Path    string
	Line    int
	Message string
}

func (i Issue) Error() string {
	location := i.Path
	if i.Line > 0 {
		location = fmt.Sprintf("%s:%d", location, i.Line)
	}
	return fmt.Sprintf("%s: %s [%s]", location, i.Message, i.Code)
}

// Index contains all parsed active scenarios and top-level E2E tests.
type Index struct {
	Scenarios []Scenario
	Tests     []Test
}

// Load parses the specs and E2E tests below the supplied directories.
func Load(specRoot, e2eRoot string) (Index, error) {
	scenarios, specIssues, err := loadScenarios(specRoot)
	if err != nil {
		return Index{}, err
	}
	tests, err := loadTests(e2eRoot)
	if err != nil {
		return Index{}, err
	}
	index := Index{Scenarios: scenarios, Tests: tests}
	indexIssues := index.linkIssues()
	if len(specIssues)+len(indexIssues) > 0 {
		issues := append(specIssues, indexIssues...)
		sortIssues(issues)
		return index, Issues(issues)
	}
	return index, nil
}

// Issues is a collection of validation failures.
type Issues []Issue

func (i Issues) Error() string {
	lines := make([]string, len(i))
	for n := range i {
		lines[n] = i[n].Error()
	}
	return strings.Join(lines, "\n")
}

func loadScenarios(root string) ([]Scenario, []Issue, error) {
	var scenarios []Scenario
	var issues []Issue
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		capability := strings.Split(filepath.ToSlash(rel), "/")[0]
		scanner := bufio.NewScanner(file)
		for line := 1; scanner.Scan(); line++ {
			heading := scenarioHeading.FindStringSubmatch(scanner.Text())
			if heading == nil || strings.Contains(heading[1], ignoreMarker) {
				continue
			}
			ids := scenarioID.FindAllStringSubmatch(heading[1], -1)
			if len(ids) == 0 {
				code, message := "missing-scenario-id", "scenario is missing an ID such as {#ABC-001}"
				if strings.Contains(heading[1], "{#") {
					code, message = "malformed-scenario-id", "scenario ID must use a 2-8 letter uppercase prefix and three digits, such as {#ABC-001}"
				}
				issues = append(issues, Issue{Code: code, Path: filepath.ToSlash(path), Line: line, Message: message})
				continue
			}
			if len(ids) > 1 {
				issues = append(issues, Issue{Code: "malformed-scenario-id", Path: filepath.ToSlash(path), Line: line, Message: "scenario must have exactly one ID"})
				continue
			}
			withoutID := strings.Replace(heading[1], ids[0][0], "", 1)
			if strings.Contains(withoutID, "{#") {
				issues = append(issues, Issue{Code: "malformed-scenario-id", Path: filepath.ToSlash(path), Line: line, Message: "scenario contains an additional malformed ID"})
				continue
			}
			name := strings.TrimSpace(withoutID)
			scenarios = append(scenarios, Scenario{ID: ids[0][1], Name: name, Capability: capability, Path: filepath.ToSlash(path), Line: line})
		}
		scanErr := scanner.Err()
		if closeErr := file.Close(); scanErr == nil {
			scanErr = closeErr
		}
		return scanErr
	})
	return scenarios, issues, err
}

func loadTests(root string) ([]Test, error) {
	var tests []Test
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		fileset := token.NewFileSet()
		file, err := parser.ParseFile(fileset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || !isGoTest(function) {
				continue
			}
			position := fileset.Position(function.Pos())
			encoded := testID.FindAllString(function.Name.Name, -1)
			ids := make([]string, len(encoded))
			for i, id := range encoded {
				ids[i] = id[:len(id)-3] + "-" + id[len(id)-3:]
			}
			tests = append(tests, Test{Name: function.Name.Name, Path: filepath.ToSlash(path), Line: position.Line, IDs: ids})
		}
		return nil
	})
	sort.Slice(tests, func(i, j int) bool { return tests[i].Name < tests[j].Name })
	return tests, err
}

func isGoTest(function *ast.FuncDecl) bool {
	name := function.Name.Name
	if function.Recv != nil || name == "TestMain" || !strings.HasPrefix(name, "Test") || len(name) == len("Test") {
		return false
	}
	if function.Type.Results != nil && len(function.Type.Results.List) != 0 {
		return false
	}
	if function.Type.Params == nil || len(function.Type.Params.List) != 1 {
		return false
	}
	pointer, ok := function.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := pointer.X.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == "T"
}

func (index Index) linkIssues() []Issue {
	known := make(map[string]Scenario, len(index.Scenarios))
	var issues []Issue
	for _, scenario := range index.Scenarios {
		if first, exists := known[scenario.ID]; exists {
			issues = append(issues, Issue{Code: "duplicate-scenario-id", Path: scenario.Path, Line: scenario.Line, Message: fmt.Sprintf("scenario ID %s already appears at %s:%d", scenario.ID, first.Path, first.Line)})
		} else {
			known[scenario.ID] = scenario
		}
	}
	linked := make(map[string]bool)
	for _, test := range index.Tests {
		if len(test.IDs) == 0 {
			issues = append(issues, Issue{Code: "unlinked-e2e-test", Path: test.Path, Line: test.Line, Message: fmt.Sprintf("E2E test %s has no scenario ID", test.Name)})
		}
		for _, id := range test.IDs {
			if _, exists := known[id]; !exists {
				issues = append(issues, Issue{Code: "unknown-test-reference", Path: test.Path, Line: test.Line, Message: fmt.Sprintf("E2E test %s references unknown scenario %s", test.Name, id)})
			} else {
				linked[id] = true
			}
		}
	}
	for _, scenario := range index.Scenarios {
		if !linked[scenario.ID] {
			issues = append(issues, Issue{Code: "scenario-without-test", Path: scenario.Path, Line: scenario.Line, Message: fmt.Sprintf("scenario %s has no E2E test", scenario.ID)})
		}
	}
	return issues
}

func sortIssues(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Path != issues[j].Path {
			return issues[i].Path < issues[j].Path
		}
		if issues[i].Line != issues[j].Line {
			return issues[i].Line < issues[j].Line
		}
		return issues[i].Code < issues[j].Code
	})
}

// Select returns linked tests matching exactly one optional selector. With no
// selector it returns every linked test.
func (index Index) Select(scenarioID, capability, specPath string) ([]Test, error) {
	selectors := 0
	for _, value := range []string{scenarioID, capability, specPath} {
		if value != "" {
			selectors++
		}
	}
	if selectors > 1 {
		return nil, fmt.Errorf("use only one of scenario, capability, or spec path")
	}
	wanted := make(map[string]bool)
	matchedSpecPaths := make(map[string]bool)
	for _, scenario := range index.Scenarios {
		match := selectors == 0 || scenario.ID == scenarioID || scenario.Capability == capability
		if specPath != "" {
			cleanWanted := filepath.ToSlash(filepath.Clean(specPath))
			cleanActual := filepath.ToSlash(filepath.Clean(scenario.Path))
			match = cleanActual == cleanWanted || strings.HasSuffix(cleanActual, "/"+cleanWanted)
			if match {
				matchedSpecPaths[cleanActual] = true
			}
		}
		if match {
			wanted[scenario.ID] = true
		}
	}
	if len(matchedSpecPaths) > 1 {
		return nil, fmt.Errorf("spec path %q is ambiguous; include its capability directory", specPath)
	}
	var selected []Test
	for _, test := range index.Tests {
		for _, id := range test.IDs {
			if wanted[id] {
				selected = append(selected, test)
				break
			}
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("selector matched no linked E2E tests")
	}
	return selected, nil
}

// TestRegex builds an exact go test -run expression for tests.
func TestRegex(tests []Test) string {
	names := make([]string, len(tests))
	for i, test := range tests {
		names[i] = regexp.QuoteMeta(test.Name)
	}
	return "^(?:" + strings.Join(names, "|") + ")$"
}
