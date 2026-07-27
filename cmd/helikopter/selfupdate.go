package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const releaseDownload = "https://github.com/hammadsaedi/helikopter/releases/download"

// selfUpdate replaces this binary with the latest release.
//
// The order matters and every step can refuse: work out where we actually
// live, check we may write there, download, verify the checksum, prove the new
// binary runs at all, and only then swap it in. A self-update that leaves
// someone with no working command is worse than one that declines.
func selfUpdate() error {
	if version == "dev" || strings.Contains(version, "-dirty") {
		return fmt.Errorf("this build came from source, so there is nothing to update to.\n" +
			"Rebuild with: go install github.com/hammadsaedi/helikopter/cmd/helikopter@latest")
	}

	latest, err := latestVersion()
	if err != nil {
		return fmt.Errorf("could not reach the release list: %w", err)
	}
	if compareVersions(version, latest) >= 0 {
		fmt.Printf("helikopter %s is already up to date.\n", version)
		return nil
	}

	exe, err := currentExecutable()
	if err != nil {
		return err
	}
	if err := writable(filepath.Dir(exe)); err != nil {
		return fmt.Errorf("%s is not writable by this user.\n"+
			"Update through whatever installed it instead:\n\n%s", filepath.Dir(exe), updateHint())
	}

	fmt.Printf("updating helikopter %s -> %s\n", version, latest)

	asset := assetName(latest)
	fmt.Printf("  downloading %s\n", asset)
	archive, err := download(releaseDownload + "/" + latest + "/" + asset)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", asset, err)
	}

	sums, err := download(releaseDownload + "/" + latest + "/checksums.txt")
	if err != nil {
		return fmt.Errorf("downloading checksums: %w", err)
	}
	if err := verifySHA256(archive, sums, asset); err != nil {
		return err
	}
	fmt.Println("  checksum ok")

	bin, err := extractBinary(archive, asset)
	if err != nil {
		return fmt.Errorf("reading %s: %w", asset, err)
	}

	if err := replaceExecutable(exe, bin, latest); err != nil {
		return err
	}

	fmt.Printf("  updated to %s\n", latest)
	return nil
}

// currentExecutable resolves symlinks, so updating a Homebrew-style link
// replaces the real file rather than clobbering the link with a binary.
func currentExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot find my own path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// writable reports whether we can create a file in dir. Checking by trying is
// the only answer that survives permissions, read-only mounts and Windows.
func writable(dir string) error {
	f, err := os.CreateTemp(dir, ".helikopter-write-check-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// assetName is the release archive for the platform we are running on, which
// is the platform that produced this binary.
func assetName(tag string) string {
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("helikopter_%s_%s_%s%s",
		strings.TrimPrefix(tag, "v"), runtime.GOOS, runtime.GOARCH, ext)
}

func download(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "helikopter/"+version)

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", url, resp.Status)
	}
	// Generous, but bounded: the archives are a few megabytes.
	return io.ReadAll(io.LimitReader(resp.Body, 128<<20))
}

// verifySHA256 checks data against the line for name in a checksums.txt.
// A missing line is a failure, not something to shrug at — otherwise a
// truncated checksums file would silently disable verification.
func verifySHA256(data, sums []byte, name string) error {
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == name {
			want = strings.ToLower(f[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum published for %s", name)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("checksum mismatch for %s:\n  expected %s\n  got      %s", name, want, got)
	}
	return nil
}

// extractBinary pulls the helikopter executable out of a release archive.
func extractBinary(archive []byte, assetName string) ([]byte, error) {
	want := "helikopter"
	if strings.HasSuffix(assetName, ".zip") {
		want = "helikopter.exe"
		return fromZip(archive, want)
	}
	return fromTarGz(archive, want)
}

func fromTarGz(archive []byte, want string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg || filepath.Base(h.Name) != want {
			continue
		}
		return io.ReadAll(io.LimitReader(tr, 128<<20))
	}
	return nil, fmt.Errorf("archive did not contain %s", want)
}

func fromZip(archive []byte, want string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) != want {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(io.LimitReader(rc, 128<<20))
	}
	return nil, fmt.Errorf("archive did not contain %s", want)
}

// replaceExecutable swaps the new binary in beside the old one.
//
// The new file is written into the same directory so the rename is on one
// filesystem and therefore atomic; a temp directory elsewhere would degrade to
// a copy that can fail halfway and leave a half-written command.
func replaceExecutable(exe string, bin []byte, tag string) error {
	dir := filepath.Dir(exe)

	mode := os.FileMode(0o755)
	if fi, err := os.Stat(exe); err == nil {
		mode = fi.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, ".helikopter-new-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename has taken it away

	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}

	// Prove it runs before trusting it. A truncated download that still
	// matched its checksum is unlikely, but a binary for the wrong platform or
	// one the kernel refuses is not, and finding out after the swap would
	// leave no working command at all.
	if err := verifyRuns(tmpName, tag); err != nil {
		return fmt.Errorf("the downloaded binary does not run, so nothing was changed: %w", err)
	}

	return swap(exe, tmpName)
}

func verifyRuns(path, tag string) error {
	cmd := exec.Command(path, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	if !strings.Contains(string(out), strings.TrimPrefix(tag, "v")) {
		return fmt.Errorf("reported %q, which is not %s", strings.TrimSpace(string(out)), tag)
	}
	return nil
}

// swap moves newPath onto exe.
func swap(exe, newPath string) error {
	if runtime.GOOS != "windows" {
		// Unix replaces the directory entry; this process keeps running from
		// the old inode until it exits.
		return os.Rename(newPath, exe)
	}

	// Windows will not let a running image be replaced, but it will let it be
	// renamed out of the way. If the second rename fails, put the original
	// back rather than leaving nothing behind.
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("moving the running binary aside: %w", err)
	}
	if err := os.Rename(newPath, exe); err != nil {
		if rbErr := os.Rename(old, exe); rbErr != nil {
			return fmt.Errorf("swap failed (%w) and the original could not be restored from %s: %v",
				err, old, rbErr)
		}
		return fmt.Errorf("installing the new binary: %w", err)
	}
	// Usually refused while the old image is still mapped; the next update
	// clears it, and it is harmless in the meantime.
	_ = os.Remove(old)
	return nil
}
