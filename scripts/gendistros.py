#!/usr/bin/env python3
"""Regenerate internal/jdk/distros.json — pinned, non-expiring JDK download
URLs for distros resolved through embedded data instead of runtime API calls.

Usage: python3 scripts/gendistros.py  (run from anywhere; writes into the repo)
"""
import concurrent.futures
import datetime
import json
import os
import re
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


GRAALVM_RELEASES = "https://api.github.com/repos/graalvm/graalvm-ce-builds/releases?per_page=100"
GRAALVM_ASSET_RE = re.compile(
    r"graalvm-community-jdk-(?:[0-9a-z]+-)?(\d+(?:\.\d+)*)_(linux|macos|windows)-(x64|aarch64)_bin\.(tar\.gz|zip)$"
)
GRAALVM_PLATFORM = {  # (asset os, asset arch) -> "goos/goarch"
    ("linux", "x64"): "linux/amd64",
    ("linux", "aarch64"): "linux/arm64",
    ("macos", "x64"): "darwin/amd64",
    ("macos", "aarch64"): "darwin/arm64",
    ("windows", "x64"): "windows/amd64",
    ("windows", "aarch64"): "windows/arm64",
}


def get_text(url, timeout=30):
    req = urllib.request.Request(url, headers={"User-Agent": "barista-gendistros"})
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return resp.read().decode()


def version_key(v):
    return tuple(int(x) for x in v.split("."))


def graalvm_releases():
    best = {}  # major -> {"version": str, "assets": {platform: url}}
    for rel in get_json(GRAALVM_RELEASES):
        urls = {a["name"]: a["browser_download_url"] for a in rel.get("assets", [])}
        for name, url in urls.items():
            m = GRAALVM_ASSET_RE.match(name)
            if not m:
                continue
            ver, platform = m.group(1), GRAALVM_PLATFORM[(m.group(2), m.group(3))]
            major = int(ver.split(".")[0])
            cur = best.setdefault(major, {"version": ver, "assets": {}})
            if version_key(ver) < version_key(cur["version"]):
                continue
            if version_key(ver) > version_key(cur["version"]):
                cur["version"], cur["assets"] = ver, {}
            cur["assets"][platform] = url
    def fetch_sha(item):
        platform, url = item
        try:
            return platform, get_text(url + ".sha256", timeout=15).split()[0]
        except Exception as exc:
            print(f"{platform}: sha256 fetch failed ({exc}), checksum omitted")
            return platform, None

    releases = []
    for major in sorted(best):
        entry = best[major]
        assets = {}
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as ex:
            for platform, sha in ex.map(fetch_sha, sorted(entry["assets"].items())):
                asset = {"url": entry["assets"][platform]}
                if sha:
                    asset["sha256"] = sha
                assets[platform] = asset
        releases.append({
            "major": major, "version": entry["version"],
            "lts": major in LTS_MAJORS, "assets": assets,
        })
        print(f"graalvm {major}: {entry['version']}, {len(assets)} platforms")
    return releases


def main():
    data = {
        "schemaVersion": 1,
        "generatedAt": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "distros": {"zulu": zulu_releases(), "graalvm": graalvm_releases()},
    }
    with open(OUT_PATH, "w", encoding="utf-8", newline="\n") as f:
        json.dump(data, f, indent=2, ensure_ascii=False)
        f.write("\n")
    print(f"wrote {OUT_PATH}")


if __name__ == "__main__":
    main()
