# Android 设备型号池

未设置账号 `device_model` 和全局 `DS2API_DEEPSEEK_DEVICE_MODEL` 时，首次使用从机型池选择，
然后保存到账号；只读部署按账号稳定选择。后续请求和 SDK ID 刷新复用已分配的机型。
示例配置、Go 基础头和 Node 基础头均不再指定固定机型。
已有账号保存的型号及显式配置继续生效；V2507A 仅作为原有可选项保留。

当前共 44 种型号、66 组型号/API 预设，其中 vivo 20 种（本次新增 19 种型号代码）。
型号代码用于 `x-device-model`、`check_device.device_model` 和 SDK 设备锚定。
API 是本地画像预设，可由账号配置覆盖，不代表已实测的固件版本或承诺升级支持。

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
| S19 | V2364A | 34 | [vivo](https://www.vivo.com.cn/vivo/param/s19) |
| S19 Pro | V2362A | 34 | [vivo](https://www.vivo.com.cn/vivo/param/s19pro) |
| S20 | V2429A | 35 | [vivo](https://www.vivo.com.cn/vivo/param/s20) |
| S20 Pro | V2430A | 35 | [vivo](https://www.vivo.com.cn/vivo/param/s20pro) |
| 原抓包保留项 | V2507A | 36 | 仅作为可选模板，不作默认值 |

原有 Google Pixel 及 Samsung 型号继续保留。账号可手动配置池外型号。
屏幕、GPU 等完整 SDK 参数仍依赖部署环境的 `profiles.json` / `gpus.json`，
本机型表不等于已提供这些设备的完整真实硬件画像。

## 配置优先级

1. 账号显式设置的型号与 API。
2. 全局环境变量设置的型号与 API。
3. 自动从池中选择相匹配的组合。

仅限制 API、且池内没有该 API 预设时，保留这个显式 API，从池中选择型号，
不会再统一回退为 V2507A。token-only 调用按调用者/令牌种子稳定选择型号，
请求头与设备检查使用同一套选择逻辑。
