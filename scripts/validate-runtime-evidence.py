#!/usr/bin/env python3
"""Validate optional reviewed runtime evidence supplied to release CI."""
import json
import os
import re
import sys

raw = os.environ.get("LUCKYLILLIA_VALIDATION_EVIDENCE", "").strip()
if not raw:
    print("LuckyLillia validation evidence is absent; installation remains disabled.")
    raise SystemExit(0)
try:
    record = json.loads(raw)
except json.JSONDecodeError as exc:
    print(f"invalid LuckyLillia validation evidence: {exc}", file=sys.stderr)
    raise SystemExit(1)

required = ("tag", "asset", "archiveSha256", "validatedAt")
if any(not isinstance(record.get(key), str) or not record[key] for key in required):
    print("validation evidence requires tag, asset, archiveSha256 and validatedAt", file=sys.stderr)
    raise SystemExit(1)
if record["asset"] != "LLBot-CLI-linux-arm64.zip" or not re.fullmatch(r"[0-9a-fA-F]{64}", record["archiveSha256"]):
    print("validation evidence asset or SHA-256 is invalid", file=sys.stderr)
    raise SystemExit(1)
print("LuckyLillia validation evidence accepted for release embedding.")
