#!/usr/bin/env bash
set -euo pipefail

# This script runs on one dedicated platform runner. The runner owns the final
# QQ-login portion; this wrapper pins the official asset and records evidence.
: "${LUCKYLILLIA_PLATFORM:?Set linux-amd64, linux-arm64, windows-amd64 or darwin-arm64.}"
: "${LUCKYLILLIA_E2E_COMMAND:?Set the reviewed platform E2E command on the validation runner.}"

case "$LUCKYLILLIA_PLATFORM" in
  linux-amd64) asset="LLBot-CLI-linux-x64.zip" ;;
  linux-arm64) asset="LLBot-CLI-linux-arm64.zip" ;;
  windows-amd64) asset="LLBot-CLI-win-x64.zip" ;;
  darwin-arm64) asset="LLBot-CLI-macos-arm64.tar.xz" ;;
  *) echo "unsupported LuckyLillia platform: $LUCKYLILLIA_PLATFORM" >&2; exit 2 ;;
esac

mkdir -p artifacts
case "$LUCKYLILLIA_PLATFORM" in
  darwin-arm64) command -v shasum >/dev/null; command -v tar >/dev/null ;;
  windows-amd64) command -v powershell >/dev/null ;;
  *) command -v sha256sum >/dev/null; command -v unzip >/dev/null ;;
esac
command -v curl >/dev/null
command -v jq >/dev/null
metadata="$(curl --fail --silent --show-error https://api.github.com/repos/LLOneBot/LuckyLilliaBot/releases/latest)"
asset_url="$(jq -r --arg asset "$asset" '.assets[] | select(.name == $asset) | .browser_download_url' <<<"$metadata")"
digest="$(jq -r --arg asset "$asset" '.assets[] | select(.name == $asset) | .digest' <<<"$metadata")"
tag="$(jq -r '.tag_name' <<<"$metadata")"
test -n "$asset_url" && test "$asset_url" != null
test "${digest#sha256:}" != "$digest"

archive="$PWD/artifacts/$asset"
trap 'rm -f "$archive"' EXIT
curl --fail --location --silent --show-error "$asset_url" -o "$archive"
case "$LUCKYLILLIA_PLATFORM" in
  darwin-arm64) actual="$(shasum -a 256 "$archive" | awk '{print $1}')" ;;
  windows-amd64) actual="$(powershell -NoProfile -Command \"(Get-FileHash -Algorithm SHA256 -LiteralPath '$archive').Hash.ToLower()\")" ;;
  *) actual="$(sha256sum "$archive" | awk '{print $1}')" ;;
esac
test "$actual" = "${digest#sha256:}"
case "$asset" in
  *.zip)
    if [ "$LUCKYLILLIA_PLATFORM" = "windows-amd64" ]; then
      powershell -NoProfile -Command "\$target = Join-Path (Get-Location) 'artifacts/luckylillia-inspection'; Remove-Item -Recurse -Force -ErrorAction SilentlyContinue \$target; Expand-Archive -LiteralPath '$archive' -DestinationPath \$target -Force; if (-not (Get-ChildItem -Path \$target -Filter llbot.exe -Recurse -File | Select-Object -First 1)) { throw 'llbot.exe missing from official archive' }"
    else
      unzip -t "$archive" >/dev/null
    fi
    ;;
  *.tar.xz) tar -tJf "$archive" >/dev/null ;;
esac

export LUCKYLILLIA_RELEASE_TAG="$tag"
export LUCKYLILLIA_ARCHIVE="$archive"
"$LUCKYLILLIA_E2E_COMMAND"

# The reviewed command must prove that start.sh (or llbot.exe) stays attached
# to the launcher process group and can be stopped by that group. It records
# the observed IDs here; a port check alone is deliberately insufficient.
process_report="artifacts/luckylillia-process-model.json"
test -f "$process_report"
process_model="$(jq -r '.processModel' "$process_report")"
launcher_pid="$(jq -r '.launcherPid' "$process_report")"
process_group_id="$(jq -r '.processGroupID' "$process_report")"
websocket_required="$(jq -r '.websocketRequired' "$process_report")"
runtime_fingerprint="$(jq -r '.runtimeFingerprint' "$process_report")"
test "$process_model" = "foreground"
case "$launcher_pid:$process_group_id" in
  *[!0-9:]*|:*) echo "invalid LuckyLillia process model report" >&2; exit 1 ;;
esac
case "$websocket_required" in true|false) ;; *) echo "invalid LuckyLillia WebSocket report" >&2; exit 1 ;; esac
case "$runtime_fingerprint" in [0-9a-fA-F][0-9a-fA-F]*) ;; *) echo "missing LuckyLillia runtime fingerprint" >&2; exit 1 ;; esac
test "${#runtime_fingerprint}" -eq 64

jq -n --arg core "luckylillia" --arg tag "$tag" --arg platform "$LUCKYLILLIA_PLATFORM" --arg asset "$asset" --arg sha256 "$actual" --arg fingerprint "$runtime_fingerprint" --arg at "$(date -u +%FT%TZ)" --arg processModel "$process_model" --argjson websocketRequired "$websocket_required" \
  '{core: $core, tag: $tag, platform: $platform, asset: $asset, archiveSha256: $sha256, runtimeFingerprint: $fingerprint, validatedAt: $at, processModel: $processModel, websocketRequired: $websocketRequired, status: "passed"}' \
  > artifacts/luckylillia-validation.json
LUCKYLILLIA_VALIDATION_EVIDENCE="$(cat artifacts/luckylillia-validation.json)" python3 scripts/validate-runtime-evidence.py
