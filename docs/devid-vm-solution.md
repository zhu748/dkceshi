# devid 无浏览器指纹方案（vm 轻量版）

## 结论

`scripts/devid-generate.cjs` 默认使用 **Node vm 模拟环境**运行数美 fp Web SDK。
2026-10-29 在用户提供的桌面包上增加 Android 模拟层：vivo X200 Pro / X100 Ultra
上报成功（HTTP 200、code=1100、detail.c=0，样本约 4.60 / 3.89 秒），
未使用真实账号验证登录、聊天或上传。原包的 Android 和站点配置曾被忽略，现已通过
运行时适配与检查修复。安装说明及限制见 [Android Web SDK 适配包](android-devid-package.md)。
后续排雷内容保留为历史调试记录，不作为当前 Android 全链路验证依据。

| 指标 | Playwright 旧方案 | vm 轻量方案 |
|------|-----------------|------------|
| 单次生成耗时 | 历史记录 2.5~30 s | 本次 Android 模拟样本约 3.89–4.60 s |
| 资源占用 | Chromium 进程 200~400 MB | Node 单进程 ~60 MB |
| 自检 | — | Android 模拟运行时自检 88/88 |
| 兜底 | — | `--browser` 参数回落 Playwright |

## 运行链

```
Node vm: browser_env.js(环境模拟) + 打补丁的 fp-1.min.js(数美 SDK)
  → SDK 读 window._smConf 自初始化，采集指纹并加密
  → XHR 被 mock 捕获进 __captured
  → Node fetch 真实转发 fp-it-acc.portal101.cn/deviceprofile/v4
  → 响应 code=1100 → device_id = 'B' + detail.deviceId
```

## 关键坑（本次排雷记录）

1. **device_id 的 B 前缀（最大坑）**：SDK `getDeviceId()` 返回 `"B" + 响应
   detail.deviceId`。直接用响应原文缺 B 前缀 → 登录一律 RISK_DEVICE_DETECTED，
   且数美端照常签发（code=1100 c=0），极具迷惑性。修复见 generate.cjs。
2. **Intl 必须用宿主实现**：`Intl.DateTimeFormat()` 无 new 调用在原 IntlShim
   下直接 TypeError → `sd` 字段空 / `ia` 字段缺失。vm 里 Intl 用 Node ICU，
   时区经 `process.env.TZ = 画像.tzName` 与档案同源。
3. **headless 真实基准**：Playwright headless 143 实测（无 window.chrome、
   plugins/mimeTypes 空集、userActivation active、enumerateDevices 空）——
   这些"像真桌面"的伪造反而与人群库不符。环境对齐以实抓为准。
4. **hook 检测位 kq**：mock 函数不包 nativeFn 会暴露 JS 源码 → kq=1（检测到
   hook）→ 风险分。mediaDevices/permissions/matchMedia/storage 全部补包。
5. **document 自有属性**：真实 Chrome `Object.keys(document)` 含 `location`
   + React 站点的 `_reactListening<随机>`；bc 字段收的就是它。
6. **dv.touchEvent**：真值来自 `'TouchEvent' in window`（Chrome 桌面恒真），
   `window.ontouchstart` 桌面不存在（加了反而 touchStart=true 露馅）。

## 诊断工具链（scripts/）

- `devid-probe-site.cjs` — Playwright 抓线上 `_smConf` 站点参数（换配置时重跑）
- `devid-probe-real.cjs` — 真浏览器加载补丁版 SDK，抓真实指纹明文 + priId（diff 基准）
- `env-snapshot.cjs` — 同一份 API 探测序列对比真浏览器 vs vm 环境
- `devid-diff.cjs` — 双份 payload 离线解密后 80 字段逐项 diff

换 SDK 版本流程：重跑 probe-real 抓新基准 → env-snapshot 对齐 API 层 →
diff 确认字段层 → verify.js 明文体检 → 登录实测。

## 档案（设备身份）

`data/devid/profiles/<identifier>.json` v3 格式：`{profile, smid_v2}`。
复用 = 同设备新指纹；`reset_profile` = 换新设备。兼容读旧 storageState
（自动提取 smidV2 延续设备身份）。
