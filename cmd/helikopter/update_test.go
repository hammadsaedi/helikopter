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
