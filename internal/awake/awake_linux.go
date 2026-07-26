//go:build linux

package awake

import (
	"os/exec"
	"sync"
)

// The lock is a child process that holds an inhibitor for its own lifetime, so
// killing it releases the lock and nothing is left behind if we crash.
type procLock struct {
	cmd    *exec.Cmd
	method string
	once   sync.Once
}

func (p *procLock) Method() string { return p.method }

func (p *procLock) Release() {
	p.once.Do(func() {
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
			_, _ = p.cmd.Process.Wait()
		}
	})
}

func acquire(o Options) (Lock, error) {
	what := "sleep:idle"
	if o.Display {
		what = "sleep:idle:handle-lid-switch"
	}

	candidates := []struct {
		bin    string
		args   []string
		method string
	}{
		{"systemd-inhibit", []string{
			"--what=" + what,
			"--who=helikopter",
			"--why=helikopter is flying",
			"--mode=block",
			"sleep", "infinity",
		}, "systemd-inhibit"},

		{"gnome-session-inhibit", []string{
			"--app-id", "helikopter",
			"--reason", "helikopter is flying",
			"--inhibit", "idle:suspend",
			"sleep", "infinity",
		}, "gnome-session-inhibit"},
	}
	// Deliberately no xset fallback: `xset s off -dpms` mutates global X
	// settings and exits, so a crash would leave the display permanently
	// awake. Only inhibitors scoped to a child process are used here.

	for _, c := range candidates {
		bin, err := exec.LookPath(c.bin)
		if err != nil {
			continue
		}
		cmd := exec.Command(bin, c.args...)
		if err := cmd.Start(); err != nil {
			continue
		}
		go func() { _ = cmd.Wait() }()
		return &procLock{cmd: cmd, method: c.method}, nil
	}

	return Noop(), ErrUnsupported
}
