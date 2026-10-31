'use strict';
const zlib = require('node:zlib');
const crypto = require('node:crypto');

function toBuffer(ab) {
    if (Buffer.isBuffer(ab)) return ab;
    if (ab instanceof ArrayBuffer) return Buffer.from(new Uint8Array(ab));
    if (ab && ab.buffer) return Buffer.from(new Uint8Array(ab.buffer, ab.byteOffset || 0, ab.byteLength));
    return Buffer.from(ab);
}
function toAB(buf) {
    const out = Buffer.from(buf);
    return out.buffer.slice(out.byteOffset, out.byteOffset + out.byteLength);
}
function makeHostPrims(log = (...args) => console.warn("[devid]", ...args)) {
    /* 定时器：返回宿主句柄，回调在 vm 侧闭包执行（跨 realm 函数调用安全） */
    let nextId = 1;
    const timers = new Map();
    const mk = (loopFn, isInterval) => (fn, ms, ...a) => {
        const id = nextId++;
        const h = loopFn(() => {
            if (!isInterval) timers.delete(id);
            try { if (typeof fn === 'function') fn(...a); }
            catch (e) { log('timer 回调异常:', e && e.message); }
        }, Math.max(0, Number(ms) || 0));
        timers.set(id, h);
        return id;
    };
    const prims = {
        setTimeout: mk(setTimeout, false),
        clearTimeout: (id) => { const h = timers.get(id); if (h) { clearTimeout(h); timers.delete(id); } },
        setInterval: mk(setInterval, true),
        clearInterval: (id) => { const h = timers.get(id); if (h) { clearInterval(h); timers.delete(id); } },
        /* 栈帧：宿主 Error.stack 同步携带 vm 侧帧（文件名=eval 时指定的 filename），
         * 解析为 [[name, pos], …]；browser_env 会渲染成 V8 格式并跳过 frame 0。 */
        stackFrames: () => {
            const lines = (new Error().stack || '').split('\n');
            const out = [];
            for (let i = 1; i < lines.length; i++) {
                const m = lines[i].match(/^\s*at (.+?) \((.+?)\)$/) || lines[i].match(/^\s*at (.+)$/);
                if (!m) continue;
                const name = m[1], pos = m[2] || '';
                if (/node:internal|node:vm|devid-generate|Script\.runIn|Module\.runIn/.test(name + ' ' + pos)) continue;
                out.push([name, pos]);
            }
            return out;
        },
        zlibDeflate: (ab) => toAB(zlib.deflateSync(toBuffer(ab))),
        randomBytes: (n) => toAB(crypto.randomBytes(Math.max(0, n | 0))),
        /* verify.js 的解密原语（--debug 体检用） */
        aesCbcDecryptHex: (hex, key, iv) => {
            try {
                const d = crypto.createDecipheriv('aes-128-cbc',
                    Buffer.from(String(key).slice(0, 16), 'latin1'),
                    Buffer.from(String(iv), 'latin1'));
                d.setAutoPadding(false);
                return toAB(Buffer.concat([d.update(Buffer.from(String(hex), 'hex')), d.final()]));
            } catch (_) { return null; }
        },
        inflate: (ab) => { try { return toAB(zlib.gunzipSync(toBuffer(ab))); } catch (_) { return null; } },
        b64Decode: (s) => { try { return toAB(Buffer.from(String(s), 'base64')); } catch (_) { return null; } },
    };
    return prims;
}


module.exports = { makeHostPrims };
