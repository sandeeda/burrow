#!/bin/sh
# Burrow installer for Linux and macOS.
#
#   curl -fsSL https://raw.githubusercontent.com/sandeeda/burrow/main/install.sh | sh
#
# Downloads the latest release for this OS/CPU from GitHub, checks its
# SHA-256 against the published checksums, and installs the `burrow` binary.
#
# Options (environment variables):
#   BURROW_VERSION=v0.1.0       install a specific version (default: latest)
#   BURROW_INSTALL_DIR=/path    where to put the binary
#                               (default: /usr/local/bin if writable, else ~/.local/bin)
#   BURROW_DOWNLOAD_URL=url     download from a mirror instead of GitHub Releases
set -eu

REPO="sandeeda/burrow"

say() { printf '%s\n' "$*"; }
fail() { printf 'burrow install: %s\n' "$*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || fail "needs '$1' - please install it and try again"; }
need uname
need tar
need mktemp

if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$1"; }
  fetch_to() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -qO- "$1"; }
  fetch_to() { wget -qO "$2" "$1"; }
else
  fail "needs curl or wget"
fi

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) fail "unsupported OS $(uname -s). On Windows, use: winget install sandeeda.Burrow" ;;
esac

case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) fail "unsupported CPU $(uname -m)" ;;
esac

version="${BURROW_VERSION:-}"
if [ -z "$version" ]; then
  version=$(fetch "https://api.github.com/repos/$REPO/releases/latest" |
    sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)
  [ -n "$version" ] || fail "couldn't find the latest release (is GitHub reachable?)"
fi
plain="${version#v}"

archive="burrow_${plain}_${os}_${arch}.tar.gz"
base="${BURROW_DOWNLOAD_URL:-https://github.com/$REPO/releases/download/$version}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

say "Downloading Burrow ${version} for ${os}/${arch}..."
fetch_to "$base/$archive" "$tmp/$archive" || fail "download failed: $base/$archive"
fetch_to "$base/checksums.txt" "$tmp/checksums.txt" || fail "couldn't download checksums"

say "Verifying checksum..."
want=$(grep " $archive\$" "$tmp/checksums.txt" | cut -d ' ' -f 1)
[ -n "$want" ] || fail "no checksum published for $archive"
if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$tmp/$archive" | cut -d ' ' -f 1)
elif command -v shasum >/dev/null 2>&1; then
  got=$(shasum -a 256 "$tmp/$archive" | cut -d ' ' -f 1)
else
  fail "needs sha256sum or shasum to verify the download"
fi
[ "$want" = "$got" ] || fail "checksum mismatch - the download may be corrupted or tampered with"

tar -xzf "$tmp/$archive" -C "$tmp" burrow

dir="${BURROW_INSTALL_DIR:-}"
if [ -z "$dir" ]; then
  if [ -w /usr/local/bin ]; then
    dir=/usr/local/bin
  else
    dir="$HOME/.local/bin"
  fi
fi
mkdir -p "$dir"
install -m 0755 "$tmp/burrow" "$dir/burrow" 2>/dev/null || {
  cp "$tmp/burrow" "$dir/burrow" && chmod 0755 "$dir/burrow"
}

# macOS: the binary isn't notarised yet, so clear the download quarantine.
if [ "$os" = darwin ] && command -v xattr >/dev/null 2>&1; then
  xattr -d com.apple.quarantine "$dir/burrow" 2>/dev/null || true
fi

say "Installed burrow ${version} to ${dir}/burrow"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) say "   Add it to your PATH:  export PATH=\"$dir:\$PATH\"" ;;
esac
say ""
say "Next: run   burrow ~/Documents   (or any folder you want on your phone)"
