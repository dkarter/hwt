//go:build darwin || linux

package worktree

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"github.com/creack/pty"
	"github.com/dkarter/hwt/internal/config"
)

// Exercise both subprocess boundaries: HWT inherits terminal stdin/stderr,
// then launches a picker with captured JSON stdout and terminal UI on stderr.
func TestRunTicketCommandInteractiveTerminal(t *testing.T) {
	for _, test := range []struct {
		name string
		key  string
		want string
	}{
		{name: "select", key: "\r", want: "feature/selected"},
		{name: "cancel", key: "\x1b", want: "exit status 23"},
	} {
		t.Run(test.name, func(t *testing.T) {
			master, slave, err := pty.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer slave.Close()
			// Register a nonblocking descriptor with Go's poller so deadlines
			// and Close can interrupt the UI reader, including on Darwin.
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
			master = os.NewFile(uintptr(fd), "picker-pty")
			defer master.Close()
			if err := pty.Setsize(slave, &pty.Winsize{Rows: 30, Cols: 100}); err != nil {
				t.Fatal(err)
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			deadline, _ := ctx.Deadline()
			if err := master.SetReadDeadline(deadline); err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(ctx, executable, "-test.run=^TestTicketPickerProcess$", "--", "transport")
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			// Cancel the picker as well as its transport parent on timeout.
			cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
			cmd.WaitDelay = 2 * time.Second
			cmd.Stdin, cmd.Stderr = slave, slave
			var stdout bytes.Buffer
			cmd.Stdout = &stdout
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			// Read while the UI runs; wait for the measured size before sending input.
			uiDone := make(chan string, 1)
			go func() {
				var ui strings.Builder
				buf := make([]byte, 4096)
				sent := false
				for {
					n, err := master.Read(buf)
					ui.Write(buf[:n])
					if !sent && strings.Contains(ui.String(), "PICKER_READY 100x30") {
						_, _ = io.WriteString(master, test.key)
						sent = true
					}
					if err != nil {
						uiDone <- ui.String()
						return
					}
				}
			}()
			waitErr := cmd.Wait()
			_ = slave.Close()
			// Darwin PTYs need the master closed to unblock a read after slave exit.
			_ = master.Close()
			ui := <-uiDone
			if waitErr != nil {
				t.Fatalf("transport: %v; UI: %q; stdout: %q", waitErr, ui, stdout.String())
			}
			if !strings.Contains(ui, "PICKER_READY 100x30") {
				t.Fatalf("picker did not receive terminal dimensions: %q", ui)
			}
			var result struct {
				Branch   string            `json:"branch"`
				Metadata map[string]string `json:"metadata"`
				Error    string            `json:"error"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("UI leaked into JSON stdout %q: %v", stdout.String(), err)
			}
			if test.name == "select" {
				if result.Branch != test.want || result.Metadata["identifier"] != "TEST-123" || result.Error != "" {
					t.Fatalf("selected result = %#v", result)
				}
			} else if !strings.Contains(result.Error, test.want) || result.Branch != "" {
				t.Fatalf("cancellation result = %#v", result)
			}
		})
	}
}

// Invoked only as a subprocess, with fake ticket data and no API access.
func TestTicketPickerProcess(t *testing.T) {
	args := os.Args
	if len(args) < 2 || args[len(args)-2] != "--" {
		return
	}
	switch args[len(args)-1] {
	case "transport":
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		result, err := runTicketCommand("", config.TicketCommand{
			Command: []string{executable, "-test.run=^TestTicketPickerProcess$", "--", "picker"},
		}, "")
		output := map[string]any{"branch": result.BranchName, "metadata": result.Metadata}
		if err != nil {
			output["error"] = err.Error()
		}
		if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
			t.Fatal(err)
		}
		os.Exit(0) // Keep the test runner's PASS line out of machine-readable stdout.
	case "picker":
		if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stderr.Fd()) {
			fmt.Fprintln(os.Stderr, "picker stdin/stderr must be terminals")
			os.Exit(2)
		}
		result, err := tea.NewProgram(ticketPickerTestModel{}, tea.WithInput(os.Stdin), tea.WithOutput(os.Stderr)).Run()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		model := result.(ticketPickerTestModel)
		if model.selected && model.width == 100 && model.height == 30 {
			fmt.Println(`{"branchName":"feature/selected","metadata":{"identifier":"TEST-123"}}`)
			os.Exit(0)
		}
		if model.cancelled {
			os.Exit(23)
		}
		fmt.Fprintln(os.Stderr, "picker timed out or received incorrect dimensions")
		os.Exit(2)
	}
}

type ticketPickerTestTimeout struct{}

type ticketPickerTestModel struct {
	width, height int
	selected      bool
	cancelled     bool
}

func (m ticketPickerTestModel) Init() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return ticketPickerTestTimeout{} })
}

func (m ticketPickerTestModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			m.selected = true
			return m, tea.Quit
		case "esc":
			m.cancelled = true
			return m, tea.Quit
		}
	case ticketPickerTestTimeout:
		return m, tea.Quit
	}
	return m, nil
}

func (m ticketPickerTestModel) View() tea.View {
	view := tea.NewView(fmt.Sprintf("PICKER_READY %dx%d\nEnter selects; Escape cancels", m.width, m.height))
	view.AltScreen = true
	return view
}
