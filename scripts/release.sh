#!/bin/sh
set -eu

usage() { echo 'Usage: ./scripts/release.sh [patch|minor|major] [--dry-run]'; }
die() { echo "Error: $*" >&2; exit 1; }
bump=
dry_run=false
for arg in "$@"; do
  case "$arg" in
    patch|minor|major) [ -z "$bump" ] || die 'choose one version increment'; bump=$arg ;;
    --dry-run) dry_run=true ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; die "unknown argument: $arg" ;;
  esac
done
cd -- "$(dirname -- "$0")/.."

[ "$(git branch --show-current)" = main ] || die 'must be on main'
[ -z "$(git status --porcelain)" ] || die 'working tree must be clean; commit changes first'
git fetch origin refs/heads/main:refs/remotes/origin/main --tags
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || die 'main must match origin/main; synchronize it first'

stable='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
if git tag --points-at HEAD | grep -Eq "$stable"; then
  die 'HEAD already has a release tag; retry its push or rerun the existing GitHub Actions job'
fi
latest=$(git tag --list 'v*' --sort=-version:refname | grep -E "$stable" | head -n 1)
if [ -z "$bump" ]; then
  printf '请选择升级类型：\n  1) patch（修订版本）\n  2) minor（次版本）\n  3) major（主版本）\n' >&2
  while :; do
    printf '输入编号或类型：' >&2
    IFS= read -r selection || die 'no version increment selected; release canceled'
    case "$selection" in
      1|patch) bump='patch' ;;
      2|minor) bump='minor' ;;
      3|major) bump='major' ;;
      *) echo '请选择 1、2、3 或 patch、minor、major。' >&2; continue ;;
    esac
    break
  done
fi
if [ -z "$latest" ]; then
  next=v0.1.0
else
  git merge-base --is-ancestor "$latest" HEAD || die "latest release $latest is not an ancestor of HEAD"
  numbers=${latest#v}
  major=${numbers%%.*}
  numbers=${numbers#*.}
  minor=${numbers%%.*}
  patch=${numbers#*.}
  case "$bump" in
    patch) next="v$major.$minor.$((patch + 1))" ;;
    minor) next="v$major.$((minor + 1)).0" ;;
    major) next="v$((major + 1)).0.0" ;;
  esac
fi

echo "Release: ${latest:-none} -> $next"
if [ "$dry_run" = true ]; then
  echo "Dry run: would create and push $next; no release tag was created or pushed."
  exit 0
fi
git tag -a "$next" -m "Release $next"
if ! git -c push.followTags=false push origin "refs/tags/$next"; then
  echo "Tag $next was kept locally. Check the remote, then retry:" >&2
  echo "  git -c push.followTags=false push origin refs/tags/$next" >&2
  exit 1
fi
echo "Pushed $next. GitHub Actions will build and publish the release."
