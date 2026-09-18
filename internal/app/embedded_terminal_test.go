package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
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

// TestEmbTermDragCopiesAndPastePassesThrough covers the clipboard both
// ways: a left drag over the box selects text that lands on the clipboard
// on release (which needs toci's mouse tracking left on in this mode —
// the real terminal's own drag-to-copy can't see the drag, so toci does
// it), and a bracketed paste from the local terminal goes into the pty.
func TestEmbTermDragCopiesAndPastePassesThrough(t *testing.T) {
	m := newEmbTermTestModel(t)
	cols, rows := m.embTermSize()
	m.embTerm = &embeddedTerm{emu: vt.NewSafeEmulator(cols, rows)}
	_, _ = m.embTerm.emu.Write([]byte("hello world"))

	if v := m.View(); v.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("MouseMode = %v, want MouseModeCellMotion so drags reach the selection", v.MouseMode)
	}

	x, y := embTermContentCol, embTermContentRow
	var mm tea.Model = m
	mm, _ = mm.Update(tea.MouseClickMsg{X: x + 6, Y: y, Button: tea.MouseLeft})
	mm, _ = mm.Update(tea.MouseMotionMsg{X: x + 10, Y: y, Button: tea.MouseLeft})
	if !strings.Contains(mm.(Model).viewContent(), "\x1b[7mworld") {
		t.Errorf("dragged-over text not highlighted")
	}
	mm, cmd := mm.Update(tea.MouseReleaseMsg{X: x + 10, Y: y, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatalf("release produced no clipboard cmd")
	}
	if got := fmt.Sprint(cmd()); got != "world" {
		t.Errorf("clipboard = %q, want %q", got, "world")
	}

	// Paste: emu.Paste writes into the emulator's reply pipe, which
	// replyLoop would forward to the pty — read it directly here.
	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := m.embTerm.emu.Read(buf)
		got <- string(buf[:n])
	}()
	mm.Update(tea.PasteMsg{Content: "ls -la\n"})
	select {
	case s := <-got:
		if s != "ls -la\n" {
			t.Errorf("pasted %q, want %q", s, "ls -la\n")
		}
	case <-time.After(time.Second):
		t.Fatalf("paste never reached the emulator's reply pipe")
	}
}

// TestEmbeddedTermSelection covers drag-to-copy: the text a selection
// resolves to (stream order across rows, trailing blanks trimmed, a
// backwards drag normalised) and the reverse-video highlight render()
// paints over it.
func TestEmbeddedTermSelection(t *testing.T) {
	et := &embeddedTerm{emu: vt.NewSafeEmulator(20, 3)}
	_, _ = et.emu.Write([]byte("hello world\r\nsecond line\r\n"))

	// Dragged backwards, from (5,1) "d" up to (6,0) "w".
	et.sel = &termSelection{start: uv.Pos(5, 1), end: uv.Pos(6, 0)}
	if got, want := et.selectedText(3), "world\nsecond"; got != want {
		t.Fatalf("selectedText = %q, want %q", got, want)
	}

	out := strings.Split(et.render(3), "\n")
	if !strings.Contains(out[0], "\x1b[7mworld") || !strings.Contains(out[1], "\x1b[7msecond") {
		t.Fatalf("selection not highlighted in reverse video:\n%q", out)
	}
	if strings.Contains(out[2], "\x1b[7m") {
		t.Fatalf("row outside the selection got highlighted: %q", out[2])
	}
	if strings.TrimRight(ansi.Strip(out[0]), " ") != "hello world" {
		t.Fatalf("highlight changed the text: %q", ansi.Strip(out[0]))
	}

	// Beyond the live screen: selecting scrollback lines after scrolling up.
	_, _ = et.emu.Write([]byte("third\r\nfourth\r\nfifth\r\n"))
	et.scrollUp(3) // scrollback holds "hello world", "second line", "third"; view now starts at its top
	et.sel = &termSelection{start: uv.Pos(0, 0), end: uv.Pos(2, 1)}
	if got, want := et.selectedText(3), "hello world\nsec"; got != want {
		t.Fatalf("scrollback selectedText = %q, want %q", got, want)
	}
}
