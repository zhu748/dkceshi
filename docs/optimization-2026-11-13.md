# 机型池扩展与上游引用清理

基线：`main` 分支 `8b02649`。本轮完成两件事：devid 设备扩展包机型池大幅扩充；移除已停更上游
`CJackHwang/ds2api` 的全部引用，本仓库（`zhu748/dkceshi`）与活跃上游（`ouqiting/ds2api`）指向各自位置。

## 机型池扩展

| 范围 | 变化 |
| --- | --- |
| 规格表 | `internal/config/android_device_specs.json` 从 19 条扩至 54 条：vivo 新增 X200s/X300/X300 Pro（官网参数页核验）；新增 iQOO 3 条、小米 10 条、Redmi 4 条、OPPO 5 条、一加 3 条、荣耀 7 条 |
| 字段 | 每条新增 `brand`（Build.BRAND 精确映射）与 `source`（硬件数据来源）；Go 与 Node 继续共用该文件 |
| 池规模 | 型号 44→87 种；型号/API 预设 66→117 组；Pixel/Samsung 新增 Pixel 10 系列与 SM-S9210/SM-S9310/SM-S731B/SM-S937B/SM-A556B |
| 品牌映射 | `AndroidBrandForModel` 先查规格表 brand 精确映射，池外型号按前缀推断（数字开头→xiaomi、`P`→oppo），保持原有 Pixel/SM-/V 规则 |
| Node 侧 | `VIVO_SPECS` 更名为 `DEVICE_SPECS`（多品牌语义），元组尾部追加 brand/source；`hardwareSource` 直接读取 source 字段；generic GPU 池扩充 Adreno 830、Immortalis-G720/-G925、Mali-G610/G710 |

新增品牌型号代码经 Google Play 官方设备目录（supported_devices）核验；vivo 新机型直接核验
官网参数页（分辨率/GPU/Android 版本）。iQOO 型号沿用 `V` 前缀归入 vivo 品牌值，与既有
V2507A 处理一致。宽高字段统一为纵向物理分辨率（宽<高）。

## 上游引用清理

- README 双语：徽章、Star History、Vercel 部署按钮与克隆命令全部指向 `zhu748/dkceshi`；
  移除属于旧仓库的 Trendshift 徽章。
- API 双语文档：`/admin/version` 示例的 `release_url` 与实现一致指向 `ouqiting/ds2api`（版本检查的真实来源）。
- `config.example.json`、WebUI（index.html/DashboardShell/LandingPage）、内置 welcome.html、
  DEPLOY/CONTRIBUTING 双语文档同步替换。
- CODE_OF_CONDUCT/SECURITY 弃用旧维护者个人邮箱，改为 GitHub Issues 与仓库私密安全报告入口。
- `ouqiting` 相关引用（提示词兼容对齐、版本检查）保持不变。

## 验证

- Go：`internal/config`、`internal/devid`、`internal/deepseek/client`、`internal/server`、
  `internal/httpapi/admin/...` 全部测试通过；`go vet` 无告警（Go 1.26）。
- Node：`devid-android`、`devid-cli`、`devid-runtime-check`、`devid-assets` 测试通过；
  无 SDK 资源的环境中按原设计跳过资源集成用例。
- 全仓库 `cjackhwang`（含大小写变体）零残留；git 历史不动。

上述结果不代表真实 DeepSeek 登录、云端设备签发或生产负载已验证。新品牌条目的 DPR、
CSS 尺寸与完整 WebGL 驱动串仍为模拟值，精度边界与 [适配包说明](android-devid-package.md#数据精度) 相同。
