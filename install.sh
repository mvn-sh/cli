#!/bin/sh
set -eu

repo="mvn-sh/cli"
install_dir="${MVNSH_INSTALL_DIR:-$HOME/.local/bin}"
version="${MVNSH_VERSION:-latest}"

fail() {
  printf 'mvnsh installer: %s\n' "$1" >&2
  exit 1
}

command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v tar >/dev/null 2>&1 || fail "tar is required"

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) fail "unsupported operating system: $(uname -s)" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) fail "unsupported architecture: $(uname -m)" ;;
esac

if [ "$version" = latest ]; then
  version=$(curl -fsSL "https://api.github.com/repos/$repo/releases/latest" |
    awk -F '"' '/"tag_name"/ && !found { print $4; found=1 }')
  [ -n "$version" ] || fail "could not determine the latest version"
fi

case "$version" in v*) ;; *) version="v$version" ;; esac
release=${version#v}
archive="mvnsh_${release}_${os}_${arch}.tar.gz"
base_url="https://github.com/$repo/releases/download/$version"
tmp=$(mktemp -d 2>/dev/null || mktemp -d -t mvnsh)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

printf 'Downloading mvnsh %s for %s/%s...\n' "$version" "$os" "$arch"
curl -fsSL "$base_url/$archive" -o "$tmp/$archive" || fail "download failed"
curl -fsSL "$base_url/checksums.txt" -o "$tmp/checksums.txt" || fail "checksum download failed"
expected=$(awk -v file="$archive" '$2 == file { print $1 }' "$tmp/checksums.txt")
[ -n "$expected" ] || fail "release checksum not found"

if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp/$archive" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')
else
  fail "sha256sum or shasum is required"
fi
[ "$actual" = "$expected" ] || fail "checksum verification failed"

tar -xzf "$tmp/$archive" -C "$tmp" mvnsh
mkdir -p "$install_dir"
install -m 0755 "$tmp/mvnsh" "$install_dir/mvnsh"

printf 'Installed mvnsh to %s/mvnsh\n' "$install_dir"
case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) printf 'Add %s to your PATH to run mvnsh.\n' "$install_dir" ;;
esac
