#!/bin/sh
set -eu

die() { echo "Error: $*" >&2; exit 1; }
[ "$#" -eq 0 ] || die 'usage: sh install.sh'
case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) die 'unsupported operating system; expected macOS or Linux' ;;
esac
case "$(uname -m)" in
  arm64|aarch64) arch=arm64 ;;
  x86_64|amd64) arch=amd64 ;;
  *) die 'unsupported architecture; expected amd64 or arm64' ;;
esac
command -v curl >/dev/null 2>&1 || die 'curl is required'
command -v tar >/dev/null 2>&1 || die 'tar is required'
if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
  die 'sha256sum or shasum is required'
fi

install_dir=${FANQIE_DL_INSTALL_DIR:-"$HOME/.local/bin"}
[ ! -d "$install_dir/fanqie-dl" ] || die "$install_dir/fanqie-dl is a directory"
repo=https://github.com/moonrailgun/fanqie-dl
fetch() { curl -fsSL --connect-timeout 15 --max-time 120 --proto '=https' --proto-redir '=https' "$@"; }
release_url=$(fetch -o /dev/null -w '%{url_effective}' "$repo/releases/latest") || die 'cannot find latest release; check the network and whether a release has been published'
version=${release_url#"$repo/releases/tag/v"}
printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || die 'cannot determine latest release version; check whether a stable release has been published'
archive="fanqie-dl_${version}_${os}_${arch}.tar.gz"
base="$repo/releases/download/v$version"

tmp=$(mktemp -d)
staged=
trap 'rm -rf "$tmp"; if [ -n "$staged" ]; then rm -f "$staged"; fi' EXIT
trap 'exit 1' HUP INT TERM
echo "Downloading fanqie-dl $version ($os/$arch)..."
fetch -o "$tmp/$archive" "$base/$archive" || die 'failed to download release archive'
fetch -o "$tmp/checksums.txt" "$base/checksums.txt" || die 'failed to download checksums'
expected=$(awk -v name="$archive" '$2 == name {print $1}' "$tmp/checksums.txt")
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp/$archive" | awk '{print $1}')
else
  actual=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')
fi
[ -n "$expected" ] && [ "$actual" = "$expected" ] || die 'release checksum verification failed'
tar -xzf "$tmp/$archive" -C "$tmp" fanqie-dl || die 'failed to extract release archive'
[ -f "$tmp/fanqie-dl" ] && [ ! -L "$tmp/fanqie-dl" ] || die 'release archive does not contain a regular binary'

# Stage beside the destination so upgrades replace the old binary atomically.
mkdir -p "$install_dir"
staged=$(mktemp "$install_dir/.fanqie-dl.XXXXXX")
install -m 755 "$tmp/fanqie-dl" "$staged"
mv -f "$staged" "$install_dir/fanqie-dl"
staged=
echo "Installed fanqie-dl $version to $install_dir/fanqie-dl"
case ":$PATH:" in
  *":$install_dir:"*) echo 'Run: fanqie-dl --version' ;;
  *) printf 'Add %s to your PATH to run fanqie-dl from any directory.\n' "$install_dir" ;;
esac
