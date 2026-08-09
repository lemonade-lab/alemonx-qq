#!/usr/bin/env bash
set -euo pipefail

# This wrapper deliberately does not invent a NapCat lifecycle. The dedicated
# host owns QQ and supplies a reviewed command which builds a runner with the
# candidate evidence below, performs the lifecycle, then writes the report.
# A Release may embed its evidence only after this wrapper has checked both
# that report and the platform-specific immutable source identity.
: "${NAPCAT_PLATFORM:?Set NAPCAT_PLATFORM (windows-amd64, linux-amd64 or linux-arm64).}"
: "${NAPCAT_E2E_COMMAND:?Set NAPCAT_E2E_COMMAND to the approved host lifecycle command.}"

mkdir -p artifacts
rm -f artifacts/napcat-e2e-report.json artifacts/napcat-validation.json artifacts/napcat-candidate-evidence.json

case "$NAPCAT_PLATFORM" in
  windows-amd64)
    command -v curl >/dev/null
    command -v jq >/dev/null
    command -v powershell >/dev/null
    metadata="$(curl --fail --silent --show-error https://api.github.com/repos/NapNeko/NapCatQQ/releases/latest)"
    tag="$(jq -r '.tag_name' <<<"$metadata")"
    asset="NapCat.Shell.Windows.OneKey.zip"
    asset_url="$(jq -r --arg asset "$asset" '.assets[] | select(.name == $asset) | .browser_download_url' <<<"$metadata")"
    digest="$(jq -r --arg asset "$asset" '.assets[] | select(.name == $asset) | .digest' <<<"$metadata")"
    test -n "$tag" && test "$tag" != null
    test -n "$asset_url" && test "$asset_url" != null
    test "${digest#sha256:}" != "$digest"
    archive="$PWD/artifacts/$asset"
    trap 'rm -f "$archive"' EXIT
    curl --fail --location --silent --show-error "$asset_url" -o "$archive"
    actual="$(powershell -NoProfile -Command "(Get-FileHash -Algorithm SHA256 -LiteralPath '$archive').Hash.ToLower()")"
    test "$actual" = "${digest#sha256:}"
    powershell -NoProfile -Command "\$target = Join-Path (Get-Location) 'artifacts/napcat-inspection'; Remove-Item -Recurse -Force -ErrorAction SilentlyContinue \$target; Expand-Archive -LiteralPath '$archive' -DestinationPath \$target -Force; if (-not (Get-ChildItem -Path \$target -Include launcher.bat,launcher.exe -Recurse -File | Select-Object -First 1)) { throw 'NapCat OneKey launcher missing from official archive' }"
    jq -n --arg core napcat --arg platform "$NAPCAT_PLATFORM" --arg tag "$tag" --arg asset "$asset" --arg sha256 "$actual" \
      '{core: $core, platform: $platform, tag: $tag, asset: $asset, archiveSha256: $sha256}' \
      > artifacts/napcat-candidate-evidence.json
    export NAPCAT_RELEASE_TAG="$tag"
    export NAPCAT_ASSET="$asset"
    export NAPCAT_ARCHIVE="$archive"
    export NAPCAT_ARCHIVE_SHA256="$actual"
    ;;
  linux-amd64|linux-arm64)
    # Linux uses a reviewed rootless installer commit instead of a Release ZIP.
    # The platform command must still write the same lifecycle report format.
    ;;
  *)
    echo "unsupported NapCat E2E platform: $NAPCAT_PLATFORM" >&2
    exit 2
    ;;
esac

bash -lc "$NAPCAT_E2E_COMMAND"
test -f artifacts/napcat-e2e-report.json

evidence="$(python3 - "$NAPCAT_PLATFORM" <<'PY'
import json
from pathlib import Path
import sys

platform = sys.argv[1]
with open("artifacts/napcat-e2e-report.json", encoding="utf-8") as handle:
    report = json.load(handle)
if report.get("platform") != platform:
    raise SystemExit("E2E report platform does not match the selected runner")
if report.get("processModel") != "foreground":
    raise SystemExit("NapCat E2E requires a foreground, process-group-manageable model")
stages = report.get("stages")
required = ("install", "gatewayWebUI", "loginPending", "oneBot", "accountConfig", "stop", "update", "rollback")
if not isinstance(stages, dict) or any(stages.get(stage) is not True for stage in required):
    raise SystemExit("E2E report is missing a successful lifecycle stage")
if report.get("temporaryEvidenceInjected") is not True:
    raise SystemExit("E2E report must prove that the runner used temporary candidate evidence")
evidence = report.get("evidence")
if not isinstance(evidence, dict):
    raise SystemExit("E2E report is missing immutable release evidence")
if not isinstance(report.get("websocketRequired"), bool):
    raise SystemExit("E2E report must state whether WebSocket is required")
candidate_path = Path("artifacts/napcat-candidate-evidence.json")
if platform == "windows-amd64":
    if not candidate_path.exists():
        raise SystemExit("Windows candidate evidence was not collected")
    candidate = json.loads(candidate_path.read_text(encoding="utf-8"))
    for field in ("tag", "asset", "archiveSha256"):
        if evidence.get(field) != candidate.get(field):
            raise SystemExit(f"E2E report {field} does not match the official candidate")
evidence["core"] = "napcat"
evidence["platform"] = platform
evidence["processModel"] = "foreground"
evidence["websocketRequired"] = report["websocketRequired"]
evidence["status"] = "passed"
print(json.dumps(evidence, separators=(",", ":")))
PY
)"

NAPCAT_VALIDATION_EVIDENCE="$evidence" python3 scripts/validate-runtime-evidence.py
printf '%s\n' "$evidence" > artifacts/napcat-validation.json
echo "NapCat E2E evidence recorded in artifacts/napcat-validation.json"
