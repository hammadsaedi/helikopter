package main

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"v1.0.0", "1.0.0", 0},
		{"1.0.0", "v1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.1.0", -1},
		{"1.0.0", "2.0.0", -1},
		// String comparison gets these two wrong, which is the whole reason
		// this is not just a ==.
		{"1.9.0", "1.10.0", -1},
		{"1.10.0", "1.9.0", 1},
		{"v2.0.0", "v10.0.0", -1},
		// Pre-release and build suffixes are ignored.
		{"1.2.3-rc1", "1.2.3", 0},
		{"1.2.3+abc", "1.2.3", 0},
		{"v1.2.3-rc1", "1.2.4", -1},
		// Short and malformed tags must not panic.
		{"1.2", "1.2.0", 0},
		{"1", "1.0.0", 0},
		{"", "1.0.0", -1},
		{"garbage", "1.0.0", -1},
		{"1.0.0", "garbage", 1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestCompareVersionsIsAntisymmetric(t *testing.T) {
	vs := []string{"0.9.9", "1.0.0", "1.0.1", "1.2.0", "1.10.0", "2.0.0", "v3.1.4"}
	for _, a := range vs {
		for _, b := range vs {
			if got, rev := compareVersions(a, b), compareVersions(b, a); got != -rev {
				t.Errorf("compareVersions(%q,%q)=%d but reversed=%d", a, b, got, rev)
			}
		}
	}
}

func TestParseVersion(t *testing.T) {
	for in, want := range map[string][3]int{
		"1.2.3":    {1, 2, 3},
		"v1.2.3":   {1, 2, 3},
		" v1.2.3 ": {1, 2, 3},
		"1.2":      {1, 2, 0},
		"10.20.30": {10, 20, 30},
		"1.2.3-rc": {1, 2, 3},
		"nonsense": {0, 0, 0},
	} {
		if got := parseVersion(in); got != want {
			t.Errorf("parseVersion(%q) = %v, want %v", in, got, want)
		}
	}
}

// The hint has to name a route the user could plausibly have installed by, and
// always the one that works regardless.
func TestUpdateHintMentionsGoInstall(t *testing.T) {
	h := updateHint()
	if !contains(h, "go install github.com/hammadsaedi/helikopter/cmd/helikopter@latest") {
		t.Errorf("update hint should always offer go install:\n%s", h)
	}
	if len(h) == 0 {
		t.Error("empty update hint")
	}
}

func TestDispatchRecognisesCommands(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"themes"}, {"help"}} {
		handled, err := dispatch(args)
		if !handled {
			t.Errorf("%v should be handled as a command", args)
		}
		if err != nil {
			t.Errorf("%v returned %v", args, err)
		}
	}
}

func TestDispatchLeavesFlagsAndTheBareCommandAlone(t *testing.T) {
	for _, args := range [][]string{{}, {"--theme", "night"}, {"-s"}, {"--idle"}} {
		if handled, _ := dispatch(args); handled {
			t.Errorf("%v is not a command and should fall through to the animation", args)
		}
	}
}

func TestDispatchRejectsUnknownCommands(t *testing.T) {
	handled, err := dispatch([]string{"fly"})
	if !handled {
		t.Fatal("an unknown verb should be reported, not silently flown")
	}
	if err == nil {
		t.Fatal("expected an error")
	}
	// The message has to say what is available, or the user is left guessing.
	for _, want := range []string{"update", "version", "themes"} {
		if !contains(err.Error(), want) {
			t.Errorf("error should list %q: %v", want, err)
		}
	}
}

// "flag provided but not defined" is accurate and useless. These are the
// mistakes people actually make, so each should say where the thing went.
func TestUpdateFlagMistakesAreExplained(t *testing.T) {
	handled, err := dispatch([]string{"update", "--check-update"})
	if !handled || err == nil {
		t.Fatal("an unknown flag on update should be an error")
	}
	msg := err.Error()
	if !contains(msg, "Did you mean") || !contains(msg, "helikopter update --check") {
		t.Errorf("a near miss for --check should be suggested:\n%s", msg)
	}
	// Reported once. The flag set is silenced precisely so it is not printed
	// by both the parser and main.
	if n := countOccurrences(msg, "usage: helikopter update"); n != 1 {
		t.Errorf("usage appears %d times, want 1:\n%s", n, msg)
	}

	// A flag that is not a near miss still gets the usage, without a bogus
	// suggestion.
	_, err = dispatch([]string{"update", "--bogus"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if contains(err.Error(), "Did you mean") {
		t.Errorf("--bogus is not a near miss for --check:\n%s", err)
	}
}

func TestHelpOnUpdateIsNotAnError(t *testing.T) {
	handled, err := dispatch([]string{"update", "--help"})
	if !handled {
		t.Fatal("update --help should be handled")
	}
	if err != nil {
		t.Errorf("asking for help is not a failure: %v", err)
	}
}

// The check used to be a top-level flag and is now behind the command, so the
// flag spelling has to point at where it went.
func TestUpdateAsAFlagPointsAtTheCommand(t *testing.T) {
	for _, arg := range []string{"--check-update", "-check-update", "--update"} {
		handled, err := dispatch([]string{arg})
		if !handled || err == nil {
			t.Fatalf("%s should be reported, got handled=%v err=%v", arg, handled, err)
		}
		if !contains(err.Error(), "helikopter update") {
			t.Errorf("%s should point at the command:\n%s", arg, err)
		}
	}

	// Real flags must still fall through to the animation.
	for _, arg := range []string{"--theme", "--idle", "-s", "--no-awake"} {
		if handled, _ := dispatch([]string{arg}); handled {
			t.Errorf("%s should fall through to the animation", arg)
		}
	}
}

func countOccurrences(hay, needle string) int {
	n := 0
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			n++
		}
	}
	return n
}

// The hint has to lead with the command that does the thing. Telling someone
// an update exists and then listing every route except `helikopter update` —
// while the program is running and already knows — is absurd.
func TestUpdateHintLeadsWithTheCommand(t *testing.T) {
	h := updateHint()
	cmd := indexOf(h, "helikopter update")
	if cmd < 0 {
		t.Fatalf("the hint must offer helikopter update:\n%s", h)
	}
	for _, other := range []string{"install.sh", "install.ps1", "go install"} {
		if i := indexOf(h, other); i >= 0 && i < cmd {
			t.Errorf("%q is suggested before `helikopter update`:\n%s", other, h)
		}
	}
}

// Suggesting a channel that is not published sends people to an error. This
// one cost a real "No available formula with the name helikopter".
func TestUpdateHintOffersOnlyWorkingRoutes(t *testing.T) {
	h := updateHint()
	for _, unpublished := range []string{"brew", "scoop", "winget"} {
		if contains(h, unpublished) {
			t.Errorf("%q is not published yet, so the hint must not suggest it:\n%s",
				unpublished, h)
		}
	}
	if !contains(h, "go install github.com/hammadsaedi/helikopter/cmd/helikopter@latest") {
		t.Errorf("go install always works and should always be offered:\n%s", h)
	}
}

func indexOf(hay, needle string) int {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
