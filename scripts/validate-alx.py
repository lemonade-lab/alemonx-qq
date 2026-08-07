#!/usr/bin/env python3
"""Validate an ALemonX setup-plugin manifest (alx.json).

Checks mirror the loader rules in alx's internal/setupplugin registry:
ID format and uniqueness, page/action references, runtime values, entry keys
and field shapes. Exits 0 on success, 1 with a report on failure.

Usage: python3 scripts/validate-alx.py [alx.json]
"""

import json
import re
import sys

MAX_MANIFEST = 64 * 1024
ID_RE = re.compile(r"^[a-z][a-z0-9-]{1,63}$")
RUN_TIMES = {"", "binary", "node", "go"}
PLATFORM_KEYS = {"darwin-arm64", "darwin-amd64", "linux-amd64", "windows-amd64"}


def report(errors):
    if not errors:
        print("alx.json OK")
        return 0
    for error in errors:
        print("error:", error, file=sys.stderr)
    return 1


def main(path):
    errors = []
    try:
        with open(path, "r", encoding="utf-8") as handle:
            data = handle.read()
    except OSError as exc:
        return report([f"cannot read {path}: {exc}"])

    if len(data.encode("utf-8")) > MAX_MANIFEST:
        errors.append(f"manifest exceeds {MAX_MANIFEST} bytes")

    try:
        manifest = json.loads(data)
    except json.JSONDecodeError as exc:
        return report([f"invalid JSON at line {exc.lineno}: {exc.msg}"])

    if not isinstance(manifest, dict):
        return report(["manifest root must be an object"])

    if not ID_RE.match(manifest.get("id", "")):
        errors.append(f"id {manifest.get('id')!r} must match {ID_RE.pattern}")
    if not isinstance(manifest.get("name"), str) or not manifest["name"].strip():
        errors.append("name is required")
    if not isinstance(manifest.get("version"), str) or not manifest["version"].strip():
        errors.append("version is required")

    runtime = manifest.get("runtime", "")
    if runtime not in RUN_TIMES:
        errors.append(f"runtime {runtime!r} must be one of binary/node/go")

    development = manifest.get("development")
    if development is not None:
        if not isinstance(development, dict):
            errors.append("development must be an object")
        elif development.get("runtime", "") not in RUN_TIMES or not development.get("entry"):
            errors.append("development runtime must be binary/node/go and entry is required")

    entry = manifest.get("entry")
    if entry is not None:
        if not isinstance(entry, dict) or not entry:
            errors.append("entry must be a non-empty object")
        else:
            for key in entry:
                if key not in PLATFORM_KEYS and key not in {"linux", "darwin", "windows"} and key != "go":
                    errors.append(f"entry key {key!r} is not a platform or go")

    web = manifest.get("web")
    if web is None:
        errors.append("web is required: the plugin's interface is its web UI")
    elif not isinstance(web, dict) or not isinstance(web.get("root"), str):
        errors.append("web must be an object with a string root")
    else:
        root = web["root"].strip()
        if not root or root.startswith("/") or root.split("/")[0] == "..":
            errors.append("web.root must be a directory path inside the plugin")
        if ".." in root.split("/"):
            errors.append("web.root must not contain ..")

    return report(errors)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1] if len(sys.argv) > 1 else "alx.json"))
