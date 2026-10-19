#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
# Keep these disposable repositories independent of the user's Git settings.
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=Test GIT_AUTHOR_EMAIL=test@example.invalid
export GIT_COMMITTER_NAME=Test GIT_COMMITTER_EMAIL=test@example.invalid

fail() { echo "FAIL: $*" >&2; exit 1; }
new_repo() {
  local name=$1
  git init -q --bare "$tmp/$name.git"
  git init -q -b main "$tmp/$name"
  cd "$tmp/$name"
  cp "$root/release.sh" .
  git add release.sh
  git commit -qm initial
  git remote add origin "$tmp/$name.git"
  git push -q -u origin main
}
expect_failure() {
  if sh release.sh patch >"$tmp/output" 2>&1; then
    fail "expected failure: $1"
  fi
  grep -Fq "$1" "$tmp/output" || { cat "$tmp/output"; fail "missing error: $1"; }
}
remote_tags() { git --git-dir="$tmp/$1.git" tag --list; }

new_repo first
if sh release.sh </dev/null >"$tmp/output" 2>&1; then fail 'missing selection published a release'; fi
grep -Fq '当前版本：尚无发布版本' "$tmp/output" || fail 'missing initial version before selection'
for option in '1) patch（修订版本） -> v0.1.0' '2) minor（次版本） -> v0.1.0' '3) major（主版本） -> v0.1.0'; do
  grep -Fq "$option" "$tmp/output" || fail "wrong initial preview: $option"
done
[[ -z $(git tag --list) && -z $(remote_tags first) ]] || fail 'EOF created a tag'
printf '1\n' | sh release.sh --dry-run >"$tmp/output"
grep -Fq 'Release: none -> v0.1.0' "$tmp/output" || fail 'wrong initial version'
[[ -z $(git tag --list) && -z $(remote_tags first) ]] || fail 'dry run created a tag'
for target in 1.0.0 v1.0.0; do
  printf '%s\n' "$target" | sh release.sh --dry-run >"$tmp/output" 2>&1 || { cat "$tmp/output"; fail "rejected interactive version: $target"; }
  grep -Fq 'Release: none -> v1.0.0' "$tmp/output" || fail 'wrong interactive target version'
  sh release.sh "$target" --dry-run >"$tmp/output" 2>&1
  grep -Fq 'Release: none -> v1.0.0' "$tmp/output" || fail 'wrong explicit target version'
done
[[ -z $(git tag --list) && -z $(remote_tags first) ]] || fail 'custom dry run created a tag'
printf 'patch\n' | sh release.sh >"$tmp/output"
[[ $(remote_tags first) == v0.1.0 ]] || fail 'first release was not pushed'
[[ $(git cat-file -t refs/tags/v0.1.0) == tag ]] || fail 'release tag is not annotated'
expect_failure 'already has a release tag'

for bump in patch minor major; do
  new_repo "$bump"
  git tag -a v1.2.3 -m previous
  git push -q origin refs/tags/v1.2.3
  git commit --allow-empty -qm next
  git push -q origin main
  # A user setting must not cause unrelated local tags to be published.
  git tag -a internal-note -m local-only
  git config push.followTags true
  case "$bump" in patch) next=v1.2.4;; minor) next=v1.3.0;; major) next=v2.0.0;; esac
  case "$bump" in patch) choice=1;; minor) choice=2;; major) choice=3;; esac
  if sh release.sh --dry-run </dev/null >"$tmp/output" 2>&1; then fail 'missing selection was accepted'; fi
  grep -Fq '当前版本：v1.2.3' "$tmp/output" || fail 'missing current version before selection'
  for option in '1) patch（修订版本） -> v1.2.4' '2) minor（次版本） -> v1.3.0' '3) major（主版本） -> v2.0.0'; do
    grep -Fq "$option" "$tmp/output" || fail "wrong version preview: $option"
  done
  printf '\ninvalid\n%s\n' "$choice" | sh release.sh --dry-run >"$tmp/output" 2>&1
  grep -Fq "Release: v1.2.3 -> $next" "$tmp/output" || fail "wrong interactive $bump selection"
  [[ $(remote_tags "$bump") == v1.2.3 ]] || fail 'interactive dry run published a tag'
  sh release.sh "$bump" >"$tmp/output"
  [[ $(remote_tags "$bump") == "$(printf 'v1.2.3\n%s' "$next")" ]] || fail "wrong $bump tags"
done

new_repo custom
printf '1.2.3\n' | sh release.sh >"$tmp/output" 2>&1
[[ $(remote_tags custom) == v1.2.3 ]] || fail 'custom first release was not pushed'
[[ $(git cat-file -t refs/tags/v1.2.3) == tag ]] || fail 'custom release tag is not annotated'
git commit --allow-empty -qm next
git push -q origin main
for target in 1.2 01.3.0 1.03.0 1.3.00 1.3.0-beta -1.3.0 v1.3.0/extra; do
  if sh release.sh "$target" --dry-run >"$tmp/output" 2>&1; then fail "accepted invalid version: $target"; fi
done
for target in 1.2.3 v1.2.3 1.2.2 1.1.9 0.9.9; do
  if sh release.sh "$target" --dry-run >"$tmp/output" 2>&1; then fail "accepted non-increasing version: $target"; fi
  grep -Fq 'must be greater than v1.2.3' "$tmp/output" || fail 'missing version order error'
done
if sh release.sh patch 1.3.0 --dry-run >"$tmp/output" 2>&1; then fail 'accepted multiple release targets'; fi
printf '1.3\n1.2.3\n1.2.2\n1.10.0\n' | sh release.sh --dry-run >"$tmp/output" 2>&1
grep -Fq 'Release: v1.2.3 -> v1.10.0' "$tmp/output" || fail 'invalid custom selection did not retry'
grep -Fq 'invalid version' "$tmp/output" || fail 'missing version format error'
grep -Fq 'must be greater than v1.2.3' "$tmp/output" || fail 'missing interactive version order error'
[[ $(git tag --list) == v1.2.3 && $(remote_tags custom) == v1.2.3 ]] || fail 'custom validation created a tag'
sh release.sh v1.10.0 >"$tmp/output" 2>&1
[[ $(remote_tags custom) == "$(printf 'v1.10.0\nv1.2.3')" ]] || fail 'explicit version was not pushed'

new_repo guards
touch untracked
expect_failure 'working tree must be clean'
rm untracked
git switch -qc other
expect_failure 'must be on main'
git switch -q main
git commit --allow-empty -qm unpushed
expect_failure 'main must match origin/main'
git push -q origin main
git reset -q --hard HEAD~1
expect_failure 'main must match origin/main'
git reset -q --hard origin/main
if sh release.sh invalid >"$tmp/output" 2>&1; then fail 'accepted invalid argument'; fi
[[ -z $(git tag --list) && -z $(remote_tags guards) ]] || fail 'preflight failure created a tag'

new_repo rejected
cat >"$tmp/rejected.git/hooks/pre-receive" <<'HOOK'
#!/bin/sh
echo 'reject tag for test' >&2
exit 1
HOOK
chmod +x "$tmp/rejected.git/hooks/pre-receive"
expect_failure 'Tag v0.1.0 was kept locally'
[[ $(git tag --list) == v0.1.0 && -z $(remote_tags rejected) ]] || fail 'push failure lost tag or published it'
grep -Fq 'git -c push.followTags=false push origin refs/tags/v0.1.0' "$tmp/output" || fail 'missing retry command'

echo 'release script tests passed'
