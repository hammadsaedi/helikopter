#!/bin/sh
# helikopter installer
#
#   curl -fsSL https://raw.githubusercontent.com/hammadsaedi/helikopter/main/install.sh | sh
#
# Environment:
#   HELIKOPTER_VERSION   tag to install (default: latest release)
#   HELIKOPTER_BIN_DIR   install directory (default: ~/.local/bin, or /usr/local/bin)

set -eu

REPO="hammadsaedi/helikopter"
BIN="helikopter"

say()  { printf '%s\n' "$*"; }
info() { printf '  %s\n' "$*"; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

need() {
  command -v "$1" >/dev/null 2>&1 || die "this installer needs '$1' on PATH"
}

detect_platform() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  arch=$(uname -m)

  case "$os" in
    linux)  os=linux ;;
    darwin) os=darwin ;;
    freebsd) os=freebsd ;;
    msys*|mingw*|cygwin*)
      die "on Windows use PowerShell:
  irm https://raw.githubusercontent.com/$REPO/main/install.ps1 | iex" ;;
    *) die "unsupported OS: $os" ;;
  esac

  case "$arch" in
    x86_64|amd64)  arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    armv7l|armv7)  arch=arm ;;
    i386|i686)     arch=386 ;;
    *) die "unsupported architecture: $arch" ;;
  esac

  PLATFORM="${os}_${arch}"
}

latest_version() {
  # The redirect on /releases/latest carries the tag, which avoids both the
  # API rate limit and a dependency on a JSON parser.
  url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
        "https://github.com/$REPO/releases/latest" 2>/dev/null) || return 1
  printf '%s\n' "${url##*/}"
}

choose_bin_dir() {
  if [ -n "${HELIKOPTER_BIN_DIR:-}" ]; then
    printf '%s\n' "$HELIKOPTER_BIN_DIR"; return
  fi
  if [ -d "$HOME/.local/bin" ] || mkdir -p "$HOME/.local/bin" 2>/dev/null; then
    printf '%s\n' "$HOME/.local/bin"; return
  fi
  printf '%s\n' "/usr/local/bin"
}

main() {
  need uname
  need curl
  need tar

  detect_platform

  version="${HELIKOPTER_VERSION:-}"
  if [ -z "$version" ]; then
    version=$(latest_version) || die "could not determine the latest version"
  fi
  [ -n "$version" ] || die "could not determine the latest version"

  say ""
  say "  installing $BIN $version for $PLATFORM"

  tmp=$(mktemp -d 2>/dev/null || mktemp -d -t helikopter)
  trap 'rm -rf "$tmp"' EXIT INT TERM

  base="https://github.com/$REPO/releases/download/$version"
  tarball="${BIN}_${version#v}_${PLATFORM}.tar.gz"

  info "downloading $tarball"
  curl -fsSL "$base/$tarball" -o "$tmp/$tarball" \
    || die "no release asset for $PLATFORM at $version"

  # Checksums are published alongside the archives; verify when we have a tool.
  if curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" 2>/dev/null; then
    if command -v sha256sum >/dev/null 2>&1; then
      ( cd "$tmp" && grep " $tarball\$" checksums.txt | sha256sum -c - >/dev/null 2>&1 ) \
        && info "checksum ok" || die "checksum verification failed"
    elif command -v shasum >/dev/null 2>&1; then
      ( cd "$tmp" && grep " $tarball\$" checksums.txt | shasum -a 256 -c - >/dev/null 2>&1 ) \
        && info "checksum ok" || die "checksum verification failed"
    else
      info "checksum skipped (no sha256sum or shasum)"
    fi
  else
    info "checksum skipped (checksums.txt not published)"
  fi

  tar -xzf "$tmp/$tarball" -C "$tmp"
  [ -f "$tmp/$BIN" ] || die "archive did not contain $BIN"
  chmod +x "$tmp/$BIN"

  dir=$(choose_bin_dir)
  mkdir -p "$dir" 2>/dev/null || true

  if [ -w "$dir" ]; then
    mv "$tmp/$BIN" "$dir/$BIN"
  elif command -v sudo >/dev/null 2>&1; then
    info "$dir needs elevation"
    sudo mv "$tmp/$BIN" "$dir/$BIN"
  else
    die "cannot write to $dir; set HELIKOPTER_BIN_DIR to somewhere you own"
  fi

  info "installed to $dir/$BIN"

  case ":$PATH:" in
    *":$dir:"*) ;;
    *)
      say ""
      say "  $dir is not on your PATH. Add it with:"
      say ""
      say "    echo 'export PATH=\"$dir:\$PATH\"' >> ~/.profile"
      ;;
  esac

  say ""
  say "  done. take off with:"
  say ""
  say "    helikopter"
  say ""
}

main "$@"
