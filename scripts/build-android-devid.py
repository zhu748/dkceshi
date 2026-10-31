#!/usr/bin/env python3
"""Build a private Android Web SDK bundle from the supplied SDK assets.

Only the allowlisted assets are copied. Accounts, archives, IDs and anchors are
excluded. The fp SDK bytes are preserved; browser_env is adapted reproducibly.
"""

import argparse
import hashlib
import json
import pathlib
import shutil
import subprocess
import zipfile


def build(root, assets, output, node):
    names = ["fp-1.min.js", "gpus.json", "patches.json", "profiles.json",
             "verify.js", "public_key", "browser_env.js", "fonts.json"]
    files = {}
    original_hashes = {}
    for name in names:
        source = assets / name
        if source.is_symlink() or not source.is_file():
            raise ValueError(f"Missing regular asset: {source}")
        data = source.read_bytes()
        if len(data) > 5 * 1024 * 1024:
            raise ValueError(f"Unexpected asset size: {source}")
        original_hashes[name] = hashlib.sha256(data).hexdigest()
        files["data/devid/" + name] = data
    program = """
const fs = require('fs');
const path = require('path');
const root = process.argv[1], assets = process.argv[2];
const { adaptBrowserEnv } = require(path.join(root, 'scripts/devid-android-env.cjs'));
const { VIVO_SPECS, ANDROID_FONTS, mobileHardware } = require(path.join(root, 'scripts/devid-android-profile.cjs'));
const screens = {}, gpus = {};
for (const d of VIVO_SPECS) {
  const h = mobileHardware({ device_model: d[0] });
  screens[d[0]] = h.screen; gpus[d[0]] = h.gpu;
}
process.stdout.write(JSON.stringify({
  env: adaptBrowserEnv(fs.readFileSync(path.join(assets, 'browser_env.js'), 'utf8')),
  fonts: ANDROID_FONTS, screens, gpus,
}));
"""
    adapted = json.loads(subprocess.check_output(
        [node, "-e", program, str(root), str(assets)], text=True, encoding="utf-8"))
    files["data/devid/browser_env.js"] = adapted["env"].encode()
    profiles = json.loads(files["data/devid/profiles.json"])
    profiles.update({"version": 4, "_comment": "Android Web SDK simulated browser profiles; see ANDROID-DEVID-README.md",
                     "osFamilies": [{"key": "android", "weight": 1,
                                     "screens": [[420, 933]], "dprs": [3],
                                     "cores": [8], "taskbar": [0]}],
                     "modelScreens": adapted["screens"]})
    files["data/devid/profiles.json"] = json.dumps(profiles, ensure_ascii=False, indent=2).encode()
    files["data/devid/gpus.json"] = json.dumps({
        "version": 1, "_comment": "GPU chip-name templates, not measured driver strings",
        "modelGpus": adapted["gpus"],
        "entries": [dict(v, families=["android"]) for v in adapted["gpus"].values()],
    }, ensure_ascii=False, indent=2).encode()
    files["data/devid/fonts.json"] = json.dumps(adapted["fonts"], ensure_ascii=False, indent=2).encode()
    runtime = json.loads((root / "scripts/devid-files.json").read_text())
    for name in runtime["scripts"]:
        files["scripts/" + name] = (root / "scripts" / name).read_bytes()
    for name in runtime["metadata"]:
        files[name] = (root / name).read_bytes()
    files["scripts/install-devid-assets.py"] = (root / "scripts/install-devid-assets.py").read_bytes()
    files["ANDROID-DEVID-README.md"] = (root / "docs/android-devid-package.md").read_bytes()
    files["examples/devid-android-anchor.json"] = json.dumps({
        "device_model": "V2405A", "android_version": 15,
        "locale": "zh-CN", "tz": "Asia/Shanghai", "tz_offset": -480,
    }, indent=2).encode()
    manifest = {
        "format": 1, "environment": "android-web-sdk", "adapter_version": 1,
        "native_android_sdk": False, "original_assets_sha256": original_hashes,
        "files_sha256": {name: hashlib.sha256(data).hexdigest() for name, data in files.items()},
    }
    files["data/devid/ANDROID-PACKAGE.json"] = json.dumps(manifest, indent=2).encode()
    output.parent.mkdir(parents=True, exist_ok=True)
    # Deterministic entries, including timestamps and modes. No device data copied.
    with zipfile.ZipFile(output, "w", zipfile.ZIP_DEFLATED) as archive:
        for name, data in sorted(files.items()):
            entry = zipfile.ZipInfo(name, (2026, 10, 29, 0, 0, 0))
            entry.external_attr = 0o100644 << 16
            entry.compress_type = zipfile.ZIP_DEFLATED
            archive.writestr(entry, data)
    print(json.dumps({"file": str(output), "size": output.stat().st_size,
                      "sha256": hashlib.sha256(output.read_bytes()).hexdigest()}))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--assets", type=pathlib.Path)
    parser.add_argument("--output", type=pathlib.Path, required=True)
    parser.add_argument("--node", default=shutil.which("node") or "node")
    args = parser.parse_args()
    repo = pathlib.Path(__file__).resolve().parent.parent
    build(repo, (args.assets or repo / "data/devid").resolve(), args.output.resolve(), args.node)
