#!/usr/bin/env python3
"""Validate optional reviewed runtime evidence supplied to release CI."""
import json
import os
import re
import sys

def records_from_env(name, label):
    raw = os.environ.get(name, "").strip()
    if not raw:
        print(f"{label} validation evidence is absent; installation remains disabled.")
        return []
    try:
        records = json.loads(raw)
    except json.JSONDecodeError as exc:
        print(f"invalid {label} validation evidence: {exc}", file=sys.stderr)
        raise SystemExit(1)
    if isinstance(records, dict):
        records = [records]
    if not isinstance(records, list) or not records:
        print("validation evidence must be one record or a non-empty record list", file=sys.stderr)
        raise SystemExit(1)
    return records

records = records_from_env("LUCKYLILLIA_VALIDATION_EVIDENCE", "LuckyLillia")
supported_assets = {
    "linux-arm64": "LLBot-CLI-linux-arm64.zip",
    "linux-amd64": "LLBot-CLI-linux-x64.zip",
    "windows-amd64": "LLBot-CLI-win-x64.zip",
    "darwin-arm64": "LLBot-CLI-macos-arm64.tar.xz",
}
if records:
    seen = set()
    for record in records:
        required = ("tag", "platform", "asset", "archiveSha256", "runtimeFingerprint", "validatedAt", "processModel")
        if not isinstance(record, dict) or any(not isinstance(record.get(key), str) or not record[key] for key in required):
            print("validation evidence requires tag, platform, asset, archiveSha256, runtimeFingerprint, validatedAt and processModel", file=sys.stderr)
            raise SystemExit(1)
        if record["platform"] in seen or record["platform"] not in supported_assets or record["asset"] != supported_assets[record["platform"]] or not re.fullmatch(r"[0-9a-fA-F]{64}", record["archiveSha256"]) or not re.fullmatch(r"[0-9a-fA-F]{64}", record["runtimeFingerprint"]) or record["processModel"] != "foreground" or not isinstance(record.get("websocketRequired"), bool):
            print("validation evidence asset, platform, SHA-256, WebSocket or process model is invalid", file=sys.stderr)
            raise SystemExit(1)
        seen.add(record["platform"])
    print("LuckyLillia validation evidence accepted for release embedding.")

napcat_records = records_from_env("NAPCAT_VALIDATION_EVIDENCE", "NapCat")
if napcat_records:
    seen = set()
    for record in napcat_records:
        if not isinstance(record, dict) or any(not isinstance(record.get(key), str) or not record[key] for key in ("platform", "runtimeFingerprint", "validatedAt", "processModel")) or not isinstance(record.get("websocketRequired"), bool):
            print("NapCat evidence requires platform, runtimeFingerprint, validatedAt and processModel", file=sys.stderr)
            raise SystemExit(1)
        if record["platform"] in seen or not re.fullmatch(r"[0-9a-fA-F]{64}", record["runtimeFingerprint"]) or record["processModel"] != "foreground":
            print("NapCat evidence platform, runtime fingerprint or process model is invalid", file=sys.stderr)
            raise SystemExit(1)
        if record["platform"] == "windows-amd64":
            required = ("tag", "asset", "archiveSha256")
            if any(not isinstance(record.get(key), str) or not record[key] for key in required) or record["asset"] != "NapCat.Shell.Windows.OneKey.zip" or not re.fullmatch(r"[0-9a-fA-F]{64}", record["archiveSha256"]):
                print("Windows NapCat evidence must bind the OneKey Release tag, asset and SHA-256", file=sys.stderr)
                raise SystemExit(1)
        elif record["platform"] in ("linux-amd64", "linux-arm64"):
            required = ("installerCommit", "installerSha256")
            if any(not isinstance(record.get(key), str) or not record[key] for key in required) or not re.fullmatch(r"[0-9a-fA-F]{64}", record["installerSha256"]):
                print("Linux NapCat evidence must bind the reviewed rootless installer commit and SHA-256", file=sys.stderr)
                raise SystemExit(1)
        else:
            print("NapCat evidence platform is unsupported", file=sys.stderr)
            raise SystemExit(1)
        seen.add(record["platform"])
    print("NapCat validation evidence accepted for release embedding.")
