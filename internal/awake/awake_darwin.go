//go:build darwin

package awake

import (
	"os"
	"os/exec"
	"strconv"
	"sync"
)

// caffeinate is already the right tool here. Handing it -w means it exits with
// us even if we're killed, so we can never leak a wake lock.
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
	bin, err := exec.LookPath("caffeinate")
	if err != nil {
		return Noop(), ErrUnsupported
	}
	args := []string{"-i", "-m"}
	if o.Display {
		args = append(args, "-d")
	}
	args = append(args, "-w", strconv.Itoa(os.Getpid()))

	cmd := exec.Command(bin, args...)
	if err := cmd.Start(); err != nil {
		return Noop(), err
	}
	go func() { _ = cmd.Wait() }()

	m := "caffeinate -im"
	if o.Display {
		m = "caffeinate -dim"
	}
	return &procLock{cmd: cmd, method: m}, nil
}
