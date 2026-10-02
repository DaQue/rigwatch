#!/usr/bin/env bash
# Cut a release: verify, tag, and push the tag. Pushing a v* tag triggers
# .github/workflows/release.yml, which builds every platform with GoReleaser and
# publishes the GitHub release that install.sh downloads.
#
# Usage: scripts/release.sh [patch|minor|major|vX.Y.Z] [--dry-run] [--yes] [--no-watch]
#   patch (default)  v0.0.20 -> v0.0.21
#   --dry-run        run every check and show the plan, change nothing
#   --yes            skip the confirmation prompt
#   --no-watch       don't follow the Actions run afterwards

set -euo pipefail
cd "$(dirname "$0")/.."

BUMP=patch DRY=0 YES=0 WATCH=1
for arg in "$@"; do
  case "$arg" in
    patch|minor|major|v[0-9]*) BUMP=$arg ;;
    --dry-run) DRY=1 ;;
    --yes|-y) YES=1 ;;
    --no-watch) WATCH=0 ;;
    -h|--help) sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown argument: $arg (try --help)" >&2; exit 2 ;;
  esac
done

die() { echo "✗ $*" >&2; exit 1; }
ok()  { echo "✓ $*"; }

# --- preflight: refuse to release anything that isn't exactly what's on origin/master
[ -z "$(git status --porcelain)" ] || die "working tree is not clean; commit or stash first"
ok "working tree clean"

branch=$(git branch --show-current)
[ "$branch" = "master" ] || die "on '$branch'; releases are cut from master"
ok "on master"

git fetch --quiet --tags origin
local_sha=$(git rev-parse HEAD)
remote_sha=$(git rev-parse origin/master)
[ "$local_sha" = "$remote_sha" ] || die "master differs from origin/master; push or pull first"
ok "in sync with origin/master"

echo "… checking build, vet and tests"
go build ./... || die "build failed"
go vet ./... || die "go vet failed"
go test -count=1 ./... >/dev/null || die "tests failed (run: go test ./...)"
ok "build, vet, tests pass"

# --- version
last=$(git tag --list 'v[0-9]*' --sort=-v:refname | head -n1)
last=${last:-v0.0.0}
if [[ "$BUMP" == v* ]]; then
  next=$BUMP
  [[ "$next" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "'$next' is not vMAJOR.MINOR.PATCH"
else
  IFS=. read -r major minor patch <<<"${last#v}"
  case "$BUMP" in
    major) major=$((major + 1)); minor=0; patch=0 ;;
    minor) minor=$((minor + 1)); patch=0 ;;
    patch) patch=$((patch + 1)) ;;
  esac
  next="v$major.$minor.$patch"
fi
git rev-parse -q --verify "refs/tags/$next" >/dev/null && die "tag $next already exists"
[ "$(git rev-list "$last"..HEAD --count 2>/dev/null || echo 1)" != 0 ] || die "nothing new since $last"

echo
echo "Release $last → $next"
git log --no-merges --pretty='  • %s' "$last"..HEAD
echo

if [ "$DRY" = 1 ]; then
  echo "dry run: would tag $next at $(git rev-parse --short HEAD) and push it. Nothing changed."
  exit 0
fi

if [ "$YES" != 1 ]; then
  read -r -p "Tag and push $next? This publishes a release. [y/N] " reply
  [[ "$reply" =~ ^[Yy]$ ]] || { echo "aborted"; exit 1; }
fi

git tag -a "$next" -m "Release $next"
git push origin "$next"
ok "pushed $next"

repo=$(git remote get-url origin | sed -E 's#(git@github.com:|https://github.com/)##; s#\.git$##')
echo "Actions: https://github.com/$repo/actions"
echo "Release: https://github.com/$repo/releases/tag/$next"

if [ "$WATCH" = 1 ] && command -v gh >/dev/null 2>&1; then
  echo "… waiting for the release workflow"
  sleep 5
  run=$(gh run list --workflow=release.yml --limit 1 --json databaseId --jq '.[0].databaseId' 2>/dev/null || true)
  if [ -n "$run" ]; then
    gh run watch "$run" --exit-status && ok "release $next published" || die "release workflow failed: gh run view $run --log-failed"
  fi
fi
