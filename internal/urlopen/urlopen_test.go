package urlopen

import (
	"os/exec"
	"strings"
	"testing"
)

func TestOpenUsesPlatformCommand(t *testing.T) {
	for _, test := range []struct {
		goos string
		want string
	}{{"darwin", "open https://example.com"}, {"linux", "xdg-open https://example.com"}} {
		t.Run(test.goos, func(t *testing.T) {
			var actual string
			command := func(name string, args ...string) *exec.Cmd {
				actual = strings.Join(append([]string{name}, args...), " ")
				return exec.Command("true")
			}
			if err := open(test.goos, command, "https://example.com"); err != nil {
				t.Fatal(err)
			}
			if actual != test.want {
				t.Fatalf("command = %q, want %q", actual, test.want)
			}
		})
	}
}

func TestOpenRejectsUnsupportedPlatform(t *testing.T) {
	err := open("windows", exec.Command, "https://example.com")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unexpected error: %v", err)
	}
}
