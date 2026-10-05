//go:build darwin || linux

package e2e

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestREV010_TicketPickerInheritsTerminalWithoutShellRedirection(t *testing.T) {
	s := newSandbox(t)
	repo := s.repo()
	herdr := s.fakeHerdr(repo)
	s.tool("tickets", `
[ -t 0 ] && [ -t 2 ] || { printf 'picker needs terminal stdin/stderr\n' >&2; exit 2; }
size=$(/bin/stty size <&2)
[ "$size" = '30 100' ] || { printf 'incorrect dimensions: %s\n' "$size" >&2; exit 2; }
printf 'PICKER_READY %s\n' "$size" >&2
IFS= read -r selection
[ "$selection" = select ] || exit 23
printf '%s\n' '{"branchName":"test-123-selected","metadata":{"identifier":"TEST-123"}}'
`)
	mustWrite(t, filepath.Join(repo, ".herdr-worktree.yaml"), "ticket_commands:\n  default:\n    command: [tickets, '{input}']\n", 0o600)
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer slave.Close()
	fd, err := syscall.Dup(int(master.Fd()))
	_ = master.Close()
	if err != nil {
		t.Fatal(err)
	}
	syscall.CloseOnExec(fd)
	if err := syscall.SetNonblock(fd, true); err != nil {
		_ = syscall.Close(fd)
		t.Fatal(err)
	}
	master = os.NewFile(uintptr(fd), "ticket-pty")
	defer master.Close()
	if err := master.SetReadDeadline(time.Now().Add(15 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := pty.Setsize(slave, &pty.Winsize{Rows: 30, Cols: 100}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, hwtBinary, "--herdr-bin", herdr, "create", "--ticket", "--json")
	cmd.Dir, cmd.Env = repo, s.env
	cmd.Stdin, cmd.Stderr = slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	var stdout strings.Builder
	cmd.Stdout = &stdout
	// This canonical-mode input is buffered until the fake picker reads it.
	if _, err := io.WriteString(master, "select\n"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Run(); err != nil {
		t.Fatalf("ticket create failed: %v; stdout: %s", err, stdout.String())
	}
	result := decode(t, stdout.String())
	if result["branch"] != "test-123-selected" {
		t.Fatalf("selected branch = %#v", result)
	}
	gitDir := s.git(result["path"].(string), "rev-parse", "--git-dir")
	metadata := mustRead(t, filepath.Join(gitDir, "hwt-ticket-metadata-v1.json"))
	requireContains(t, metadata, "TEST-123")
	ui := make([]byte, 4096)
	n, err := master.Read(ui)
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, string(ui[:n]), "PICKER_READY 30 100")
}
