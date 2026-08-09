#!/usr/bin/env python3
"""Validate reviewed runtime evidence and prepare platform-specific bundles."""
import argparse
import base64
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SHA = re.compile(r"^[0-9a-fA-F]{64}$")
LUCKY_ASSETS = {
    "windows-amd64": "LLBot-CLI-win-x64.zip",
    "darwin-arm64": "LLBot-CLI-macos-arm64.tar.xz",
    "linux-amd64": "LLBot-CLI-linux-x64.zip",
    "linux-arm64": "LLBot-CLI-linux-arm64.zip",
}


def fail(message: str) -> None:
    raise ValueError(message)


def load(core: str, platform: str):
    path = ROOT / "evidence" / core / f"{platform}.json"
    if not path.exists():
        return None
    try:
        record = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        fail(f"{path}: invalid JSON: {exc}")
    if not isinstance(record, dict):
        fail(f"{path}: evidence must be an object")
    validate(core, platform, record, path)
    return record


def required(record, *fields):
    for field in fields:
        if not isinstance(record.get(field), str) or not record[field]:
            fail(f"evidence requires {field}")


def validate(core: str, platform: str, record: dict, path: Path) -> None:
    if record.get("core") != core or record.get("platform") != platform:
        fail(f"{path}: core/platform does not match its path")
    required(record, "validatedAt", "processModel", "runtimeFingerprint")
    if record.get("status") != "passed" or record["processModel"] != "foreground" or not SHA.fullmatch(record["runtimeFingerprint"]):
        fail(f"{path}: status, process model or runtime fingerprint is invalid")
    if not isinstance(record.get("websocketRequired"), bool):
        fail(f"{path}: websocketRequired must be boolean")
    if core == "luckylillia":
        required(record, "tag", "asset", "archiveSha256")
        if platform not in LUCKY_ASSETS or record["asset"] != LUCKY_ASSETS[platform] or not SHA.fullmatch(record["archiveSha256"]):
            fail(f"{path}: LuckyLillia asset or SHA-256 is invalid")
    elif core == "napcat":
        if platform == "windows-amd64":
            required(record, "tag", "asset", "archiveSha256")
            if record["asset"] != "NapCat.Shell.Windows.OneKey.zip" or not SHA.fullmatch(record["archiveSha256"]):
                fail(f"{path}: Windows NapCat asset or SHA-256 is invalid")
        elif platform in {"linux-amd64", "linux-arm64"}:
            required(record, "installerCommit", "installerSha256")
            if not SHA.fullmatch(record["installerSha256"]):
                fail(f"{path}: Linux NapCat installer SHA-256 is invalid")
        else:
            fail(f"{path}: unsupported NapCat platform")
    else:
        fail(f"unknown core: {core}")


def checked_records():
    evidence = ROOT / "evidence"
    if not evidence.exists():
        return
    for core_dir in evidence.iterdir():
        if not core_dir.is_dir() or core_dir.name not in {"napcat", "luckylillia"}:
            fail(f"unexpected evidence directory: {core_dir}")
        for path in core_dir.glob("*.json"):
            platform = path.stem
            load(core_dir.name, platform)


def prepare_manifest(source: Path, output: Path, platform: str) -> None:
    manifest = json.loads(source.read_text(encoding="utf-8"))
    flags = {
        "napcat-webui": bool((load("napcat", platform) or {}).get("websocketRequired")),
        "luckylillia-webui": bool((load("luckylillia", platform) or {}).get("websocketRequired")),
    }
    for service in manifest.get("services", []):
        if service.get("id") in flags:
            service["websocket"] = flags[service["id"]]
    output.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check-tree", action="store_true")
    parser.add_argument("--core")
    parser.add_argument("--platform")
    parser.add_argument("--encode", action="store_true")
    parser.add_argument("--prepare-manifest", action="store_true")
    parser.add_argument("--input")
    parser.add_argument("--output")
    args = parser.parse_args()
    try:
        if args.check_tree:
            checked_records()
            return 0
        if not args.platform:
            fail("--platform is required")
        if args.prepare_manifest:
            if not args.input or not args.output:
                fail("--prepare-manifest requires --input and --output")
            prepare_manifest(Path(args.input), Path(args.output), args.platform)
            return 0
        if not args.core:
            fail("--core is required")
        record = load(args.core, args.platform)
        if args.encode:
            print(base64.urlsafe_b64encode(json.dumps(record, separators=(",", ":")).encode()).decode().rstrip("=") if record else "")
        return 0
    except ValueError as exc:
        print(f"evidence error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
