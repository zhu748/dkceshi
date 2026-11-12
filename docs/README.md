# DS2API 文档导航 | Documentation Index

语言 / Language: [中文](README.md) | [English](README.md#english)

## 中文

为减少重复维护，本仓库文档按“入口文档 + 专题文档”拆分。建议从下列顺序阅读：

1. [项目总览（README）](../README.MD)
2. [架构与目录说明](./ARCHITECTURE.md)
3. [接口文档（API）](../API.md)
4. [部署指南](./DEPLOY.md)
5. [测试指南](./TESTING.md)
6. [开发者速查](./DEVELOPMENT.md)
7. [贡献指南](./CONTRIBUTING.md)

### 专题文档

- [DS2API 项目价值说明](./project-value.md)
- [2026-11-11 管理查询、统计性能与代码职责审查](./performance-review-2026-11-11.md)
- [2026-11-11 后续审查：上传复用、聊天记录与续写释放](./performance-review-2026-11-11-round2.md)
- [2026-11-10 流式边界、工具修复与归档性能审查](./review-2026-11-10.md)
- [2026-11-10 非流式收尾、Token 计数与 JSON 性能优化](./performance-2026-11-10.md)
- [2026-10-31 项目审查与性能优化记录](./review-2026-10-31.md)
- [2026-10-31 第二阶段全面审查与修复](./review-2026-10-31-followup.md)
- [2026-10-31 代码分类与可维护性审查](./maintainability-review-2026-10-31.md)
- [2026-11-04 运行时恢复、管理台与连接复用修复](./runtime-optimization-2026-11-04.md)
- [2026-11-03 项目审查、性能测量与用量模块整理](./project-review-2026-11-03.md)
- [2026-11-04 Android 请求头与指纹逐项对齐](./android-har-alignment-2026-11-04.md)
- [2026-11-04 加密登录、MFA 与安装头补充核对](./android-login-har-2026-11-04.md)
- [API -> 网页对话纯文本兼容主链路说明](./prompt-compatibility.md)
- [Tool Calling 统一语义](./toolcall-semantics.md)
- [DeepSeek SSE 行为结构说明（逆向观察）](./DeepSeekSSE行为结构说明-2026-04-05.md)

### 文档维护约定

- 文档更新必须以实际代码实现为依据：总路由装配看 `internal/server/router.go`，协议/resource 路由看 `internal/httpapi/**/handler*.go` 与 `internal/httpapi/admin/handler.go`，配置默认值看 `internal/config/*`，模型/alias 看 `internal/config/models.go`，prompt 兼容链路看 `docs/prompt-compatibility.md` 列出的代码入口。
- `README.MD` / `README.en.md`：面向首次接触用户，保留“是什么 + 怎么快速跑起来”。
- `docs/ARCHITECTURE*.md`：面向开发者，集中维护项目结构、模块职责与调用链。
- `API*.md`：面向客户端接入者，聚焦接口行为、鉴权和示例。
- `docs/prompt-compatibility.md`：面向维护者，集中维护“API -> 网页对话纯文本上下文”的统一兼容语义；相关行为修改时必须同步更新。
- 其他 `docs/*.md`：主题化说明，避免在多个文档重复粘贴同一段内容。

---

## English

To reduce maintenance drift, docs are split into an “entry doc + topical docs” layout.

Recommended reading order:

1. [Project overview (README)](../README.en.md)
2. [Architecture and project layout](./ARCHITECTURE.en.md)
3. [API reference](../API.en.md)
4. [Deployment guide](./DEPLOY.en.md)
5. [Testing guide](./TESTING.md)
6. [Developer quick reference](./DEVELOPMENT.md)
7. [Contributing guide](./CONTRIBUTING.en.md)

### Topical docs

- [DS2API project value note](./project-value.md)
- [2026-11-11 admin query, usage performance and module review (Chinese)](./performance-review-2026-11-11.md)
- [2026-11-11 follow-up: upload reuse, history lifecycle and stream cleanup (Chinese)](./performance-review-2026-11-11-round2.md)
- [API -> pure-text web-chat compatibility pipeline](./prompt-compatibility.md)
- [Tool-calling unified semantics](./toolcall-semantics.md)
- [DeepSeek SSE behavior notes (reverse-engineered)](./DeepSeekSSE行为结构说明-2026-04-05.md)

### Maintenance conventions

- Documentation updates must be grounded in the actual implementation: root routing lives in `internal/server/router.go`, protocol/resource routes live in `internal/httpapi/**/handler*.go` and `internal/httpapi/admin/handler.go`, config defaults in `internal/config/*`, models/aliases in `internal/config/models.go`, and the prompt compatibility pipeline in the code entrypoints listed by `docs/prompt-compatibility.md`.
- `README.MD` / `README.en.md`: onboarding-oriented (“what + quick start”).
- `docs/ARCHITECTURE*.md`: developer-oriented source of truth for module boundaries and execution flow.
- `API*.md`: integration-oriented behavior/contracts.
- `docs/prompt-compatibility.md`: maintainer-oriented source of truth for the “API -> pure-text web-chat context” compatibility flow; update it whenever related behavior changes.
- Other `docs/*.md`: focused topics, avoid copy-pasting the same section into multiple files.

- [与参考项目的功能对齐及验证边界](function-alignment.md)

- [工具调用诊断与异常保留](./tool-call-diagnostics.md)

- [Token 用量与来源](token-usage.md)：上游原始计数、共享结算、客户端字段与历史请求展示。
