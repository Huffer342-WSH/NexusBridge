#!/usr/bin/env bash
set -euo pipefail

tag="${1:-${TAG:-${GITHUB_REF_NAME:-}}}"
asset_root="${2:-release-assets}"

if [[ -z "$tag" ]]; then
  echo "Release tag is required. Pass it as an argument or set TAG." >&2
  exit 2
fi

if [[ "$tag" != v* ]]; then
  echo "Release tag must start with v: $tag" >&2
  exit 2
fi

if [[ -z "${GH_TOKEN:-}" ]]; then
  echo "GH_TOKEN is required to create a GitHub Release." >&2
  exit 2
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "GitHub CLI is required: gh" >&2
  exit 2
fi

shopt -s nullglob
assets=("$asset_root"/*/*)
if [[ "${#assets[@]}" -eq 0 ]]; then
  echo "No release assets found under $asset_root" >&2
  exit 1
fi

gh release create "$tag" "${assets[@]}" --verify-tag --generate-notes --title "NexusBridge $tag"
