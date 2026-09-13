package app

import (
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	vt "github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
)

// embeddedTerm runs an interactive shell command (typically an ssh
// invocation) in a real pty and feeds its output into a virtual terminal
// emulator, so the session can be rendered as part of toci's own bubbletea
// view instead of handing the whole screen over to an external process —
// the same trick editors like Neovim use for their built-in `:terminal`.
type embeddedTerm struct {
	cmd *exec.Cmd
	pty *os.File
	emu *vt.SafeEmulator

	changed   chan struct{}
	done      chan struct{}
	exitErr   error
	startedAt time.Time

	// killed marks an intentional close() (the ctrl+\ escape hatch) so the
	// embTermExitMsg that follows shortly after — cmd.Wait() reports a
	// SIGKILLed process as an error, "signal: killed" — isn't mistaken for
	// an actual connection failure.
	killed bool

	// scrollback is how many lines up from the live bottom the view is
	// currently scrolled — 0 means "following the live screen" (the
	// normal state). Only meaningful outside the remote app's alt screen;
	// see render.
	scrollback int
}

// startEmbeddedTerm launches shellCmd (via "sh -c") attached to a new pty
// sized to cols x rows.
func startEmbeddedTerm(shellCmd string, cols, rows int) (*embeddedTerm, error) {
	cmd := exec.Command("sh", "-c", shellCmd)
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	et := &embeddedTerm{
		cmd:       cmd,
		pty:       f,
		emu:       vt.NewSafeEmulator(cols, rows),
		changed:   make(chan struct{}, 1),
		done:      make(chan struct{}),
		startedAt: time.Now(),
	}
	go et.readLoop()
	go et.replyLoop()
	return et, nil
}

// quickFailWindow is how soon after starting an exit counts as "never
// really connected" rather than "was connected, then dropped" — see
// quickFail. A real ssh auth/connection failure exits in well under a
// second; a session that's actually been used runs far longer than this.
//
// ponytail: a fixed threshold, not "did we ever see a shell prompt" — a
// pathologically slow network handshake that then fails would slip past
// this and just show as a normal error instead of triggering the
// create-a-fresh-session retry. Upgrade path: track whether any pty output
// was ever received instead of timing it.
const quickFailWindow = 5 * time.Second

// quickFail reports whether the process exited implausibly soon after
// starting — the signal a caller uses to tell "this session/key doesn't
// actually work" apart from a normal, later disconnect.
func (et *embeddedTerm) quickFail() bool {
	return time.Since(et.startedAt) < quickFailWindow
}

// readLoop feeds pty output into the emulator until the pty closes (the
// shell/ssh process exited), then records the process's exit error and
// closes done. Runs for the lifetime of the session in its own goroutine.
func (et *embeddedTerm) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := et.pty.Read(buf)
		if n > 0 {
			_, _ = et.emu.Write(buf[:n])
			select {
			case et.changed <- struct{}{}:
			default: // a redraw is already pending
			}
		}
		if err != nil {
			et.exitErr = et.cmd.Wait()
			_ = et.emu.Close() // unblocks replyLoop's Read
			close(et.done)
			return
		}
	}
}

// replyLoop drains the emulator's own generated replies (device attribute
// queries, cursor position reports, OSC color queries, focus events, ...
// anything a real terminal would answer on its own) and writes them back
// into the pty. Without this, vt.Emulator's handlers block forever trying
// to write a reply nobody's reading (Write on the emulator would then
// never return, freezing readLoop) the moment a program probes for
// terminal capabilities — tmux does this on startup, which is why a
// plain shell could look fine while tmux immediately hung.
func (et *embeddedTerm) replyLoop() {
	buf := make([]byte, 1024)
	for {
		n, err := et.emu.Read(buf)
		if n > 0 {
			_, _ = et.pty.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// waitForActivity returns a tea.Cmd that blocks until new output has been
// written to the emulator or the process exits. The caller re-issues this
// command after every embTermActivityMsg to keep listening.
func (et *embeddedTerm) waitForActivity() tea.Cmd {
	return func() tea.Msg {
		select {
		case <-et.changed:
			return embTermActivityMsg{}
		case <-et.done:
			return embTermExitMsg{err: et.exitErr}
		}
	}
}

func (et *embeddedTerm) resize(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	et.emu.Resize(cols, rows)
	_ = pty.Setsize(et.pty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

// scrollUp/scrollDown move the scrollback view; a remote full-screen app
// (vim, less) using the alt screen has no scrollback of its own to show,
// so scrolling is a no-op there — matches how a real terminal emulator
// behaves.
func (et *embeddedTerm) scrollUp(n int) {
	if et.emu.IsAltScreen() {
		return
	}
	et.scrollback += n
	if max := et.emu.Scrollback().Len(); et.scrollback > max {
		et.scrollback = max
	}
}

func (et *embeddedTerm) scrollDown(n int) {
	et.scrollback -= n
	if et.scrollback < 0 {
		et.scrollback = 0
	}
}

func (et *embeddedTerm) resetScroll() {
	et.scrollback = 0
}

// render returns the rows lines that should currently be visible: the live
// screen when scrollback is 0 (the common case, and the only ANSI-styled
// string vt.Emulator.Render() actually knows how to produce), or a blend
// of scrollback lines and the top of the live screen when scrolled up.
func (et *embeddedTerm) render(rows int) string {
	if et.scrollback == 0 {
		return et.emu.Render()
	}
	sb := et.emu.Scrollback()
	sbLen := sb.Len()
	screenLines := strings.Split(et.emu.Render(), "\n")

	off := et.scrollback
	if off > sbLen {
		off = sbLen
	}
	start := sbLen + len(screenLines) - rows - off
	if start < 0 {
		start = 0
	}

	lines := make([]string, 0, rows)
	for i := start; i < start+rows; i++ {
		switch {
		case i < sbLen:
			lines = append(lines, sb.Line(i).Render())
		case i-sbLen < len(screenLines):
			lines = append(lines, screenLines[i-sbLen])
		default:
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}

// close force-terminates the session — used for the manual "give up on
// this session" escape hatch, not the normal exit path (a clean remote
// `exit` is picked up by readLoop's io.EOF instead).
func (et *embeddedTerm) close() {
	et.killed = true
	if et.cmd.Process != nil {
		_ = et.cmd.Process.Kill()
	}
	_ = et.pty.Close()
	_ = et.emu.Close() // unblocks replyLoop's Read
}

type embTermActivityMsg struct{}
type embTermExitMsg struct{ err error }

// keyMsgToBytes translates a decoded bubbletea key event back into the raw
// bytes a terminal would have sent for it, to forward into the pty.
//
// ponytail: arrow/nav/function keys always encode as "normal" (non
// DECCKM/application) mode sequences — a nested full-screen app (vim, less
// -X) that flips the terminal into application-cursor-key mode won't get
// the alternate escapes it asked for. Upgrade path: track mode changes via
// the vt.Emulator's callback hooks and switch encodings accordingly.
func keyMsgToBytes(msg tea.KeyPressMsg) []byte {
	if msg.Mod.Contains(tea.ModAlt) {
		return append([]byte{0x1b}, keyMsgToBytesNoAlt(msg)...)
	}
	return keyMsgToBytesNoAlt(msg)
}

func keyMsgToBytesNoAlt(msg tea.KeyPressMsg) []byte {
	if len(msg.Text) > 0 {
		return []byte(msg.Text)
	}
	// shift+tab shares KeyTab's Code with plain tab — Mod is the only thing
	// that tells them apart, so it has to be checked before the Code-only
	// lookups below.
	if msg.Code == tea.KeyTab && msg.Mod.Contains(tea.ModShift) {
		return []byte("\x1b[Z")
	}
	if seq, ok := namedKeySequences[msg.Code]; ok {
		return []byte(seq)
	}
	// The plain C0 controls (enter, tab, backspace, esc) carry their
	// literal control byte as Code already — see bubbletea's key.go
	// (KeyEnter = '\r', KeyTab = '\t', etc).
	if msg.Code >= 0 && msg.Code <= 31 || msg.Code == 127 {
		return []byte{byte(msg.Code)}
	}
	// Unlike v1, ctrl+letter combos carry the *base* letter as Code (e.g.
	// 'c') with Mod holding ModCtrl separately, rather than the control
	// byte itself — mask it down the same way a real terminal would.
	if msg.Mod.Contains(tea.ModCtrl) && msg.Code > 0 && msg.Code < 128 {
		return []byte{byte(msg.Code) & 0x1f}
	}
	return nil
}

// namedKeySequences covers the keys bubbletea decodes to a Code above the
// literal control-byte range that a remote terminal application still
// expects to see as a normal-mode ANSI escape sequence.
var namedKeySequences = map[rune]string{
	tea.KeyUp:     "\x1b[A",
	tea.KeyDown:   "\x1b[B",
	tea.KeyRight:  "\x1b[C",
	tea.KeyLeft:   "\x1b[D",
	tea.KeyHome:   "\x1b[H",
	tea.KeyEnd:    "\x1b[F",
	tea.KeyPgUp:   "\x1b[5~",
	tea.KeyPgDown: "\x1b[6~",
	tea.KeyDelete: "\x1b[3~",
	tea.KeyInsert: "\x1b[2~",
	tea.KeySpace:  " ",
	tea.KeyF1:     "\x1bOP",
	tea.KeyF2:     "\x1bOQ",
	tea.KeyF3:     "\x1bOR",
	tea.KeyF4:     "\x1bOS",
	tea.KeyF5:     "\x1b[15~",
	tea.KeyF6:     "\x1b[17~",
	tea.KeyF7:     "\x1b[18~",
	tea.KeyF8:     "\x1b[19~",
	tea.KeyF9:     "\x1b[20~",
	tea.KeyF10:    "\x1b[21~",
	tea.KeyF11:    "\x1b[23~",
	tea.KeyF12:    "\x1b[24~",
}
