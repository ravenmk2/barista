#!/usr/bin/env python3
"""Regenerate internal/jdk/distros.json — pinned, non-expiring JDK download
URLs for distros resolved through embedded data instead of runtime API calls.

Usage: python3 scripts/gendistros.py  (run from anywhere; writes into the repo)
"""
import datetime
import json
import os
import urllib.parse
import urllib.request

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT_PATH = os.path.join(REPO_ROOT, "internal", "jdk", "distros.json")

# (goos, goarch) -> (azul os, azul arch, archive_type)
ZULU_MATRIX = [
    ("windows", "amd64", "windows", "x64", "zip"),
    ("windows", "arm64", "windows", "aarch64", "zip"),
    ("linux", "amd64", "linux", "x64", "tar.gz"),
    ("linux", "arm64", "linux", "aarch64", "tar.gz"),
    ("darwin", "amd64", "macos", "x64", "tar.gz"),
    ("darwin", "arm64", "macos", "aarch64", "tar.gz"),
]
ZULU_MAJORS = [8, 11, 17, 21, 25]
LTS_MAJORS = {8, 11, 17, 21, 25}


def get_json(url):
    req = urllib.request.Request(url, headers={"User-Agent": "barista-gendistros"})
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.load(resp)


def java_version_string(arr):
    # [1, 8, 0, 472] -> "1.8.0_472"; [17, 0, 20, 1] -> "17.0.20.1"
    if arr[0] == 1:
        return f"1.{arr[1]}.0_{arr[3]}" if len(arr) > 3 else f"1.{arr[1]}.0_{arr[2]}"
    return ".".join(map(str, arr))


def zulu_releases():
    releases = []
    for major in ZULU_MAJORS:
        assets, version = {}, ""
        for goos, goarch, azul_os, azul_arch, ext in ZULU_MATRIX:
            q = urllib.parse.urlencode({
                "java_version": major, "os": azul_os, "arch": azul_arch,
                "archive_type": ext, "java_package_type": "jdk",
                "release_status": "ga", "latest": "true", "availability_type": "CA",
            })
            pkgs = get_json(f"https://api.azul.com/metadata/v1/zulu/packages/?{q}")
            plain = [p for p in pkgs if "-ca-jdk" in p["name"] and "musl" not in p["name"]]
            if not plain:
                print(f"zulu {major} {goos}/{goarch}: no build, skipped")
                continue
            best = max(plain, key=lambda p: p["java_version"])
            assets[f"{goos}/{goarch}"] = {"url": best["download_url"]}
            version = java_version_string(best["java_version"])
        if assets:
            releases.append({
                "major": major, "version": version,
                "lts": major in LTS_MAJORS, "assets": assets,
            })
            print(f"zulu {major}: {version}, {len(assets)} platforms")
    return releases


def main():
    data = {
        "schemaVersion": 1,
        "generatedAt": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "distros": {"zulu": zulu_releases()},
    }
    with open(OUT_PATH, "w", encoding="utf-8", newline="\n") as f:
        json.dump(data, f, indent=2, ensure_ascii=False)
        f.write("\n")
    print(f"wrote {OUT_PATH}")


if __name__ == "__main__":
    main()
