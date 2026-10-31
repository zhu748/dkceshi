# Android Web SDK 适配包

本包使用用户提供的数美 **Web SDK**，将浏览器模拟层改为 Android 手机环境。
`fp-1.min.js` 与原包字节相同，没有实现 APK 的原生 `SmAntiFraud` SDK，也不提供设备认证。
成功签发 device_id 仅验证数美上报链路；App 登录、聊天和上传需要用账号另行验证。

## 安装与使用

使用本轮更新后的项目或 Release 时，也可以沿用手工安装：将压缩包中的 `data/devid`
合并到项目的 `data/devid/`，覆盖资源文件但保留已有 `profiles/` 目录。旧目录如有
`generate.cjs`，先将其改名备份，避免优先加载旧生成器；设置过 `DS2API_DEVID_SCRIPT`
的部署应将其指向当前 `scripts/devid-generate.cjs`。

需要 Node.js 22+，安装器另需 Python 3。推荐在项目根目录执行：

```sh
python3 scripts/install-devid-assets.py /path/to/devid-android-websdk-v1.zip
```

安装器只导入 `data/devid/` 白名单资源，不覆盖仓库脚本，不导入账号、设备档案或
已有 ID。安装前校验资源清单，并用当前仓库生成器执行不联网的 SDK 载荷自检。
可用 `--verify-only` 只做检查，或用 `--assets-dir` 指定资源目录。
更新资源时先停止正在使用这些文件的服务；安装失败会恢复已经替换的资源文件。
脚本及其依赖由仓库、各平台 Release 提供，运行文件清单为 `scripts/devid-files.json`。

项目 auto 模式会根据账号型号、Android 主版本、语言及时区创建锚定配置并调用生成器。
默认设备 ID 来源为 manual；仅复制资源不会切换模式。使用本地 SDK 时，请在
「设置中心 → 运行时设置 → 设备 ID 来源」选择「auto · SDK 自动生成」并保存，再刷新 Token。
账号管理的「登录设备 ID」入口随 manual/auto 模式打开手工池或 SDK ID 清单；
「Android 设备资料」用于型号和安装身份，不能替代登录设备 ID 来源设置。
本包不包含账号、密码、token、已生成的 ID 或设备存档。Go 编译不会打包 Node 和这些文件，
运行时需要同时提供；默认 Docker 镜像保持精简；需要自动生成时使用下方 `runtime-with-sdk` 构建目标。
如果配置了 `DS2API_DEVID_SCRIPT` 或另有 `data/devid/generate.cjs`，它们会覆盖仓库内置生成器，
需将覆盖项指向本包的 `scripts/devid-generate.cjs`。

`DS2API_DEVID_DIR` 同时控制 Go 与 Node 查找资源的目录，相对路径按启动工作目录解析。
`DS2API_DEVID_STATE_DIR` 可把可写档案与只读资源分离；未配置时仍使用资源目录。
修改状态目录时，先把原目录中的 `profiles/` 迁入新目录。
正常 VM 生成会先检查 profiles.json、gpus.json、fp-1.min.js、browser_env.js 和 public_key；
缺失或不可读时列出文件名与实际目录。脚本非零退出时，后台保留 stdout JSON 中的 error，
不再只显示 exit status 1。Windows 包另附 `check-device-sdk.bat` 进行不联网的本地自检。

在项目根目录执行本地自检（不联网）：

```sh
node scripts/devid-generate.cjs --selftest
```

生成并持久化设备档案（会向数美上报，不需要 DeepSeek 账号密码）：

```sh
node scripts/devid-generate.cjs --profile data/devid/profiles/test-device.json --timeout 35
```

stdout 只有一行结果 JSON，成功时包含 `device_id`、`smid_v2`、`environment=android-web-sdk`。
同一 `--profile` 复用设备种子和 smid；`--reset` 会重建。没有锚定文件时从 19 个 vivo 模板中
随机选择并保存，不设置固定 V2507A。账号已有型号继续优先使用，不会被独立示例覆盖。
`android_version` 是 Android 版本（如 15），不是 API level（如 35）。
重复上报可能得到不同的 `device_id`；项目在成功生成后保存并复用该 ID，不会在每次请求时重新生成。
上述适配和校验针对默认 VM 路径；旧 `--browser` / `--via-chromium` 实验通道未进行同等 Android 验证。

## 生成、持久化与网络边界

- `--selftest` 校验环境、清单摘要、SDK 语法，并实际运行 SDK 直到产出可解析的上报载荷；
  不联网、不保存档案，结果包含 `sdk_payload=true`。`--check-assets` 是更快的资源检查。
  计时接口检查有限、非负值，允许整数精度，避免大时间戳浮点舍入造成偶发误判。
- `--capture` 只采集载荷并写入指定的暂存档案，不联网。项目自动生成先调用这一模式，
  再由 Go 使用账号的 SOCKS5/SOCKS5h、环境代理或显式直连策略提交。上报保留 SDK 浏览器头，
  不携带 App token。独立脚本普通生成仍使用自身的 Node 网络环境。
- 资源清单校验仅针对资源字节；不会要求旧包的脚本哈希等于当前仓库脚本。
  没有清单的旧资源可继续运行，但 `assets_verified=false`，应通过安装器升级为可校验包。
- 后台就绪状态执行离线载荷检查和状态目录写入检查，缓存 10 秒；就绪不代表云端签发或
  真实账号登录已验证。过旧、不支持采集模式的覆盖脚本会在执行前拒绝。
- 服务端登录和刷新入口共用最多两个并发槽位；同账号刷新和删除共用可取消锁。档案使用完整账号
  标识的 SHA-256 文件名。旧文件名只在当前账号中不存在冲突时迁移，存在冲突时不复用。
- 重置不会预先删除旧档案。生成先写私有临时文件，确认账号设备、路由和账号版本未变化后
  替换档案并保存 ID；配置保存失败会回滚。这个回滚处理返回错误，不宣称两个文件具备
  跨进程崩溃的原子事务。设备资料变更会清除未显式替换的旧自动 ID。
- 不支持的 Android API level 明确报错；当前支持 24–37。已知 vivo 规格统一维护在
  `internal/config/android_device_specs.json`，由 Go 和 Node 共用。无显式时区偏移时按 IANA
  时区计算当前偏移；校准屏幕、GPU 覆盖值更新复用档案，并清除旧 smid。

## Docker SDK 镜像

先用安装器准备本机资源，再执行：

```sh
docker compose -f docker-compose.yml -f docker-compose.sdk.yml up -d --build
```

覆盖文件使用 `runtime-with-sdk` 目标，包含 Node 和当前生成器。资源只读挂载到
`/opt/devid-assets`；可写档案保存在独立 `devid_state` 卷。资源与档案都不会编进镜像。
进入后台把设备 ID 来源保存为 auto，SDK 未就绪时弹窗会显示原因并禁用生成按钮。

## 适配范围

- 移动 UA 使用 Chrome 的缩减形式 `Android 10; K`；实际型号和 Android 主版本放在高熵
  Client Hints 中，`platform=Android`、`mobile=true`、架构 `arm/64` 保持一致。
- 页面、Origin、Referer 使用 `chat.deepseek.com`；上报 UA、平台和 mobile 头来自相同档案。
- 触屏、coarse pointer、无 hover、空 PDF 插件/MIME 列表作为当前模拟模板。
- 不加载 Windows 字体度量或 Direct3D GPU；使用 Android 字体名称、移动 GPU 和屏幕模板。
- 语言、Accept-Language、时区同步更新；旧版没有适配版本的设备存档重建，避免沿用桌面 smid。
- 原始旧包在加载时按已知 SHA-256 适配；预先适配过的包不会重复修改。未知版本不盲目套补丁。
- 运行时检查 SDK 实际可见的值，失败时不会提交云端上报。

## 数据精度

19 个 vivo 型号代码的物理分辨率和 GPU 芯片名来自 vivo 官方参数页：X90/X100/X200 系列及
S19/S20 系列。来源链接和映射保存在 `scripts/devid-android-profile.cjs`。
这是公开规格模板；DPR、CSS 屏幕尺寸、完整 WebGL 驱动字符串、字体宽度、Canvas、存储配额、
电池和网络参数仍是模拟值，尚未用这些机型的真实 Android Chrome 抓包校准。

池外型号及尚无硬件表的型号（包括 V2507A、Pixel/Samsung）保留型号和 Android 版本，
使用明确标记的 `generic-mobile-template`，日志会提示，结果 `hardware_approximate=true`。
独立锚定 JSON 可用 `screen: [CSS宽,CSS高,DPR]`、`gpu: {vendor,renderer}` 覆盖；
也可以在扩展包 `profiles.json.modelScreens`、`gpus.json.modelGpus` 中补充真机参数。

参考：

- [Chrome 移动模拟与 Client Hints](https://developer.chrome.com/docs/chromedriver/mobile-emulation)
- [vivo X200 Pro 官方参数](https://www.vivo.com.cn/vivo/param/x200pro)
- [vivo X100 Ultra 官方参数](https://www.vivo.com.cn/vivo/param/x100ultra)

## 从原包重新构建

在包含 SDK 8 个原始文件的仓库根目录执行（需要 Python 3 和 Node.js）：

```sh
python3 scripts/build-android-devid.py --output /path/to/devid-android-websdk.zip
```

构建采用文件白名单，排除账号、anchors、profiles、生成结果和其他运行数据。
`data/devid/ANDROID-PACKAGE.json` 记录原资产与包内文件的 SHA-256，便于核对 SDK 未被替换。
