#!/usr/bin/env node
/* devid-diff.cjs — 真实指纹 vs vm 指纹 逐字段 diff */
'use strict';
const fs = require('fs');
const crypto = require('node:crypto');
const zlib = require('node:zlib');

function decryptBody(bodyJson, priId) {
    const j = JSON.parse(bodyJson);
    const d = crypto.createDecipheriv('aes-128-cbc',
        Buffer.from(String(priId).slice(0, 16), 'latin1'),
        Buffer.from('0102030405060708', 'latin1'));
    d.setAutoPadding(false);
    let plain = Buffer.concat([d.update(Buffer.from(String(j.data), 'hex')), d.final()]);
    // PKCS7 去
    let pad = plain[plain.length - 1];
    if (pad >= 1 && pad <= 16) {
        let ok = true;
        for (let i = plain.length - pad; i < plain.length; i++) if (plain[i] !== pad) ok = false;
        if (ok) plain = plain.slice(0, plain.length - pad);
    }
    let s = plain.toString('latin1');
    s = s.replace(/[\u0000\r\n \t]+$/, '');
    let nul = s.indexOf('\u0000'); if (nul >= 0) s = s.slice(0, nul);
    while (s.length && !/[A-Za-z0-9+/=]/.test(s[s.length - 1])) s = s.slice(0, -1);
    const gz = Buffer.from(s, 'base64');
    const raw = zlib.gunzipSync(gz);
    return { fields: JSON.parse(raw.toString('latin1')), top: Object.keys(j).filter(k => k !== 'data').map(k => k + '=' + String(j[k]).slice(0, 12)) };
}

const real = JSON.parse(fs.readFileSync('/tmp/real-fp.json', 'utf8'));
const vmBody = fs.readFileSync('/tmp/vm-body.json', 'utf8');
const vmLeak = JSON.parse(fs.readFileSync('/tmp/vm-leak.json', 'utf8'));

const realFp = decryptBody(real.body, real.pri_id);
const vmFp = decryptBody(vmBody, vmLeak.priId);

console.log('真实字段数:', Object.keys(realFp.fields).length, ' vm字段数:', Object.keys(vmFp.fields).length);
const allKeys = [...new Set([...Object.keys(realFp.fields), ...Object.keys(vmFp.fields)])].sort();
const onlyReal = [], onlyVm = [], typeDiff = [], valueDiff = [], same = [];
for (const k of allKeys) {
    const inR = k in realFp.fields, inV = k in vmFp.fields;
    if (inR && !inV) { onlyReal.push(k + ' = ' + JSON.stringify(realFp.fields[k]).slice(0, 80)); continue; }
    if (!inR && inV) { onlyVm.push(k + ' = ' + JSON.stringify(vmFp.fields[k]).slice(0, 80)); continue; }
    const r = realFp.fields[k], v = vmFp.fields[k];
    if (typeof r !== typeof v) { typeDiff.push(k + ': real=' + typeof r + ' vm=' + typeof v + ' (' + JSON.stringify(r).slice(0, 40) + ' vs ' + JSON.stringify(v).slice(0, 40) + ')'); continue; }
    const rs = JSON.stringify(r), vs = JSON.stringify(v);
    if (rs === vs) same.push(k);
    else valueDiff.push(k + ':  real=' + rs.slice(0, 100) + '\n         vm=' + vs.slice(0, 100));
}
console.log('\n===== 仅真实有 (' + onlyReal.length + ') =====');
onlyReal.forEach(x => console.log('  ' + x));
console.log('\n===== 仅 vm 有 (' + onlyVm.length + ') =====');
onlyVm.forEach(x => console.log('  ' + x));
console.log('\n===== 类型不同 (' + typeDiff.length + ') =====');
typeDiff.forEach(x => console.log('  ' + x));
console.log('\n===== 值不同 (' + valueDiff.length + ') =====');
valueDiff.forEach(x => console.log('  ' + x));
console.log('\n完全一致:', same.length, '个 →', same.slice(0, 40).join(', '));
fs.writeFileSync('/tmp/real-fp-fields.json', JSON.stringify(realFp.fields, null, 1));
fs.writeFileSync('/tmp/vm-fp-fields.json', JSON.stringify(vmFp.fields, null, 1));
