'use strict';
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const vm = require('node:vm');

const REQUIRED = ['profiles.json', 'gpus.json', 'fp-1.min.js', 'browser_env.js', 'public_key'];

// Runner code is distributed with the repository. An asset-only install keeps
// newer code and validates the external resource bytes against its manifest.
function checkAssets(directory, options = {}) {
    if (Number(process.versions.node.split('.')[0]) < 22) throw new Error(`SDK 生成需要 Node.js 22+，当前版本 ${process.versions.node}`);
    const required = options.debug ? [...REQUIRED, 'verify.js'] : REQUIRED;
    const missing = required.filter(name => {
        try {
            const file = path.join(directory, name), info = fs.lstatSync(file);
            fs.accessSync(file, fs.constants.R_OK);
            return !info.isFile() || info.size === 0 || info.size > 5 * 1024 * 1024;
        } catch { return true; }
    });
    if (missing.length) throw new Error(`SDK 资源不完整或不可读：${missing.join(', ')}。请将资源直接放入 ${directory}`);
    let verified = false;
    const manifestFile = path.join(directory, 'ANDROID-PACKAGE.json');
    if (fs.existsSync(manifestFile)) {
        const manifest = JSON.parse(fs.readFileSync(manifestFile, 'utf8'));
        if (manifest.format !== 1 || manifest.environment !== 'android-web-sdk' || manifest.adapter_version !== 1 || manifest.native_android_sdk !== false) throw new Error('不支持的 SDK 资源清单版本');
        const hashes = manifest.files_sha256 || {};
        for (const name of required) {
            if (!hashes['data/devid/' + name]) throw new Error('SDK 清单缺少资源: ' + name);
        }
        for (const [file, hash] of Object.entries(hashes)) {
            if (!file.startsWith('data/devid/')) continue;
            const name = file.slice('data/devid/'.length);
            if (!name || path.basename(name) !== name || name.includes('\\') || !/^[a-f0-9]{64}$/.test(hash)) throw new Error('SDK 清单路径或摘要无效');
            const target = path.join(directory, name), info = fs.lstatSync(target);
            if (!info.isFile() || info.size > 5 * 1024 * 1024 || crypto.createHash('sha256').update(fs.readFileSync(target)).digest('hex') !== hash) throw new Error('SDK 资源校验失败: ' + name);
        }
        verified = true;
    }
    new vm.Script(fs.readFileSync(path.join(directory, 'fp-1.min.js'), 'utf8'), { filename: 'fp-1.min.js' });
    return { verified, directory };
}

module.exports = { checkAssets, REQUIRED };
