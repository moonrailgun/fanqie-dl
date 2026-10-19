#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/tools" "$tmp/assets" "$tmp/payload"
export FANQIE_DL_INSTALL_DIR="$tmp/install path"
export INSTALL_TEST_ASSETS="$tmp/assets" INSTALL_TEST_LOG="$tmp/requests"
export PATH="$tmp/tools:$PATH"

fail() { echo "FAIL: $*" >&2; exit 1; }
cat >"$tmp/tools/uname" <<'SH'
#!/bin/sh
case "$1" in -s) echo "$INSTALL_TEST_OS";; -m) echo "$INSTALL_TEST_ARCH";; esac
SH
cat >"$tmp/tools/curl" <<'SH'
#!/bin/sh
set -eu
output=
while [ "$#" -gt 0 ]; do
  case "$1" in -o) output=$2; shift 2;; *) url=$1; shift;; esac
done
echo "$url" >>"$INSTALL_TEST_LOG"
[ "${INSTALL_TEST_NETWORK_FAIL:-false}" = false ] || exit 22
case "$url" in
  https://github.com/moonrailgun/fanqie-dl/releases/latest)
    printf 'https://github.com/moonrailgun/fanqie-dl/releases/tag/%s' "${INSTALL_TEST_TAG:-v1.2.3}" ;;
  https://github.com/moonrailgun/fanqie-dl/releases/download/v1.2.3/*)
    cp "$INSTALL_TEST_ASSETS/${url##*/}" "$output" ;;
  *) exit 22 ;;
esac
SH
chmod +x "$tmp/tools/uname" "$tmp/tools/curl"
printf '#!/bin/sh\necho "fanqie-dl 1.2.3"\n' >"$tmp/payload/fanqie-dl"
chmod +x "$tmp/payload/fanqie-dl"
for os in darwin linux; do
  for arch in amd64 arm64; do
    tar -czf "$tmp/assets/fanqie-dl_1.2.3_${os}_${arch}.tar.gz" -C "$tmp/payload" fanqie-dl
  done
done
(cd "$tmp/assets" && shasum -a 256 ./*.tar.gz | sed 's|  ./|  |' >checksums.txt)

for target in 'Darwin arm64 darwin_arm64' 'Darwin x86_64 darwin_amd64' 'Linux aarch64 linux_arm64' 'Linux x86_64 linux_amd64'; do
  read -r INSTALL_TEST_OS INSTALL_TEST_ARCH asset <<<"$target"
  export INSTALL_TEST_OS INSTALL_TEST_ARCH
  sh "$root/scripts/install.sh" >"$tmp/output" 2>&1 || { cat "$tmp/output"; fail "$target installation"; }
  [[ -x "$FANQIE_DL_INSTALL_DIR/fanqie-dl" ]] || fail 'installed binary is not executable'
  [[ $("$FANQIE_DL_INSTALL_DIR/fanqie-dl" --version) == 'fanqie-dl 1.2.3' ]] || fail 'wrong installed binary'
  grep -Fq "fanqie-dl_1.2.3_${asset}.tar.gz" "$INSTALL_TEST_LOG" || fail 'wrong asset selected'
done

expect_failure() {
  if sh "$root/scripts/install.sh" >"$tmp/output" 2>&1; then fail "$1 succeeded"; fi
  grep -Fq "$1" "$tmp/output" || { cat "$tmp/output"; fail "missing error: $1"; }
  cmp "$FANQIE_DL_INSTALL_DIR/fanqie-dl" "$tmp/payload/fanqie-dl" || fail 'old binary was changed'
}
export INSTALL_TEST_NETWORK_FAIL=true
expect_failure 'latest release'
unset INSTALL_TEST_NETWORK_FAIL
export INSTALL_TEST_TAG=v1.2.3/../../bad
expect_failure 'release version'
unset INSTALL_TEST_TAG
cp "$tmp/assets/checksums.txt" "$tmp/checksums.original"
printf 'invalid checksums\n' >"$tmp/assets/checksums.txt"
expect_failure 'checksum'
cp "$tmp/checksums.original" "$tmp/assets/checksums.txt"
printf 'corrupt download' >"$tmp/assets/fanqie-dl_1.2.3_linux_amd64.tar.gz"
expect_failure 'checksum'
(cd "$tmp/assets" && shasum -a 256 ./*.tar.gz | sed 's|  ./|  |' >checksums.txt)
expect_failure 'extract release archive'
mkdir "$tmp/symlink"
ln -s "$tmp/payload/fanqie-dl" "$tmp/symlink/fanqie-dl"
tar -czf "$tmp/assets/fanqie-dl_1.2.3_linux_amd64.tar.gz" -C "$tmp/symlink" fanqie-dl
(cd "$tmp/assets" && shasum -a 256 ./*.tar.gz | sed 's|  ./|  |' >checksums.txt)
expect_failure 'regular binary'
rm "$tmp/assets/fanqie-dl_1.2.3_linux_amd64.tar.gz"
expect_failure 'download'
export INSTALL_TEST_ARCH=riscv64
expect_failure 'architecture'
export INSTALL_TEST_OS=Windows_NT
expect_failure 'operating system'

echo 'install script tests passed'
