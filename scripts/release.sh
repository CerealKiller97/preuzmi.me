#!/usr/bin/env bash
#
# release.sh — cut a SemVer release.
#
# Bumps ./version (the value embedded into the binary via go:embed), keeps
# package.json in sync, commits the change, and creates an annotated vX.Y.Z
# tag. Pushing that tag triggers .github/workflows/release.yml, which builds
# and publishes the multi-arch Docker image.
#
# Usage:
#   scripts/release.sh patch          # 1.0.0 -> 1.0.1
#   scripts/release.sh minor          # 1.0.0 -> 1.1.0
#   scripts/release.sh major          # 1.0.0 -> 2.0.0
#   scripts/release.sh 2.3.1          # set an explicit version
#   scripts/release.sh patch --push   # also push the commit and tag to origin
#
# Flags:
#   --push        push the current branch and the new tag to origin
#   --allow-dirty skip the clean-working-tree guard
#   -h, --help    show this help
#
set -euo pipefail

die() {
	echo "error: $*" >&2
	exit 1
}

usage() {
	sed -n '2,26p' "$0" | sed 's/^# \{0,1\}//'
	exit "${1:-0}"
}

# --- parse args ---------------------------------------------------------------
BUMP=""
PUSH=false
ALLOW_DIRTY=false

for arg in "$@"; do
	case "$arg" in
	-h | --help) usage 0 ;;
	--push) PUSH=true ;;
	--allow-dirty) ALLOW_DIRTY=true ;;
	-*) die "unknown flag: $arg (see --help)" ;;
	*)
		[[ -n "$BUMP" ]] && die "unexpected extra argument: $arg"
		BUMP="$arg"
		;;
	esac
done

[[ -n "$BUMP" ]] || usage 1

# Run from the repo root so ./version and ./package.json resolve regardless of
# where the script is invoked from.
ROOT="$(git rev-parse --show-toplevel)" || die "not inside a git repository"
cd "$ROOT"

VERSION_FILE="version"
[[ -f "$VERSION_FILE" ]] || die "missing $VERSION_FILE at repo root"

SEMVER_RE='^[0-9]+\.[0-9]+\.[0-9]+$'

# --- read current version -----------------------------------------------------
current="$(tr -d '[:space:]' <"$VERSION_FILE")"
[[ "$current" =~ $SEMVER_RE ]] || die "current version '$current' in ./$VERSION_FILE is not X.Y.Z"

IFS=. read -r major minor patch <<<"$current"

# --- compute the new version --------------------------------------------------
case "$BUMP" in
major) new="$((major + 1)).0.0" ;;
minor) new="${major}.$((minor + 1)).0" ;;
patch) new="${major}.${minor}.$((patch + 1))" ;;
*)
	new="${BUMP#v}" # tolerate a leading v
	[[ "$new" =~ $SEMVER_RE ]] || die "'$BUMP' is not a bump keyword (major|minor|patch) or an X.Y.Z version"
	;;
esac

tag="v${new}"

# --- safety guards ------------------------------------------------------------
if ! $ALLOW_DIRTY && [[ -n "$(git status --porcelain)" ]]; then
	die "working tree is dirty; commit or stash first (or pass --allow-dirty)"
fi

if git rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
	die "tag ${tag} already exists"
fi

echo "Releasing ${current} -> ${new}  (tag ${tag})"

# --- write the version --------------------------------------------------------
# No trailing newline: ./version is embedded verbatim and used as-is.
printf '%s' "$new" >"$VERSION_FILE"

# Keep package.json's version field in step (best-effort; only the first
# "version" key). awk keeps this portable across BSD (macOS) and GNU — the
# sed "0,/re/" address form is GNU-only and no-ops silently on BSD sed.
if [[ -f package.json ]]; then
	tmp="$(mktemp)"
	awk -v v="$new" '
		!done && /"version"[[:space:]]*:/ {
			sub(/"version"[[:space:]]*:[[:space:]]*"[^"]*"/, "\"version\": \"" v "\"")
			done = 1
		}
		{ print }
	' package.json >"$tmp" && mv "$tmp" package.json
fi

# --- commit and tag -----------------------------------------------------------
git add "$VERSION_FILE"
[[ -f package.json ]] && git add package.json

git commit -m "chore(release): ${tag}"
git tag -a "${tag}" -m "Release ${tag}"

echo "Committed and tagged ${tag}."

# --- push (optional) ----------------------------------------------------------
if $PUSH; then
	branch="$(git rev-parse --abbrev-ref HEAD)"
	echo "Pushing ${branch} and ${tag} to origin..."
	git push origin "${branch}"
	git push origin "${tag}"
	echo "Pushed. The release workflow will build and publish the image."
else
	echo
	echo "Next: push to trigger the release workflow:"
	echo "  git push origin $(git rev-parse --abbrev-ref HEAD) && git push origin ${tag}"
fi
