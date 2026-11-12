# 请求 Token 用量

历史请求、客户端响应和用量账本使用共享的请求用量结果。历史列表展示输入、输出、合计和来源；详情补充思考用量及上游原始计数。思考是输出的一部分，合计只计算输入加输出。

## 计数来源

- `upstream`：上游明确提供输入和输出分项。
- `mixed`：部分分项来自上游，缺失分项使用本地估算。
- `estimated`：输入和输出均使用本地 tokenizer / 估算器。文件元数据仍按共享文件处理规则计入。
- `unknown`：旧记录或未知分项没有来源证据。没有计数的界面显示 `—`，真实零显示 `0`。

`input_source`、`output_source`、`reasoning_source` 描述各分项来源。`total_source=upstream` 表示合计来自上游，`derived` 表示根据输入与输出计算。`complete` 描述请求输出是否完整结束；取消、读错误和内容规则截断时为 `false`，不表示本地估算变成了精确计数。

明确的上游输入分项已经代表上游输入，不再次叠加文件 token。未知字段才使用本地估算。负数、小数、布尔值、损坏数字字符串和超过 JavaScript 安全整数范围的值不作为计数。真实 `0` 不触发估算回退。输入、输出与显式总量冲突时保留原始证据并回退估算。

## 上游消息累计计数

最新 Android 2.7.1 抓包同时包含 `history_messages?scenario=stream_close` 的 USER 与 ASSISTANT 计数，确认可以直接结算：输入使用对应 USER 的 `accumulated_token_usage`，合计使用回复的累计值，输出为两者之差。不能把 ASSISTANT 累计值直接当作输出。

| 抓包请求 | 输入 | 输出 | 合计 |
| --- | ---: | ---: | ---: |
| DOC / HTML / TXT | 86,969 | 102 | 87,071 |
| 两张图片 | 1,953 | 576 | 2,529 |
| 纯文本 | 58 | 23 | 81 |

成功完成时查询准确的会话与回复消息 ID，沿父消息关系找到对应 USER；状态未完成、消息缺失、负差值或计数缺失时回退估算。查询沿用原账号和代理，不换号，最长 3 秒。同一回复的查询结果缓存，供 Vercel 校验、预览、最终结算复用。已有完整显式分项仍可直接使用。拿到输入和输出后跳过整段提示词、输出的本地计数，也不重复叠加引用文件 token。上游没有单独提供思考用量时，思考分项仍标记估算或未知。

配对得到的权威分项保存在 `upstream.request`；SSE 原始消息计数继续保存在 `upstream.messages[]`。单独的 SSE 累计值缺少输入基线时不足以拆出输出，继续保留原值。文件级 `token_usage` 仅用于文件计数，不冒充请求总量。

同一消息的累计快照取最高已观察值，不逐帧相加。例如首次生成到 `2413`，续写再次报告 `2413` 后增长到 `2551`，保留 `2551`。不同消息保留各自计数，通过会话历史的父消息关系确认生成范围。重新生成或换账号会重置旧生成的计数与查询器；续写保留当前生成状态。

每次请求最多保存 64 条上游消息快照，超出后标记 `truncated`。原始快照不完整时仍可采用成功配对的请求级计数；无法配对时使用估算。

## 客户端格式

保留各协议的标准用量字段，并在对应 `usage` 或 `usageMetadata` 中附加 `ds2api` 元数据。客户端可以忽略此扩展字段。

```json
{
  "usage": {
    "prompt_tokens": 11,
    "completion_tokens": 29,
    "total_tokens": 40,
    "completion_tokens_details": {"reasoning_tokens": 9},
    "ds2api": {
      "source": "upstream",
      "input_source": "upstream",
      "output_source": "upstream",
      "reasoning_source": "upstream",
      "total_source": "derived",
      "complete": true,
      "upstream": {
        "messages": [{"message_id": 2, "input_tokens": 11, "output_tokens": 29, "reasoning_tokens": 9, "accumulated_tokens": 2551}]
      }
    }
  }
}
```

上例演示存在显式分项的情况；累计标量本身不能推导这些分项。

| 协议 | 用量位置 |
| --- | --- |
| OpenAI Chat | 非流式 `usage`，流式结束帧 `usage` |
| OpenAI Responses | 完整对象和 `response.completed` 中的 `usage` |
| Claude | 非流式 `usage`，流式最终 `message_delta.usage`；起始输入计数可能仍是估算，最终帧可更新 |
| Gemini | `usageMetadata`；`candidatesTokenCount` 不含单列的 `thoughtsTokenCount`，总量不重复计入思考 |

OpenAI Responses 同时提供 `output_tokens_details.reasoning_tokens`。无法给出合理思考分项时省略该分项，并标记 `reasoning_source=unknown`。历史记录保留完整用量及来源，列表仅投影轻量数字摘要，不逐条加载消息正文。

Vercel Node 桥把同一代上游元数据通过内部鉴权的校验、用量预览和 release 接口传给 Go。Go 使用共享计算器结算，并将同一结果用于客户端、历史和账本。预览不表示客户端已经收到完整响应，最终写入证据仍遵守既有交付规则。旧桥缺少新元数据时继续使用估算。

失败请求如果已经收到计数或部分输出，历史仍保留不完整用量供排查；失败前没有生成信息时保持未知，失败归档不触发成功用量记账。

旧历史不重写用量或重复入账。关闭历史记录仍可记录独立用量账本。失败尝试、保活和修复调用延续现有账本策略，不混入最终有效调用计数。
