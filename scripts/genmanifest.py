#!/usr/bin/env python3
import hashlib
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

ARCHIVE_FORMATS = {".tar.gz": "tar.gz", ".zip": "zip"}


def main() -> int:
    if len(sys.argv) != 4:
        print(f"usage: {sys.argv[0]} <dist-dir> <version> <output>", file=sys.stderr)
        return 2
    dist, version, output = Path(sys.argv[1]), sys.argv[2], Path(sys.argv[3])

    assets = {}
    for f in sorted(dist.glob("barista-*")):
        stem, fmt = None, None
        for suffix, archive_format in ARCHIVE_FORMATS.items():
            if f.name.endswith(suffix):
                stem, fmt = f.name[: -len(suffix)], archive_format
                break
        if stem is None:
            continue
        parts = stem.rsplit("-", 2)
        if len(parts) != 3 or parts[0] != f"barista-{version}":
            print(f"unexpected artifact name: {f.name}", file=sys.stderr)
            return 2
        _, os_name, arch = parts
        assets[f"{os_name}/{arch}"] = {
            "file": f.name,
            "format": fmt,
            "entry": "barista.exe" if os_name == "windows" else "barista",
            "sha256": hashlib.sha256(f.read_bytes()).hexdigest(),
            "size": f.stat().st_size,
        }
    if not assets:
        print(f"no barista-* archives in {dist}", file=sys.stderr)
        return 2

    manifest = {
        "schemaVersion": 2,
        "version": version,
        "publishedAt": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "assets": assets,
    }
    output.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {output} ({len(assets)} platforms)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
