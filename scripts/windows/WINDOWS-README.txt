DS2API Windows 使用说明

启动与升级
1. 将整个 ZIP 解压到可写目录，保留 static、scripts 等子目录。
2. 首次配置可将 config.example.json 复制为 config.json，填写自己的账号与 API Key；
   也可以先启动，通过管理界面导入配置。示例账号和 Key 需要替换。
3. 双击 start.bat，保持终端窗口开启。
4. 默认管理地址：http://127.0.0.1:5001/admin
   未配置管理密码时，初始随机密码显示在终端中，登录后请修改。
5. 客户端 Base URL：http://127.0.0.1:5001/v1
   停止服务请按 Ctrl+C。端口可通过 PORT 调整。
6. 升级前备份已有 config.json、.env 和整个 data 目录；将这些文件复制到新目录，
   保留原账号和设备档案。不要用 config.example.json 覆盖已有配置。

SDK 自动生成设备 ID
1. 安装 Node.js 22+。本包附有 scripts 下的四个生成脚本，不附独立 SDK 原始资源。
2. 把 SDK 文件直接放入 ds2api.exe 同级目录下的 data\devid\：
   fp-1.min.js、browser_env.js、profiles.json、gpus.json、fonts.json、patches.json、
   verify.js、public_key。避免多套一层 devid 或 data 目录。
3. 设置中心 → 运行时设置 → 设备 ID 来源，选择 auto · SDK 自动生成，保存设置。
   默认 manual 表示使用手工设备 ID 池；仅放入 SDK 文件不会改变模式。
4. 返回账号管理，刷新 Token。需要生成时会先运行 SDK，再登录账号。
5. 登录设备 ID 用于管理 SDK ID 或手工 ID 池；Android 设备资料用于编辑型号、
   Android API 和安装身份。它们分别有独立入口。

故障检查
双击 check-device-sdk.bat，执行不提交设备档案的本地自检。
程序现在会显示脚本返回的错误详情、缺失资源名称和查找目录。
自定义资源目录可用 DS2API_DEVID_DIR；通过系统环境变量设置时，自检脚本也会使用该值。
如果只在 .env 中设置了自定义目录，手动自检前请在终端设置同一个环境变量。
Node 不在 PATH 时，可通过 DS2API_DEVID_NODE 指向 node.exe 的绝对路径。
Vercel 部署仅支持 manual；本地 Windows 可以使用 auto。

.env.example 中 /app 开头的路径是容器参考；在 Windows 启用 .env 前请将
DS2API_CONFIG_PATH 改为 config.json，并修改示例管理密码。
更多设备资源说明见 ANDROID-DEVID-README.md。

本包由 Linux 交叉编译为 Windows EXE，包含已构建的管理网页。
构建校验不等同于 Windows 真机运行验证。
