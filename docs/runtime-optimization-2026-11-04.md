# 运行时恢复、管理台与连接复用（2026-11-04）

基于 `c85008b`，将本轮审查发现的八项问题落实为实现与回归测试。

## 修复与行为

| 问题 | 处理 | 回归入口 |
| --- | --- | --- |
| Vercel 管理 API 命中 SPA HTML | 为用量、工具诊断、Mihomo API 增加 Go 重写，放在页面兜底之前；浏览器导航继续由 Go 的导航 fallback 区分 | `tests/node/runtime-resilience.test.cjs` |
| 预检首块或 JSON 读取挂起 | Go 的阻塞预检受 idle/total/context 约束；Node 预检沿用请求剩余预算。错误正文限制 64 KiB 并明确标记截断 | `completion_inspection_watch_test.go`、Node runtime/lifecycle |
| Node 网络异常跳过恢复 | 在可重放阶段将传输异常交给账号恢复；受本地及共享账号预算限制，交付答案后禁止切号重放 | Node runtime/recovery |
| 非流式替换账号准备失败后提前退出 | 共享账号重启循环继续尝试其他候选，保留文件重建、账号归属及全局切换预算 | `account_restart_regression_test.go` |
| 自动会话清理缺少结束入口 | Resolver 的 Release 对托管与直传账号均结束资源；Chat/Vercel 先消费共享 finalizer，防止重复删除。退役只删单个会话，最终结束按 single/all 执行 | Responses session cleanup、runtime lifecycle、Chat auto delete |
| 超过 JSON 检查上限丢失尾部 | 将缓存前缀和剩余 reader 一起移交；超时、错误与取消均释放 reader | Node runtime/lifecycle |
| Vercel 成功轮询不再安排下一轮 | 每次请求完成后安排下次轮询，合并重复刷新，保留退避、暂停、手动恢复与取消 | `admin-hooks.test.mjs` |
| 切换语言清空设置草稿 | 加载回调不依赖翻译函数；读取与保存绑定请求身份，来源切换或卸载取消旧操作；重复保存合并 | `admin-hooks.test.mjs` |

## 性能与维护

- Node HTTP/2 增加独占租用的空闲连接池：最多 64 条空闲连接，每身份最多 2 条，90 秒过期。连接按 origin、凭据及设备身份隔离；即使线上省略安装头，本地安装身份仍参与隔离。活跃请求各用独立连接，取消不会重置兄弟请求。错误、GOAWAY 与未完整消费的响应不回池。
- 实际 HTTP/2 服务器回归中，同一身份连续 5 次请求只建立 1 条连接；更换凭据或安装身份后分别建立新连接。此结果证明连接复用与隔离，不代表公网端到端延迟的固定降幅。
- 用量脏检查点复用条目缓冲，在同一写入锁内拍摄快照；编码/写盘结束后清除条目字符串引用。导入、清空会释放缓冲。减少稳定账本周期 flush 的重复大切片分配，代价是保留一份条目切片的容量。文件格式、原子替换、失败重试和 revision 提交规则不变。
- 将账号恢复编排从非流式执行文件移出，将设置字段映射与 React 生命周期拆开；新增文件遵守行数上限，没有扩大历史豁免。

## 本地验证结果

| 检查 | 结果 |
| --- | --- |
| `./tests/scripts/run-unit-all.sh`，以及最终 Node 全量重跑 | Go 65 个测试包通过；Node 633 项，631 通过、2 项 SDK/VM 环境测试跳过，0 失败 |
| `go test -race` | auth、completionruntime、deepseek/client、OpenAI chat、OpenAI responses、usagestats 六包通过 |
| `./scripts/lint.sh` | golangci-lint 2.11.4，0 issues |
| `./tests/scripts/check-refactor-line-gate.sh` | 838 个源码文件，无缺失或超限；未增加历史豁免 |
| `npm run build --prefix webui` | 通过；保留既有 ECharts 大 chunk 提示 |
| `git diff --check` | 通过 |

## 范围与后续

历史记录仍使用同步持久化锁，用量账本仍全量写盘。增量日志、历史单写入队列涉及恢复和错误提交模型，需独立负载测量与迁移设计；本轮没有据微基准宣称这两处已成为实际瓶颈。

本地验证不使用真实 DeepSeek 账号或线上 Vercel 部署。SDK/VM 专用回归仍取决于外部环境。生产验证应覆盖反向代理、账号限流、慢磁盘及真实上游中断。
