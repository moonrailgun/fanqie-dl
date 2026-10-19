#!/bin/sh
set -eu

usage() { echo 'Usage: ./release.sh [patch|minor|major|VERSION] [--dry-run]'; }
die() { echo "Error: $*" >&2; exit 1; }
target=
dry_run=false
for arg in "$@"; do
  case "$arg" in
    patch|minor|major|[0-9]*|v[0-9]*) [ -z "$target" ] || die 'choose one version increment or target'; target=$arg ;;
    --dry-run) dry_run=true ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; die "unknown argument: $arg" ;;
  esac
done
cd -- "$(dirname -- "$0")"

[ "$(git branch --show-current)" = main ] || die 'must be on main'
[ -z "$(git status --porcelain)" ] || die 'working tree must be clean; commit changes first'
git fetch origin refs/heads/main:refs/remotes/origin/main --tags
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || die 'main must match origin/main; synchronize it first'

stable='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
if git tag --points-at HEAD | grep -Eq "$stable"; then
  die 'HEAD already has a release tag; retry its push or rerun the existing GitHub Actions job'
fi
latest=$(git tag --list 'v*' --sort=-version:refname | grep -E "$stable" | head -n 1)
if [ -z "$latest" ]; then
  next_patch=v0.1.0
  next_minor=v0.1.0
  next_major=v0.1.0
else
  git merge-base --is-ancestor "$latest" HEAD || die "latest release $latest is not an ancestor of HEAD"
  numbers=${latest#v}
  major=${numbers%%.*}
  numbers=${numbers#*.}
  minor=${numbers%%.*}
  patch=${numbers#*.}
  next_patch="v$major.$minor.$((patch + 1))"
  next_minor="v$major.$((minor + 1)).0"
  next_major="v$((major + 1)).0.0"
fi
select_version() {
  case "$1" in
    patch) next=$next_patch ;;
    minor) next=$next_minor ;;
    major) next=$next_major ;;
    *) next="v${1#v}" ;;
  esac
  if [ "$(printf '%s\n' "$next" | grep -E "$stable" | head -n 1)" != "$next" ]; then
    echo 'Error: invalid version; use X.Y.Z or vX.Y.Z (for example, 1.0.0)' >&2
    return 1
  fi
  if [ -n "$latest" ]; then
    highest=$(printf '%s\n' "${latest#v}" "${next#v}" | sort -t. -k1,1n -k2,2n -k3,3n | tail -n 1)
    if [ "$next" = "$latest" ] || [ "${next#v}" != "$highest" ]; then
      echo "Error: target version must be greater than $latest" >&2
      return 1
    fi
  fi
}
printf '当前版本：%s\n' "${latest:-尚无发布版本}"
if [ -z "$target" ]; then
  printf '请选择升级类型或直接输入目标版本：\n  1) patch（修订版本） -> %s\n  2) minor（次版本） -> %s\n  3) major（主版本） -> %s\n' "$next_patch" "$next_minor" "$next_major" >&2
  while :; do
    printf '输入编号、类型或目标版本（如 1.0.0）：' >&2
    IFS= read -r selection || die 'no version selected; release canceled'
    case "$selection" in
      1) selection='patch' ;;
      2) selection='minor' ;;
      3) selection='major' ;;
    esac
    select_version "$selection" && break
  done
else
  select_version "$target" || exit 1
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
