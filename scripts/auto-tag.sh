#!/usr/bin/env bash
# Auto-tag releases from merged PRs.
#
# Finds the last tag, reads the squash-merged PR titles since then, and
# creates the next SemVer tag: feat -> minor, anything else -> patch,
# a breaking change (type! or BREAKING CHANGE) -> major. Weeks with no
# merged PRs tag nothing. The tag push triggers .github/workflows/release.yml.
#
# Usage:
#   bash scripts/auto-tag.sh [--dry-run]
#
# --dry-run prints the tag that would be created without creating it.
# With no tags at all, the first tag is v1.0.0.
#
# The tag must be pushed with a personal access token, not the built-in
# GITHUB_TOKEN: pushes made with GITHUB_TOKEN do not trigger other
# workflows, so the release workflow would never fire. In CI the checkout
# step runs with the RELEASE_PAT secret, which makes the local push use it.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# last_tag prints the most recent tag reachable from HEAD, or nothing.
last_tag() {
	git -C "$REPO_ROOT" describe --tags --abbrev=0 2>/dev/null || true
}

# pr_subjects <tag> prints the squash-merged PR titles since <tag>
# (one per line). Squash merges land as "<title> (#N)"; anything without
# that suffix is a direct push, not a merged PR, and is ignored.
# With an empty tag, all of history is in range.
# Title-only: --format=%s reads the squash subject, so a BREAKING CHANGE
# note that lives in the commit body is not seen.
pr_subjects() {
	local range="HEAD"
	if [[ -n "$1" ]]; then
		range="$1..HEAD"
	fi
	git -C "$REPO_ROOT" log "$range" --format=%s | grep -E '\(#[0-9]+\)$' || true
}

# next_version <base> <subjects...> prints the next version (without the
# leading v) for the given base, or nothing when there is nothing to tag.
# With an empty base (no tags yet) the first release is 1.0.0.
next_version() {
	local base="$1"
	shift
	if [[ $# -eq 0 ]]; then
		return 0
	fi
	if [[ -z "$base" ]]; then
		echo "1.0.0"
		return 0
	fi
	local major minor patch
	IFS=. read -r major minor patch <<<"$base"
	local bump="patch"
	local subject
	local breaking_re='^[a-z]+(\([^)]*\))?!:'
	local feat_re='^feat(\(|:)'
	for subject in "$@"; do
		if [[ "$subject" =~ $breaking_re ]] || [[ "$subject" == *"BREAKING CHANGE"* ]]; then
			bump="major"
			break
		elif [[ "$subject" =~ $feat_re ]]; then
			bump="minor"
		fi
	done
	case "$bump" in
		major) echo "$((major + 1)).0.0" ;;
		minor) echo "$major.$((minor + 1)).0" ;;
		*) echo "$major.$minor.$((patch + 1))" ;;
	esac
}

main() {
	local dry_run=0
	while [[ $# -gt 0 ]]; do
		case "$1" in
			--dry-run) dry_run=1; shift ;;
			*) echo "unknown argument: $1" >&2; exit 2 ;;
		esac
	done

	local tag base
	tag="$(last_tag)"
	base="${tag#v}"
	mapfile -t subjects < <(pr_subjects "$tag")
	echo "last tag: ${tag:-<none>}, merged PRs since: ${#subjects[@]}"
	for subject in "${subjects[@]}"; do
		echo "  $subject"
	done

	local version
	version="$(next_version "$base" "${subjects[@]}")"
	if [[ -z "$version" ]]; then
		echo "nothing to tag: no merged PRs since ${tag:-the start}"
		return 0
	fi
	local next_tag="v$version"
	if git -C "$REPO_ROOT" rev-parse -q --verify "refs/tags/$next_tag" >/dev/null; then
		echo "tag $next_tag already exists, nothing to do"
		return 0
	fi
	if [[ "$dry_run" -eq 1 ]]; then
		echo "dry run: would tag $next_tag"
		return 0
	fi
	git -C "$REPO_ROOT" tag -a "$next_tag" -m "Release $next_tag"
	git -C "$REPO_ROOT" push origin "$next_tag"
	echo "tagged and pushed $next_tag"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
	main "$@"
fi
