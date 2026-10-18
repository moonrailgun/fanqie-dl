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
  mkdir scripts
  cp "$root/scripts/release.sh" scripts/
  git add scripts
  git commit -qm initial
  git remote add origin "$tmp/$name.git"
  git push -q -u origin main
}
expect_failure() {
  if sh scripts/release.sh patch >"$tmp/output" 2>&1; then
    fail "expected failure: $1"
  fi
  grep -Fq "$1" "$tmp/output" || { cat "$tmp/output"; fail "missing error: $1"; }
}
remote_tags() { git --git-dir="$tmp/$1.git" tag --list; }

new_repo first
if sh scripts/release.sh </dev/null >"$tmp/output" 2>&1; then fail 'missing selection published a release'; fi
[[ -z $(git tag --list) && -z $(remote_tags first) ]] || fail 'EOF created a tag'
printf '1\n' | sh scripts/release.sh --dry-run >"$tmp/output"
grep -Fq v0.1.0 "$tmp/output" || fail 'wrong initial version'
[[ -z $(git tag --list) && -z $(remote_tags first) ]] || fail 'dry run created a tag'
printf 'patch\n' | sh scripts/release.sh >"$tmp/output"
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
  printf '\ninvalid\n%s\n' "$choice" | sh scripts/release.sh --dry-run >"$tmp/output" 2>&1
  grep -Fq "Release: v1.2.3 -> $next" "$tmp/output" || fail "wrong interactive $bump selection"
  [[ $(remote_tags "$bump") == v1.2.3 ]] || fail 'interactive dry run published a tag'
  sh scripts/release.sh "$bump" >"$tmp/output"
  [[ $(remote_tags "$bump") == "$(printf 'v1.2.3\n%s' "$next")" ]] || fail "wrong $bump tags"
done

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
if sh scripts/release.sh invalid >"$tmp/output" 2>&1; then fail 'accepted invalid argument'; fi
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
