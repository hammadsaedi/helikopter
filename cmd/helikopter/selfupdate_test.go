package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func makeTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func makeZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	zw.Close()
	return buf.Bytes()
}

func TestAssetNameMatchesThisPlatform(t *testing.T) {
	got := assetName("v1.2.3")
	if !strings.Contains(got, "1.2.3") || strings.Contains(got, "v1.2.3") {
		t.Errorf("asset name should carry the bare version: %s", got)
	}
	if !strings.Contains(got, runtime.GOOS) || !strings.Contains(got, runtime.GOARCH) {
		t.Errorf("asset name should name this platform: %s", got)
	}
	wantExt := ".tar.gz"
	if runtime.GOOS == "windows" {
		wantExt = ".zip"
	}
	if !strings.HasSuffix(got, wantExt) {
		t.Errorf("asset name %s should end in %s", got, wantExt)
	}
}

func TestVerifySHA256(t *testing.T) {
	data := []byte("a helicopter")
	name := "helikopter_1.0.0_linux_amd64.tar.gz"
	sums := fmt.Sprintf("%s  other.tar.gz\n%s  %s\n", sha([]byte("x")), sha(data), name)

	if err := verifySHA256(data, []byte(sums), name); err != nil {
		t.Errorf("matching checksum rejected: %v", err)
	}
	if err := verifySHA256([]byte("tampered"), []byte(sums), name); err == nil {
		t.Error("a mismatched checksum must fail")
	}
	// A missing line must fail rather than silently skip verification: a
	// truncated checksums file would otherwise disable the check entirely.
	if err := verifySHA256(data, []byte(sums), "helikopter_9.9.9_linux_amd64.tar.gz"); err == nil {
		t.Error("a missing checksum line must fail")
	}
	if err := verifySHA256(data, []byte(""), name); err == nil {
		t.Error("an empty checksums file must fail")
	}
}

func TestExtractBinary(t *testing.T) {
	want := []byte("\x7fELF pretend binary")

	tgz := makeTarGz(t, map[string][]byte{
		"./README.md":  []byte("docs"),
		"./LICENSE":    []byte("mit"),
		"./helikopter": want,
	})
	got, err := extractBinary(tgz, "helikopter_1.0.0_linux_amd64.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("tar.gz: got %q", got)
	}

	z := makeZip(t, map[string][]byte{
		"README.md":      []byte("docs"),
		"helikopter.exe": want,
	})
	got, err = extractBinary(z, "helikopter_1.0.0_windows_amd64.zip")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("zip: got %q", got)
	}
}

func TestExtractBinaryRejectsArchivesWithoutIt(t *testing.T) {
	tgz := makeTarGz(t, map[string][]byte{"./README.md": []byte("docs")})
	if _, err := extractBinary(tgz, "helikopter_1.0.0_linux_amd64.tar.gz"); err == nil {
		t.Error("an archive with no binary must fail")
	}
	if _, err := extractBinary([]byte("not an archive"), "x.tar.gz"); err == nil {
		t.Error("garbage must fail")
	}
	if _, err := extractBinary([]byte("not a zip"), "x.zip"); err == nil {
		t.Error("garbage zip must fail")
	}
}

func TestWritable(t *testing.T) {
	dir := t.TempDir()
	if err := writable(dir); err != nil {
		t.Errorf("a temp dir should be writable: %v", err)
	}
	if err := writable(filepath.Join(dir, "does-not-exist")); err == nil {
		t.Error("a missing directory should not report writable")
	}
	// The check must not leave anything behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("the writability check left files behind: %v", entries)
	}
}

// The swap is the dangerous part: it must either install the new binary or
// leave the old one working.
func TestReplaceExecutableSwapsAndKeepsMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stand-in binaries are shell scripts")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "helikopter")

	old := "#!/bin/sh\necho 'helikopter v1.0.0'\n"
	if err := os.WriteFile(exe, []byte(old), 0o755); err != nil {
		t.Fatal(err)
	}

	newBin := []byte("#!/bin/sh\necho 'helikopter v1.1.0 (abc, built now)'\n")
	if err := replaceExecutable(exe, newBin, "v1.1.0"); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newBin) {
		t.Errorf("binary was not replaced: %q", got)
	}
	fi, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want 0755", fi.Mode().Perm())
	}
	if leftovers := tempLeftovers(t, dir); len(leftovers) > 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}

// A binary that cannot run, or reports the wrong version, must not be
// installed — otherwise a bad release leaves no working command.
func TestReplaceExecutableRefusesABinaryThatWillNotRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stand-in binaries are shell scripts")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "helikopter")
	old := []byte("#!/bin/sh\necho 'helikopter v1.0.0'\n")
	if err := os.WriteFile(exe, old, 0o755); err != nil {
		t.Fatal(err)
	}

	for name, bin := range map[string][]byte{
		"exits non-zero":      []byte("#!/bin/sh\nexit 3\n"),
		"reports the old one": []byte("#!/bin/sh\necho 'helikopter v1.0.0'\n"),
		"prints nothing":      []byte("#!/bin/sh\n:\n"),
	} {
		if err := replaceExecutable(exe, bin, "v1.1.0"); err == nil {
			t.Errorf("%s: expected refusal", name)
		}
		got, err := os.ReadFile(exe)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, old) {
			t.Fatalf("%s: the working binary was replaced anyway", name)
		}
		if leftovers := tempLeftovers(t, dir); len(leftovers) > 0 {
			t.Errorf("%s: temp files left behind: %v", name, leftovers)
		}
	}
}

func tempLeftovers(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".helikopter-") {
			out = append(out, e.Name())
		}
	}
	return out
}
