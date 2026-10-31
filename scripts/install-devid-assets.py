#!/usr/bin/env python3
"""Validate and install SDK assets without replacing repository runner scripts."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import zipfile


ASSETS = ["fp-1.min.js", "gpus.json", "patches.json", "profiles.json",
          "verify.js", "public_key", "browser_env.js", "fonts.json"]
MANIFEST = "ANDROID-PACKAGE.json"
MAX_FILE = 5 * 1024 * 1024


def read_bundle(source):
    names = ASSETS + [MANIFEST]
    if source.is_dir():
        directory = source / "data" / "devid" if (source / "data" / "devid").is_dir() else source
        files = {}
        for name in names:
            file = directory / name
            if file.is_symlink() or not file.is_file() or file.stat().st_size > MAX_FILE:
                raise ValueError(f"Invalid SDK asset: {name}")
            files[name] = file.read_bytes()
        return files
    with zipfile.ZipFile(source) as bundle:
        files = {}
        for name in names:
            entries = [entry for entry in bundle.infolist() if entry.filename == "data/devid/" + name]
            if len(entries) != 1 or entries[0].is_dir() or entries[0].file_size > MAX_FILE:
                raise ValueError(f"Missing, duplicate or oversized SDK asset: {name}")
            if (entries[0].external_attr >> 16) & 0o170000 == 0o120000:
                raise ValueError(f"Symlink SDK asset: {name}")
            files[name] = bundle.read(entries[0])
        return files


def validate(files):
    manifest = json.loads(files[MANIFEST])
    if (manifest.get("format") != 1 or manifest.get("adapter_version") != 1
            or manifest.get("environment") != "android-web-sdk"
            or manifest.get("native_android_sdk") is not False):
        raise ValueError("Unsupported SDK asset manifest")
    for name in ASSETS:
        digest = hashlib.sha256(files[name]).hexdigest()
        if manifest.get("files_sha256", {}).get("data/devid/" + name) != digest:
            raise ValueError(f"SDK asset checksum mismatch: {name}")


def install(source, target, node, verify_only=False):
    files = read_bundle(source)
    validate(files)
    root = Path(__file__).resolve().parent.parent
    with tempfile.TemporaryDirectory(prefix="devid-check-") as temporary:
        stage = Path(temporary)
        for name, data in files.items():
            (stage / name).write_bytes(data)
        env = dict(os.environ, DS2API_DEVID_DIR=str(stage))
        checked = subprocess.run([node, str(root / "scripts/devid-generate.cjs"), "--selftest", "--timeout", "10"],
                                 env=env, capture_output=True, text=True, encoding="utf-8", timeout=15)
        if checked.returncode:
            raise ValueError("SDK offline selftest failed: " + checked.stdout.strip() + " " + checked.stderr[-1500:])
        result = json.loads(checked.stdout.strip().splitlines()[-1])
        if not result.get("ok") or not result.get("sdk_payload"):
            raise ValueError("SDK runner did not validate offline payload generation")
    if not verify_only:
        target.mkdir(parents=True, exist_ok=True)
        previous = {}
        written = []
        try:
            for name, data in files.items():
                file = target / name
                if file.is_symlink() or (file.exists() and not file.is_file()):
                    raise ValueError(f"SDK destination is not a regular file: {file}")
                previous[name] = file.read_bytes() if file.exists() else None
                atomic_write(file, data)
                written.append(name)
        except Exception:
            for name in reversed(written):
                if previous[name] is None:
                    (target / name).unlink()
                else:
                    atomic_write(target / name, previous[name])
            raise
    return dict(ok=True, installed=not verify_only, assets_dir=str(target),
                assets_verified=True, sdk_payload=True)


def atomic_write(file, data):
    fd, temporary = tempfile.mkstemp(prefix=".sdk-install-", dir=file.parent)
    try:
        with os.fdopen(fd, "wb") as stream:
            # SDK resources contain no account state; containers may read them
            # under a different UID. Generated device archives stay private.
            os.chmod(temporary, 0o644)
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, file)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", type=Path, help="SDK zip or extracted asset directory")
    parser.add_argument("--assets-dir", type=Path, default=Path(os.environ.get("DS2API_DEVID_DIR", "data/devid")))
    parser.add_argument("--node", default=shutil.which("node") or "node")
    parser.add_argument("--verify-only", action="store_true")
    args = parser.parse_args()
    try:
        print(json.dumps(install(args.source.resolve(), args.assets_dir.resolve(), args.node, args.verify_only)))
    except (ValueError, OSError, subprocess.SubprocessError, zipfile.BadZipFile) as error:
        parser.exit(1, f"SDK installation failed: {error}\n")
