package urltemplate

import (
	"reflect"
	"strings"
	"testing"
)

func TestExpandEscapesComponents(t *testing.T) {
	template := "https://{sanitized_branch}.preview.example/{repository}/{branch}?worktree={deployment.worktree}"
	values := map[string]string{
		"repository":          "my app",
		"branch":              "Feature/café + tea",
		"sanitized_branch":    SanitizeBranch("Feature/café + tea"),
		"deployment.worktree": "feature café",
	}
	result, err := Expand(template, values)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://feature-caf-tea.preview.example/my%20app/Feature%2Fcaf%C3%A9%20%2B%20tea?worktree=feature%20caf%C3%A9"
	if result != want {
		t.Fatalf("URL = %q, want %q", result, want)
	}
}

func TestSanitizeBranch(t *testing.T) {
	if result := SanitizeBranch("--Feature/API__V2...Ready--"); result != "feature-api-v2-ready" {
		t.Fatalf("sanitized branch = %q", result)
	}
	if result := SanitizeBranch("Feature/Kelvin-İstanbul"); result != "feature-elvin-stanbul" {
		t.Fatalf("non-ASCII sanitization = %q", result)
	}
	if result := SanitizeBranch(strings.Repeat("a", 80)); len(result) != 63 {
		t.Fatalf("sanitized branch length = %d", len(result))
	}
}

func TestTemplateErrors(t *testing.T) {
	tests := []struct {
		name     string
		template string
		values   map[string]string
		want     string
	}{
		{name: "invalid name", template: "https://example.com/{bad name}", want: "invalid placeholder"},
		{name: "unclosed", template: "https://example.com/{branch", want: "unclosed"},
		{name: "relative", template: "/{branch}", want: "absolute URL"},
		{name: "invalid escape", template: "https://example.com/%zz/{branch}", want: "valid URL"},
		{name: "unknown value", template: "https://example.com/{worktree}", values: map[string]string{"branch": "main"}, want: "unknown placeholder"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var err error
			if test.values == nil {
				err = ValidateTemplate(test.template)
			} else {
				_, err = Expand(test.template, test.values)
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestPlaceholdersPreservesFirstUseOrder(t *testing.T) {
	names, err := Placeholders("postgres://{database.user}@{database.host}/{repository}?user={database.user}")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"database.user", "database.host", "repository"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("placeholders = %#v, want %#v", names, want)
	}
}

func TestExpandDoesNotReadAmbientEnvironment(t *testing.T) {
	t.Setenv("HWT_TEST_SECRET", "expanded-secret")
	result, err := Expand("https://example.com/$HWT_TEST_SECRET/{branch}", map[string]string{"branch": "main"})
	if err != nil {
		t.Fatal(err)
	}
	if result != "https://example.com/$HWT_TEST_SECRET/main" {
		t.Fatalf("ambient environment was expanded: %q", result)
	}
}
