# 2026-11-10 追加性能优化

在上一轮流式边界和归档初始化修复的基础上，继续对非流式收尾、Token 计数和 JSON 解码测量。全部优化保留原有工具校验、数值精度和编码检查。

## 1. 删除非流式收尾的重复构建

原实现先在 `collectAttempt` 中执行一次 `BuildTurnFromCollected`，再在重试循环中用它的原始字段重建一次 turn。两次均执行输出清理、工具解析、参数归一化、Schema / 策略校验，以及整个提示词的 BPE 计数；第二次才允许 LLM 修复。

现在完整响应的收集阶段只返回原始正文、思考、引用和状态；重试循环合并思考后执行一次权威构建。中断响应仍单独构建失败 turn，保留部分输出、用量、诊断和明确错误，并禁止修复截断参数。移除了构建前累加后又被覆盖的可见思考字符串。

OpenAI、Claude、Gemini 的共享非流式链路同时受益。现有重试、换号、输出格式、工具修复和部分输出回归用例继续验证这些行为。

## 2. Token 计数使用普通文本编码

原来的 `Encode(text, nil, nil)` 未允许任何特殊 Token，所有标记本来都按普通文本计数。但依赖库仍会逐个寻找特殊标记，并多次复制和扫描剩余文本。

改用同一编码器的 `EncodeOrdinary`，避免这段特殊标记查找。增加与旧入口的计数一致性测试，包含中文、Emoji、组合字符、特殊标记、空白和异常 UTF-8。未支持模型的启发式回退保持原口径。

## 3. JSON 键名与数值的等价快速路径

- 无转义且 UTF-8 合法的键名直接复制键内容，避免每个键再次创建 JSON 解码器。带转义的键仍解码，`name` / `\u006eame` 不能绕过重复键拒绝；异常 UTF-8 也保留解码器比较语义，随后仍受编码检查约束。
- 数字经 `ParseFloat` 后，若原始文本与最短往返文本完全相同，直接返回该 float64。不同拼写继续以精确有理数比较；大整数、超精度小数、溢出与极端指数仍按原逻辑保留 `json.Number`。
- 新增负零、指数、非规范小数、超出安全整数范围、超精度小数和转义别名等边界回归。

## 基准数据

基线为 `9c65ac4` 的独立源码快照，使用同一组新增基准；优化版包含本次全部性能改动。Go 1.26.0、Linux amd64，同一环境依次测量，每个子项运行三次取中位数。包执行设置 `-p 1`，测量时不同时运行全量测试或构建。

| 场景 | 原耗时 | 优化后耗时 | 原分配内存 | 优化后分配内存 |
| --- | ---: | ---: | ---: | ---: |
| 非流式：148 KiB 提示词、短文本回复 | 64.176 ms | 32.616 ms | 36,207,560 B | 17,287,129 B |
| 非流式：148 KiB 提示词、工具回复 | 65.437 ms | 31.828 ms | 36,323,702 B | 17,343,576 B |
| 256 段字面特殊标记的 Token 计数 | 6.971 ms | 2.227 ms | 4,339,472 B | 859,385 B |
| 1024 组混合精度数值 JSON 解码 | 5.011 ms | 3.278 ms | 1,896,608 B | 978,595 B |
| 1024 个工具结果消息 JSON 解码 | 4.061 ms | 3.318 ms | 3,087,873 B | 2,203,142 B |
| 4096 个普通键名的重复键检查 | 1.263 ms | 0.656 ms | 1,157,562 B | 567,739 B |
| 无特殊标记的长文本单次 Token 计数 | 27.337 ms | 28.219 ms | 16,066,949 B | 15,329,618 B |

普通文本的单次 Token 计数在本次样本中没有计时收益；主要收益来自删除整次重复计数和避免字面特殊标记的后缀扫描。表内耗时只覆盖本地处理，不包含上游推理和真实网络延迟。

复现命令：

```bash
go test -p 1 ./internal/completionruntime ./internal/util ./internal/jsonvalue \
  -run '^$' \
  -bench 'BenchmarkNonStreamLargeContext|BenchmarkTokenizerCount|BenchmarkJSONDecode|BenchmarkDuplicateKeys' \
  -benchmem -benchtime=500ms -count=3
```

## 验证范围

针对性复核覆盖共享 completion runtime、assistant turn、OpenAI Chat / Claude / Gemini、JSON 解码、请求体检查、工具 Schema 和 PromptCompat。交付前进一步运行仓库完整 Go / Node 单元检查、静态检查、行数门禁、WebUI 构建及共享模块竞态检查。

| 检查 | 结果 |
| --- | --- |
| `./tests/scripts/run-unit-all.sh` | 通过：69 个 Go 测试包，711 个 Node 用例 |
| `./scripts/lint.sh` | 通过：0 issues，改动 Go 文件已 gofmt |
| `./tests/scripts/check-refactor-line-gate.sh` | 通过：995 个源文件，无缺失、无超限 |
| `npm run build --prefix webui` | 通过；保留既有图表分块体积提示 |
| 账号池、鉴权、SSE、stream、completionruntime、responsehistory、util、jsonvalue、toolcall、promptcompat 的 `go test -race` | 10 个包全部通过 |
| `git diff --check` | 通过 |

测试使用本地 HTTP 模拟与现有协议夹具；未使用生产账号执行真实上游生成。
