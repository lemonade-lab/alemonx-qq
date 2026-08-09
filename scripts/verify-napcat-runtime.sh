#!/usr/bin/env bash
set -euo pipefail

# Run this on a host where NapCat has already been installed. It intentionally
# does not install, start, stop, or log in to QQ: the check records the live
# runtime contract without changing the operator's session.
: "${ALX_QQ_RUNNER:?Set ALX_QQ_RUNNER to the built almonx-qq executor path.}"

run_action() {
  printf '{"protocol":"alx/v1","method":"run","action":"%s","params":{}}' "$1" | "$ALX_QQ_RUNNER"
}

mkdir -p artifacts
status_envelope="$(run_action napcat-status)"
status="$(jq -cer '.output | fromjson' <<<"$status_envelope")"
if [[ "$(jq -r '.installed' <<<"$status")" != "true" ]]; then
  echo "NapCat is not installed on this host; no runtime validation was recorded." >&2
  exit 2
fi
if [[ "$(jq -r '.running' <<<"$status")" != "true" ]]; then
  echo "NapCat is installed but not running; start it and rerun this validation." >&2
  exit 2
fi
if [[ "$(jq -r '.webUiReady' <<<"$status")" != "true" ]]; then
  echo "NapCat is running but its WebUI is not reachable." >&2
  exit 2
fi

qr_envelope="$(run_action napcat-qrcode)"
qr="$(jq -cer '.output | fromjson' <<<"$qr_envelope")"
if [[ "$(jq -r '.loginPending' <<<"$status")" == "true" && "$(jq -r '.available' <<<"$qr")" != "true" ]]; then
  echo "NapCat is waiting for login but no guarded QR image is available." >&2
  exit 2
fi

jq -n --arg at "$(date -u +%FT%TZ)" --argjson status "$status" --argjson qr "$qr" \
  '{validatedAt: $at, status: $status, qrCode: {available: $qr.available, updatedAt: $qr.updatedAt}}' \
  > artifacts/napcat-runtime-validation.json
echo "NapCat runtime validation recorded in artifacts/napcat-runtime-validation.json"
