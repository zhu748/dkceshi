# Release 构建

`Release Artifacts` 在 Release 发布或手动指定 tag 后运行，始终检出该 tag。

- `checks` 调用 `release-checks.yml`，并行执行 Lint/行数门禁及应用/代理桥 Go 测试。
- `prepare` 执行完整 Node 测试、一次前端构建及 SDK 校验/打包，产生不可变的共享输入。
- `archives` 从 `scripts/release-targets.sh` 动态读取七个平台，在独立 runner 上构建各自的程序与代理桥；每个平台复用相同前端。
- `publish` 等待上述全部成功，再汇总归档、上传 Release、构建和验证 GHCR 镜像并生成 Docker 离线包及最终校验和。

Go 测试和 Lint 默认使用 `go env GOCACHE`，与 setup-go 一致。归档构建按平台独立缓存编译结果，缓存键包含依赖摘要及源提交；新提交恢复同平台的旧缓存，由 Go 判断哪些包需要重编译。模块和自动下载的代理桥工具链共用依赖缓存。归档已经压缩，Actions 传输不再次压缩。

本地构建全部平台仍使用 `scripts/build-release-archives.sh`；单个平台使用 `--target linux_amd64` 等目录标签。`bash scripts/release-targets.sh --json` 输出规范平台清单。每个平台仍包含程序、代理桥及许可证、前端和 Android SDK 安装脚本。

优化前 v1.0.8 约 13 分半：Lint 安装 72 秒、串行质量检查 225 秒、七平台归档 421 秒。并行构建的首轮仍需填充新平台缓存，实际耗时由 Actions 运行记录确认。
