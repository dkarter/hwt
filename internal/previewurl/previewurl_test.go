package previewurl

import (
	"strings"
	"testing"
)

func TestExpandEscapesComponents(t *testing.T) {
	template := "https://{sanitized_branch}.preview.example/{repository}/{branch}?worktree={worktree}"
	values := map[string]string{
		"repository":       "my app",
		"branch":           "Feature/café + tea",
		"sanitized_branch": SanitizeBranch("Feature/café + tea"),
		"worktree":         "feature café",
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
		{name: "unknown", template: "https://example.com/{ref}", want: "unknown placeholder"},
		{name: "unclosed", template: "https://example.com/{branch", want: "unclosed"},
		{name: "relative", template: "/{branch}", want: "absolute http or https"},
		{name: "invalid escape", template: "https://example.com/%zz/{branch}", want: "valid URL"},
		{name: "missing value", template: "https://example.com/{worktree}", values: map[string]string{"branch": "main"}, want: "unavailable"},
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
