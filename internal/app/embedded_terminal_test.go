package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEmbeddedTermRunsAndRenders(t *testing.T) {
	et, err := startEmbeddedTerm("echo hello-toci", 40, 10)
	if err != nil {
		t.Fatalf("startEmbeddedTerm: %v", err)
	}
	defer et.close()

	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-et.changed:
			if strings.Contains(et.emu.Render(), "hello-toci") {
				return
			}
		case <-et.done:
			if strings.Contains(et.emu.Render(), "hello-toci") {
				return
			}
			t.Fatalf("process exited without ever rendering expected output; render:\n%s", et.emu.Render())
		case <-deadline:
			t.Fatalf("timed out waiting for output; render so far:\n%s", et.emu.Render())
		}
	}
}

// TestEmbeddedTermAnswersTerminalQueries guards against the deadlock this
// package hit for real: vt.Emulator answers terminal capability queries
// (device attributes, cursor position reports, ...) by writing into its
// own internal pipe. Without something draining that pipe (replyLoop),
// the emulator's Write call — invoked from readLoop for every chunk of
// pty output — blocks forever the moment output contains one of these
// queries, freezing the whole session. tmux sends several on startup;
// this reproduces it with a plain DSR query instead of a real tmux
// dependency.
func TestEmbeddedTermAnswersTerminalQueries(t *testing.T) {
	et, err := startEmbeddedTerm(`printf '\033[6n'; echo done-marker`, 40, 10)
	if err != nil {
		t.Fatalf("startEmbeddedTerm: %v", err)
	}
	defer et.close()

	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-et.changed:
			if strings.Contains(et.emu.Render(), "done-marker") {
				return
			}
		case <-et.done:
			t.Fatalf("process exited without ever rendering expected output; render:\n%s", et.emu.Render())
		case <-deadline:
			t.Fatalf("timed out — readLoop likely deadlocked writing a terminal query reply nobody drains")
		}
	}
}

func TestKeyMsgToBytes(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.KeyMsg
		want string
	}{
		{"rune", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}, "a"},
		{"ctrl+c", tea.KeyMsg{Type: tea.KeyCtrlC}, "\x03"},
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}, "\r"},
		{"tab", tea.KeyMsg{Type: tea.KeyTab}, "\t"},
		{"backspace", tea.KeyMsg{Type: tea.KeyBackspace}, "\x7f"},
		{"esc", tea.KeyMsg{Type: tea.KeyEsc}, "\x1b"},
		{"up", tea.KeyMsg{Type: tea.KeyUp}, "\x1b[A"},
		{"alt+a", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a"), Alt: true}, "\x1ba"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(keyMsgToBytes(c.msg))
			if got != c.want {
				t.Errorf("keyMsgToBytes(%+v) = %q, want %q", c.msg, got, c.want)
			}
		})
	}
}
