package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	vt "github.com/charmbracelet/x/vt"

	"toci/internal/registry"
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
		msg  tea.KeyPressMsg
		want string
	}{
		{"rune", tea.KeyPressMsg{Code: 'a', Text: "a"}, "a"},
		{"ctrl+c", tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, "\x03"},
		{"enter", tea.KeyPressMsg{Code: tea.KeyEnter}, "\r"},
		{"tab", tea.KeyPressMsg{Code: tea.KeyTab}, "\t"},
		{"shift+tab", tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, "\x1b[Z"},
		{"backspace", tea.KeyPressMsg{Code: tea.KeyBackspace}, "\x7f"},
		{"esc", tea.KeyPressMsg{Code: tea.KeyEsc}, "\x1b"},
		{"up", tea.KeyPressMsg{Code: tea.KeyUp}, "\x1b[A"},
		{"alt+a", tea.KeyPressMsg{Code: 'a', Text: "a", Mod: tea.ModAlt}, "\x1ba"},
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

func newEmbTermTestModel(t *testing.T) Model {
	t.Helper()
	m := Model{
		resources: registry.All(nil),
		compPath:  []crumb{{ID: "root", Name: "root"}},
		table:     newTable(20),
		mode:      modeEmbeddedTerm,
	}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return mm.(Model)
}

func TestViewPlacesCursorOverEmbeddedTermCursor(t *testing.T) {
	m := newEmbTermTestModel(t)
	cols, rows := m.embTermSize()
	emu := vt.NewSafeEmulator(cols, rows)
	// Move the pty cursor to (3, 2) — writing a newline advances the row,
	// writing runes advances the column.
	emu.Write([]byte("xy\r\nabc"))
	m.embTerm = &embeddedTerm{emu: emu}

	v := m.View()
	if v.Cursor == nil {
		t.Fatal("Cursor is nil, want it set to the ssh session's live cursor position")
	}
	if got, want := v.Cursor.X, 3+embTermContentCol; got != want {
		t.Errorf("Cursor.X = %d, want %d", got, want)
	}
	if got, want := v.Cursor.Y, 1+embTermContentRow; got != want {
		t.Errorf("Cursor.Y = %d, want %d", got, want)
	}
}

func TestViewHidesCursorWhenScrolledBack(t *testing.T) {
	m := newEmbTermTestModel(t)
	cols, rows := m.embTermSize()
	m.embTerm = &embeddedTerm{emu: vt.NewSafeEmulator(cols, rows), scrollback: 1}

	if v := m.View(); v.Cursor != nil {
		t.Errorf("Cursor = %+v, want nil while scrolled back", v.Cursor)
	}
}

func TestViewHidesCursorOutsideEmbeddedTermMode(t *testing.T) {
	m := newEmbTermTestModel(t)
	m.mode = modeTable

	if v := m.View(); v.Cursor != nil {
		t.Errorf("Cursor = %+v, want nil outside modeEmbeddedTerm", v.Cursor)
	}
}

// TestViewDisablesMouseTrackingInEmbeddedTermMode reproduces a reported
// bug: toci's own mouse tracking (needed elsewhere for row/menu clicks)
// stayed on inside an ssh session too, which made the real terminal route
// mouse drags to us instead of its own native drag-to-copy selection.
// TestRenderCapturesCursorPositionAlongsideContent reproduces a reported
// bug: View() used to call emu.CursorPosition() on its own, well after
// render() had already captured the content — long enough (building the
// rest of the screen) for the readLoop goroutine to write more pty output
// into the emulator in between, most visible right when a burst of output
// (e.g. `history`) overflows the box and scrolls several lines through in
// quick succession. That gap is closed by having render() itself capture
// lastCursor in the same breath as the content it returns.
func TestRenderCapturesCursorPositionAlongsideContent(t *testing.T) {
	cols, rows := 20, 5
	emu := vt.NewSafeEmulator(cols, rows)
	emu.Write([]byte("xy\r\nabc"))
	et := &embeddedTerm{emu: emu}

	et.render(rows)

	want := emu.CursorPosition()
	if et.lastCursor != want {
		t.Errorf("lastCursor = %+v, want %+v (emu's cursor position as of the render() call)", et.lastCursor, want)
	}
	if want.X != 3 || want.Y != 1 {
		t.Fatalf("test setup: cursor at %+v, want (3, 1) after writing \"xy\\r\\nabc\"", want)
	}
}

func TestViewDisablesMouseTrackingInEmbeddedTermMode(t *testing.T) {
	m := newEmbTermTestModel(t)
	cols, rows := m.embTermSize()
	m.embTerm = &embeddedTerm{emu: vt.NewSafeEmulator(cols, rows)}

	if v := m.View(); v.MouseMode != tea.MouseModeNone {
		t.Errorf("MouseMode = %v, want MouseModeNone inside an embedded ssh session", v.MouseMode)
	}

	m.mode = modeTable
	if v := m.View(); v.MouseMode != tea.MouseModeCellMotion {
		t.Errorf("MouseMode = %v, want MouseModeCellMotion outside modeEmbeddedTerm", v.MouseMode)
	}
}
