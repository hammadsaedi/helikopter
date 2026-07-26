// Package term wraps the small amount of terminal control the animation needs:
// size, raw-mode key reads, the alternate screen, and cursor visibility.
package term

import (
	"os"

	xterm "golang.org/x/term"
)

type Term struct {
	In  *os.File
	Out *os.File

	fd       int
	oldState *xterm.State
	inAlt    bool
	keys     chan byte
}

// Open wires up stdin/stdout. It never fails: if there is no terminal, IsTTY
// reports false and the control methods become no-ops.
func Open() *Term {
	t := &Term{In: os.Stdin, Out: os.Stdout}
	t.fd = int(t.Out.Fd())
	return t
}

func (t *Term) IsTTY() bool { return xterm.IsTerminal(t.fd) }

// Size returns the terminal dimensions, falling back to 80x24.
func (t *Term) Size() (cols, rows int) {
	c, r, err := xterm.GetSize(t.fd)
	if err != nil || c <= 0 || r <= 0 {
		return 80, 24
	}
	return c, r
}

// MakeRaw puts the input terminal into raw mode so single keypresses arrive
// without a newline and are not echoed.
func (t *Term) MakeRaw() error {
	fd := int(t.In.Fd())
	if !xterm.IsTerminal(fd) {
		return nil
	}
	st, err := xterm.MakeRaw(fd)
	if err != nil {
		return err
	}
	t.oldState = st
	return nil
}

func (t *Term) RestoreMode() {
	if t.oldState != nil {
		_ = xterm.Restore(int(t.In.Fd()), t.oldState)
		t.oldState = nil
	}
}

// Keys streams keypresses. The reader goroutine outlives the call; it ends when
// stdin closes, which happens on exit.
func (t *Term) Keys() <-chan byte {
	if t.keys != nil {
		return t.keys
	}
	t.keys = make(chan byte, 16)
	go func() {
		buf := make([]byte, 8)
		for {
			n, err := t.In.Read(buf)
			if err != nil {
				close(t.keys)
				return
			}
			for _, b := range buf[:n] {
				select {
				case t.keys <- b:
				default:
				}
			}
		}
	}()
	return t.keys
}

func (t *Term) write(s string) {
	if t.Out != nil {
		_, _ = t.Out.WriteString(s)
	}
}

func (t *Term) EnterAlt() {
	if t.inAlt {
		return
	}
	// ?7l disables auto-wrap. Without it, a single character too many on any
	// row wraps and scrolls the whole alternate screen, which corrupts every
	// subsequent differential update.
	t.write("\x1b[?1049h\x1b[?25l\x1b[?7l\x1b[2J")
	t.inAlt = true
}

func (t *Term) ExitAlt() {
	if !t.inAlt {
		return
	}
	t.write("\x1b[0m\x1b[?7h\x1b[?25h\x1b[?1049l")
	t.inAlt = false
}

// Cleanup restores everything. Safe to call more than once.
func (t *Term) Cleanup() {
	t.ExitAlt()
	t.RestoreMode()
}
