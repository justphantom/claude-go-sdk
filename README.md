# claude-go-sdk

Go SDK：将 Claude Code CLI 包装为子进程后端。每轮对话启动一个
`claude -p --output-format stream-json --verbose` 子进程，prompt 走 stdin，
stdout 的 JSONL 事件流解析为 `Event` 通道。零第三方依赖，仅标准库。

## 安装

```sh
go get github.com/justphantom/claude-go-sdk
```

要求：已安装并登录 Claude Code CLI（≥ 2.x），仅支持 linux/darwin。

## 快速开始

```go
package main

import (
	"context"
	"fmt"

	claude "github.com/justphantom/claude-go-sdk"
)

func main() {
	c := claude.New(claude.Options{})
	ch, err := c.Run(context.Background(), claude.RunOptions{Prompt: "hello"})
	if err != nil {
		panic(err)
	}
	for ev := range ch {
		if ev.Type == claude.EventResult {
			fmt.Println(ev.Result)
		}
	}
}
```

调用方必须 drain 返回的 channel 直到关闭；关闭前必有终态事件
（`EventResult` 或 `EventError`）。ctx 取消 → SIGKILL 整个进程组 →
合成 `EventError`。

## API 概览

| 符号 | 说明 |
|---|---|
| `Options` | 构造参数：CLIPath（默认 `"claude"`）、PermissionMode（默认 `"acceptEdits"`，CLI 的 `default` 模式在 `-p` 下会挂起交互提示）、AppendSystemPrompt、MaxConcurrent（≤0 → 4）、SettingsDir（默认 `~/.claude`，支持 `~` 展开）、SettingsCacheTTL（0 → 1h；<0 → 禁用缓存）、Logger（nil → 静默） |
| `RunOptions` | 单轮参数：Prompt、Directory、SessionID（非空 → `--resume`）、Model、PermissionMode / EffortLevel / SettingsFile（每轮覆盖，空用 Client 默认）、LineSink（逐行旁路原始 stream-json） |
| `Event` | 扁平事件结构体，26 个导出字段，含 `Raw`（原始行） |
| `New(opts Options) *Client` | 构造客户端 |
| `(*Client) Run(ctx, RunOptions) (<-chan Event, error)` | 启动一轮，返回事件通道 |
| `(*Client) IsReady(ctx) error` | `<cli> --version` 探活（10s 超时） |
| `(*Client) ListSettings(ctx) ([]string, error)` | 扫描 settings 目录（带 TTL 缓存） |
| `ParseEvent(line string) ([]Event, error)` | 解析单行 stream-json（回放归档用） |
| 常量 | `EventSystem/EventText/EventThinking/EventToolUse/EventToolResult/EventResult/EventError/EventTaskStarted/EventTaskProgress/EventTaskNotification`、`SubtypeInit`、`PermissionModeAcceptEdits/PermissionModePlan/PermissionModeBypassPermissions` |

## 事件模型

| line type | 处理 |
|---|---|
| `system` | subtype `init`（携带 session_id/model）、`task_*`（子代理生命周期） |
| `assistant`/`user` | `message.content[]` → text/thinking/tool_use/tool_result，一行可产多事件 |
| `result` | 终态：最终答案、cost、duration、turns、token 用量；坏行有宽松二次解析兜底 |
| 未知类型 | 原样转发（前向兼容），`Raw` 保留 |

解析失败仅记录并跳过，不致错；pump 保证关闭 channel 前必发终态事件。
`LineSink` 可逐行旁路原始输出（归档用）。

## 会话管理

懒会话：首轮 `SessionID` 留空，从 `system/init` 事件捕获 `session_id`
由调用方持久化，后续轮次经 `RunOptions.SessionID` 传 `--resume`。
失效会话（"No conversation found"）的重试策略由消费侧决定，SDK 不内置。

## 平台约束

取消依赖 `Setpgid` + `syscall.Kill(-pid)`（Unix 专有），仅支持
linux/darwin，不做 Windows 抽象。运行机器须安装 Claude Code CLI ≥ 2.x。

## 集成测试

单测默认全部本地可跑。真实 CLI 端到端测试用环境变量门槛：

```sh
CLAUDE_SDK_INTEGRATION=1 go test -run TestIntegration ./...
```

## License

MIT，见 LICENSE。
