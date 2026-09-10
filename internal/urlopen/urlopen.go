package urlopen

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

func Open(url string) error {
	return open(runtime.GOOS, exec.Command, url)
}

func open(goos string, command func(string, ...string) *exec.Cmd, url string) error {
	var name string
	switch goos {
	case "darwin":
		name = "open"
	case "linux":
		name = "xdg-open"
	default:
		return fmt.Errorf("opening URLs is unsupported on %s", goos)
	}
	if output, err := command(name, url).CombinedOutput(); err != nil {
		if detail := strings.TrimSpace(string(output)); detail != "" {
			return fmt.Errorf("open URL with %s: %s: %w", name, detail, err)
		}
		return fmt.Errorf("open URL with %s: %w", name, err)
	}
	return nil
}
