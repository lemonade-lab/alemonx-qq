#!/usr/bin/env python3
"""Set the release version in alx.json from a Git tag."""

import json
import re
import sys
from pathlib import Path


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: set-version.py <tag>", file=sys.stderr)
        return 2
    tag = sys.argv[1].strip()
    version = tag[1:] if tag.startswith("v") else tag
    # Accept SemVer-style release versions, including valid 0.x.y releases.
    if not re.fullmatch(r"(?:0|[1-9]\d*)(?:\.(?:0|[1-9]\d*)){1,2}(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?", version):
        print(f"invalid release tag: {tag}", file=sys.stderr)
        return 1
    path = Path(__file__).resolve().parent.parent / "alx.json"
    data = json.loads(path.read_text(encoding="utf-8"))
    data["version"] = version
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"alx.json version = {version}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
