#!/usr/bin/env python3
import hashlib
import json
import sys
from datetime import datetime, timezone
from pathlib import Path


def main() -> int:
    if len(sys.argv) != 4:
        print(f"usage: {sys.argv[0]} <dist-dir> <version> <output>", file=sys.stderr)
        return 2
    dist, version, output = Path(sys.argv[1]), sys.argv[2], Path(sys.argv[3])

    assets = {}
    for f in sorted(dist.glob("barista-*")):
        platform = f.name.removeprefix("barista-").removesuffix(".exe")
        os_name, _, arch = platform.partition("-")
        if not os_name or not arch:
            print(f"unexpected artifact name: {f.name}", file=sys.stderr)
            return 2
        assets[f"{os_name}/{arch}"] = {
            "file": f.name,
            "sha256": hashlib.sha256(f.read_bytes()).hexdigest(),
            "size": f.stat().st_size,
        }
    if not assets:
        print(f"no barista-* artifacts in {dist}", file=sys.stderr)
        return 2

    manifest = {
        "schemaVersion": 1,
        "version": version,
        "publishedAt": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "assets": assets,
    }
    output.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {output} ({len(assets)} platforms)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
