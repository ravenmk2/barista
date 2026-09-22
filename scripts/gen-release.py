#!/usr/bin/env python3
import argparse
import hashlib
import json
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path

EXECUTABLE_KINDS = {".tar.gz": "executable-tgz", ".zip": "executable-zip"}


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("dist", help="directory containing the release assets")
    ap.add_argument("version", help="release tag (e.g. v0.7.0)")
    ap.add_argument("output", help="manifest output path (release.json)")
    ap.add_argument("--commit", default=None, help="git commit SHA the release is built from (default: git rev-parse HEAD)")
    ap.add_argument("--base-url", default=None, help="when set, assets get an absolute url under this base (mirror copies)")
    args = ap.parse_args()
    dist = Path(args.dist)

    commit = args.commit
    if commit is None:
        r = subprocess.run(["git", "rev-parse", "HEAD"], capture_output=True, text=True)
        if r.returncode != 0:
            print("cannot resolve commit: git rev-parse HEAD failed", file=sys.stderr)
            return 2
        commit = r.stdout.strip()

    assets = []
    for f in sorted(dist.glob("barista-*")):
        stem, kind = None, None
        for suffix, k in EXECUTABLE_KINDS.items():
            if f.name.endswith(suffix):
                stem, kind = f.name[: -len(suffix)], k
                break
        if stem is None:
            continue
        parts = stem.rsplit("-", 2)
        if len(parts) != 3 or parts[0] != f"barista-{args.version}":
            print(f"unexpected artifact name: {f.name}", file=sys.stderr)
            return 2
        _, os_name, arch = parts
        asset = {
            "kind": kind,
            "platforms": [f"{os_name}/{arch}"],
            "file": f.name,
            "entry": "barista.exe" if os_name == "windows" else "barista",
            "hashes": {"sha256": sha256(f)},
            "size": f.stat().st_size,
        }
        if args.base_url:
            asset["url"] = args.base_url.rstrip("/") + "/" + f.name
        assets.append(asset)
    if not assets:
        print(f"no barista-* archives in {dist}", file=sys.stderr)
        return 2

    sums = dist / "checksums.txt"
    if sums.exists():
        assets.append({
            "kind": "checksums",
            "file": "checksums.txt",
            "hashes": {"sha256": sha256(sums)},
            "size": sums.stat().st_size,
        })

    manifest = {
        "schemaVersion": 2,
        "version": args.version,
        "commit": commit,
        "publishedAt": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "assets": assets,
    }
    Path(args.output).write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {args.output} ({len(assets)} assets)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
