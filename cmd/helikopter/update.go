package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// The check is deliberately something you ask for rather than something that
// happens. A tool whose selling point is that it sits still and costs nothing
// has no business making network calls on its own, and nobody expects a
// terminal animation to phone home.
const releaseAPI = "https://api.github.com/repos/hammadsaedi/helikopter/releases/latest"

// checkUpdate compares the built-in version against the latest release and
// says what to do about it.
func checkUpdate() error {
	if version == "dev" || strings.Contains(version, "-dirty") {
		fmt.Printf("helikopter %s was built from source, so there is nothing to compare against.\n", version)
		fmt.Println("Update with: go install github.com/hammadsaedi/helikopter/cmd/helikopter@latest")
		return nil
	}

	latest, err := latestVersion()
	if err != nil {
		return fmt.Errorf("could not reach the release list: %w", err)
	}

	switch cmp := compareVersions(version, latest); {
	case cmp < 0:
		fmt.Printf("helikopter %s is out of date; %s is available.\n\n", version, latest)
		fmt.Print(updateHint())
	case cmp > 0:
		fmt.Printf("helikopter %s is newer than the latest release (%s).\n", version, latest)
	default:
		fmt.Printf("helikopter %s is up to date.\n", version)
	}
	return nil
}

func latestVersion() (string, error) {
	req, err := http.NewRequest(http.MethodGet, releaseAPI, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "helikopter/"+version)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github returned %s", resp.Status)
	}

	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return "", err
	}
	if body.TagName == "" {
		return "", fmt.Errorf("no tag in the response")
	}
	return body.TagName, nil
}

// updateHint lists the ways to actually get the new version.
//
// It leads with `helikopter update`, which is the whole reason that command
// exists — suggesting anything else first, when the program is already running
// and already knows there is an update, is absurd.
//
// Every route here has to work. Homebrew, Scoop and winget are prepared under
// packaging/ but not published, and suggesting `brew upgrade helikopter` to
// someone whose only fault was following the advice gets them
// "No available formula". Add each one back here when it is genuinely
// installable.
func updateHint() string {
	var b strings.Builder
	b.WriteString("Update with:\n\n")
	b.WriteString("  helikopter update\n\n")
	b.WriteString("or, if this copy came from somewhere else:\n\n")
	if runtime.GOOS == "windows" {
		b.WriteString("  irm https://hammadsaedi.github.io/helikopter/install.ps1 | iex\n")
	} else {
		b.WriteString("  curl -fsSL https://hammadsaedi.github.io/helikopter/install.sh | sh\n")
	}
	b.WriteString("  go install github.com/hammadsaedi/helikopter/cmd/helikopter@latest\n")
	return b.String()
}

// compareVersions orders two release tags. It returns -1 if a precedes b, +1
// if a follows it, and 0 when they match. Leading "v" is optional and any
// pre-release suffix is ignored, which is all the release tags ever carry.
func compareVersions(a, b string) int {
	pa, pb := parseVersion(a), parseVersion(b)
	for i := 0; i < 3; i++ {
		switch {
		case pa[i] < pb[i]:
			return -1
		case pa[i] > pb[i]:
			return 1
		}
	}
	return 0
}

func parseVersion(s string) [3]int {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	// Drop a pre-release or build suffix: 1.2.3-rc1, 1.2.3+deadbeef.
	if i := strings.IndexAny(s, "-+ "); i >= 0 {
		s = s[:i]
	}

	var out [3]int
	for i, part := range strings.SplitN(s, ".", 3) {
		if i > 2 {
			break
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return out
		}
		out[i] = n
	}
	return out
}
