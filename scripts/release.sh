#!/bin/sh
# Cut a release.
#
#   make release VERSION=v1.1.2
#
# Everything after the tag is automatic: the workflow builds ten targets,
# publishes them with checksums, and commits the refreshed package manifests.
# This script exists for what comes before it, which is the part that is easy
# to get quietly wrong — releasing from the wrong branch, with uncommitted
# work, or from a main that is behind the remote.

set -eu

say()  { printf '  %s\n' "$*"; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

version="${VERSION:-}"
[ -n "$version" ] || die "usage: make release VERSION=v1.2.3"

case "$version" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  [0-9]*.[0-9]*.[0-9]*)  die "prefix the tag with v: VERSION=v$version" ;;
  *) die "expected a semver tag like v1.2.3, got '$version'" ;;
esac

branch=$(git rev-parse --abbrev-ref HEAD)
[ "$branch" = "main" ] || die "releases are cut from main, not '$branch'"

# Checked before the tree, because "you have already released this" is more
# use than "go and stash" when both are true.
git rev-parse "$version" >/dev/null 2>&1 &&
  die "$version already exists — tags are not reusable once pushed, so cut the next patch instead"

[ -z "$(git status --porcelain)" ] || die "working tree is not clean; commit or stash first"

say "fetching"
git fetch --quiet origin

local_head=$(git rev-parse HEAD)
remote_head=$(git rev-parse origin/main)
[ "$local_head" = "$remote_head" ] ||
  die "main and origin/main differ; pull or push before releasing"

previous=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
if [ -n "$previous" ]; then
  say "changes since $previous:"
  git log --no-merges --format='    %s' "$previous..HEAD" | sed 's/^    /      /'
  echo
  # Semver, briefly: anything users can newly type or press is a minor.
  if git log --no-merges --format='%s' "$previous..HEAD" | grep -qiE '^(add|allow|let|support)'; then
    say "note: this looks like it adds functionality, which is a minor bump"
    say "      (v1.1.1 -> v1.2.0), not a patch. Patch is for fixes only."
    echo
  fi
fi

say "running tests"
go test ./... >/dev/null || die "tests failed; not releasing"
gofmt -l . | grep -q . && die "gofmt is unhappy; not releasing"
go vet ./... >/dev/null 2>&1 || die "go vet failed; not releasing"

say "tagging $version"
git tag -a "$version" -m "helikopter $version"

say "pushing"
git push --quiet origin "$version"

repo=$(git config --get remote.origin.url | sed -e 's#.*github.com[:/]##' -e 's#\.git$##')
echo
say "released. the workflow is building now:"
say "  https://github.com/$repo/actions"
say "  https://github.com/$repo/releases/tag/$version"
echo
say "when it finishes, check it installs:"
say "  curl -fsSL https://${repo%%/*}.github.io/${repo#*/}/install.sh | sh"
