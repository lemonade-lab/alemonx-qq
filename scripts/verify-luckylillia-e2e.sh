#!/usr/bin/env bash
set -euo pipefail

# This script intentionally runs only on the dedicated, physical Linux ARM64
# validation host. It verifies the exact official asset before handing control
# to the host-maintained end-to-end procedure (QQ dependency, WebUI, login
# pending, OneBot ready, stop, update and rollback).
: "${LUCKYLILLIA_E2E_COMMAND:?Set the reviewed Linux ARM64 E2E command on the validation runner.}"

mkdir -p artifacts
metadata="$(curl --fail --silent --show-error https://api.github.com/repos/LLOneBot/LuckyLilliaBot/releases/latest)"
asset_url="$(jq -r '.assets[] | select(.name == "LLBot-CLI-linux-arm64.zip") | .browser_download_url' <<<"$metadata")"
digest="$(jq -r '.assets[] | select(.name == "LLBot-CLI-linux-arm64.zip") | .digest' <<<"$metadata")"
tag="$(jq -r '.tag_name' <<<"$metadata")"
test -n "$asset_url" && test "$asset_url" != null
test "${digest#sha256:}" != "$digest"

archive="$(mktemp -t luckylillia.XXXXXX.zip)"
trap 'rm -f "$archive"' EXIT
curl --fail --location --silent --show-error "$asset_url" -o "$archive"
actual="$(sha256sum "$archive" | awk '{print $1}')"
test "$actual" = "${digest#sha256:}"
unzip -t "$archive" >/dev/null

export LUCKYLILLIA_RELEASE_TAG="$tag"
export LUCKYLILLIA_ARCHIVE="$archive"
"$LUCKYLILLIA_E2E_COMMAND"

jq -n --arg tag "$tag" --arg sha256 "$actual" --arg at "$(date -u +%FT%TZ)" \
  '{tag: $tag, archiveSha256: $sha256, validatedAt: $at, status: "passed"}' \
  > artifacts/luckylillia-validation.json
