//go:build linux

package awake

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// The inhibitor holds the lock for as long as the command it runs, so we hand
// it a sleep of the requested length.
func sleepArg(d time.Duration) string {
	if d <= 0 {
		return "infinity"
	}
	return strconv.Itoa(int(d.Seconds() + 0.5))
}

func inhibitCandidates(o Options, d time.Duration) []struct {
	bin  string
	args []string
} {
	what := "sleep:idle"
	if o.Display {
		what = "sleep:idle:handle-lid-switch"
	}
	return []struct {
		bin  string
		args []string
	}{
		{"systemd-inhibit", []string{
			"systemd-inhibit",
			"--what=" + what,
			"--who=helikopter",
			"--why=helikopter is holding this machine awake",
			"--mode=block",
			"sleep", sleepArg(d),
		}},
		{"gnome-session-inhibit", []string{
			"gnome-session-inhibit",
			"--app-id", "helikopter",
			"--reason", "helikopter is holding this machine awake",
			"--inhibit", "idle:suspend",
			"sleep", sleepArg(d),
		}},
	}
}

func nativeInhibitor(o Options) (string, bool) {
	for _, c := range inhibitCandidates(o, 0) {
		if _, err := exec.LookPath(c.bin); err == nil {
			return c.bin, true
		}
	}
	return "", false
}

func become(o Options, d time.Duration) error {
	for _, c := range inhibitCandidates(o, d) {
		bin, err := exec.LookPath(c.bin)
		if err != nil {
			continue
		}
		return syscall.Exec(bin, c.args, os.Environ())
	}
	return ErrUnsupported
}
