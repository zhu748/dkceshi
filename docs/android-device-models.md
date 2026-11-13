# Android 设备型号池

未设置账号 `device_model` 和全局 `DS2API_DEEPSEEK_DEVICE_MODEL` 时，首次使用从机型池选择，
然后保存到账号；只读部署按账号稳定选择。后续请求和 SDK ID 刷新复用已分配的机型。
示例配置、Go 基础头和 Node 基础头均不指定固定机型。
已有账号保存的型号及显式配置继续生效；V2507A 仅作为原有可选项保留。

当前共 87 种型号、117 组型号/API 预设。其中 54 种（vivo 22、iQOO 3、小米 10、Redmi 4、
OPPO 5、一加 3、荣耀 7）在 `internal/config/android_device_specs.json` 中带有
分辨率/GPU 芯片名硬件规格，由 Go 与 Node 共用；Google Pixel 与 Samsung 型号沿用
通用移动模板，机型/API 预设成对扩展。
型号代码用于 `x-device-model`、`check_device.device_model` 和 SDK 设备锚定。
API 是本地画像预设，可由账号配置覆盖，不代表已实测的固件版本或承诺升级支持。

型号代码经 Google Play 官方设备目录（supported device catalog）核验；vivo 条目的
分辨率与 GPU 来自 vivo 官网参数页，其余品牌来自厂商公开规格页，核验日期 2026-11-13。
iQOO 型号按项目既有约定归入 vivo 品牌值（`V` 前缀）。

## vivo 机型

型号代码参照 vivo 官网参数页的全网通/进网信息：

| 产品 | 型号代码 | 默认 API 预设 | 官方参数 |
| --- | --- | --- | --- |
| X90 | V2241A | 33 | [vivo](https://www.vivo.com.cn/vivo/param/x90) |
| X90 Pro | V2242A | 33 | [vivo](https://www.vivo.com.cn/vivo/param/x90pro) |
| X90 Pro+ | V2227A | 33 | [vivo](https://www.vivo.com.cn/vivo/param/x90proplus) |
| X100 | V2309A | 34 | [vivo](https://www.vivo.com.cn/vivo/param/x100) |
| X100 Pro | V2324A | 34 | [vivo](https://www.vivo.com.cn/vivo/param/x100pro) |
| X100s | V2359A | 34 | [vivo](https://www.vivo.com.cn/vivo/param/x100s) |
| X100s Pro | V2324HA | 34 | [vivo](https://www.vivo.com.cn/vivo/param/x100spro) |
| X100 Ultra | V2366GA、V2366HA | 34 | [vivo](https://www.vivo.com.cn/vivo/param/x100ultra) |
| X200 | V2415A | 35 | [vivo](https://www.vivo.com.cn/vivo/param/x200) |
| X200 Pro | V2405A、V2405DA | 35 | [vivo](https://www.vivo.com.cn/vivo/param/x200pro) |
| X200 Pro mini | V2419A | 35 | [vivo](https://www.vivo.com.cn/vivo/param/x200promini) |
| X200 Ultra | V2454A、V2454DA | 35 | [vivo](https://www.vivo.com.cn/vivo/param/x200ultra) |
| X200s | V2458A | 35 | [vivo](https://www.vivo.com.cn/vivo/param/x200s) |
| X300 | V2509A | 36 | [vivo](https://www.vivo.com.cn/vivo/param/x300) |
| X300 Pro | V2502A | 36 | [vivo](https://www.vivo.com.cn/vivo/param/x300pro) |
| S19 | V2364A | 34 | [vivo](https://www.vivo.com.cn/vivo/param/s19) |
| S19 Pro | V2362A | 34 | [vivo](https://www.vivo.com.cn/vivo/param/s19pro) |
| S20 | V2429A | 35 | [vivo](https://www.vivo.com.cn/vivo/param/s20) |
| S20 Pro | V2430A | 35 | [vivo](https://www.vivo.com.cn/vivo/param/s20pro) |
| 原抓包保留项 | V2507A | 36 | 仅作为可选模板，不作默认值 |

## iQOO 机型

型号代码经 Google Play 设备目录核验，硬件规格来自厂商公开规格页：

| 产品 | 型号代码 | 默认 API 预设 |
| --- | --- | --- |
| iQOO 12 | V2307A | 34 |
| iQOO 13 | V2408A | 35 |
| iQOO Neo9 | V2338A | 34 |

## 小米 / Redmi 机型

型号代码经 Google Play 设备目录核验，硬件规格来自厂商公开规格页：

| 产品 | 型号代码 | 默认 API 预设 |
| --- | --- | --- |
| Xiaomi 12 | 2201123C | 31 |
| Xiaomi 12 Pro | 2201122C | 31 |
| Xiaomi 13 | 2211133C | 33 |
| Xiaomi 13 Pro | 2210132C | 33 |
| Xiaomi 14 | 23127PN0CC | 34 |
| Xiaomi 14 Pro | 23116PN5BC | 34 |
| Xiaomi 14 Ultra | 24031PN0DC | 34 |
| Xiaomi 15 | 24129PN74C | 35 |
| Xiaomi 15 Pro | 2410DPN6CC | 35 |
| Xiaomi 15 Ultra | 25010PN30C | 35 |
| Redmi K70 | 23113RKC6C | 34 |
| Redmi K70 Pro | 23117RK66C | 34 |
| Redmi K80 | 24117RK2CC | 35 |
| Redmi K80 Pro | 24122RKC7C | 35 |

## OPPO 机型

型号代码经 Google Play 设备目录核验，硬件规格来自厂商公开规格页：

| 产品 | 型号代码 | 默认 API 预设 |
| --- | --- | --- |
| Find X7 | PHZ110 | 34 |
| Find X7 Ultra | PHY110 | 34 |
| Find X8 | PKB110 | 35 |
| Find X8 Pro | PKC110 | 35 |
| Find X8 Ultra | PKJ110 | 35 |

## 一加机型

型号代码经 Google Play 设备目录核验，硬件规格来自厂商公开规格页：

| 产品 | 型号代码 | 默认 API 预设 |
| --- | --- | --- |
| OnePlus 12 | PJD110 | 34 |
| OnePlus 13 | PJZ110 | 35 |
| OnePlus Ace 3 | PJE110 | 34 |

## 荣耀机型

型号代码经 Google Play 设备目录核验，硬件规格来自厂商公开规格页：

| 产品 | 型号代码 | 默认 API 预设 |
| --- | --- | --- |
| HONOR Magic6 | BVL-AN00 | 34 |
| HONOR Magic6 Pro | BVL-AN16 | 34 |
| HONOR Magic7 | PTP-N29 | 35 |
| HONOR Magic7 Pro | PTP-N49 | 35 |
| HONOR 100 | MAA-AN00 | 34 |
| HONOR 200 | ELI-AN00 | 34 |
| HONOR 200 Pro | ELP-AN00 | 34 |

## Google Pixel 及 Samsung 机型

原有 Google Pixel 及 Samsung 型号继续保留，并新增 Pixel 10 系列与
SM-S9210/SM-S9310（国行 S24/S25）、SM-S731B（S25 FE）、SM-S937B（S25 Edge）、
SM-A556B（Galaxy A55）。这些型号按 35/36 两种 API 预设入池。
账号可手动配置池外型号。

Android Web SDK 适配器已为上述 54 个规格表机型提供公开分辨率/GPU 芯片名模板。
DPR、字体度量和完整 WebGL 驱动串仍为模拟值；V2507A、Pixel/Samsung 及其他未建表型号
使用明确标记的通用移动模板，可通过 `profiles.json.modelScreens` / `gpus.json.modelGpus`
补充真机参数。本机型表不等于完整真实硬件画像，详见 [适配包精度说明](android-devid-package.md#数据精度)。

`device_brand` 按型号精确映射：规格表内条目使用表内 `brand` 字段（vivo/xiaomi/redmi/oppo/
oneplus/honor），池外型号按前缀推断（`Pixel`→google、`SM-`→samsung、`V`→vivo、
数字开头→xiaomi、`P`→oppo），其余为 `android`。

## 配置优先级

1. 账号显式设置的型号与 API。
2. 全局环境变量设置的型号与 API。
3. 自动从池中选择相匹配的组合。

仅限制 API、且池内没有该 API 预设时，保留这个显式 API，从池中选择型号，
不会再统一回退为 V2507A。token-only 调用按调用者/令牌种子稳定选择型号，
请求头与设备检查使用同一套选择逻辑。
