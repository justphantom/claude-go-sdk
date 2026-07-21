# claude-go-sdk 提炼执行方案

## 1. 背景与目标

lark-bridge 的 `internal/claude` 包是一个成熟的 Claude Code CLI 子进程客户端
（9 源文件 ~1043 行 + 5 测试文件 ~763 行，零第三方依赖）。目标：将其提炼为
独立 SDK `github.com/justphantom/claude-go-sdk`，供 lark-bridge 及未来项目复用。

## 2. 源码梳理评估

### 2.1 调用模型
- 每轮对话一个子进程：`claude -p --output-format stream-json --verbose
  [--permission-mode M] [--append-system-prompt S] [--resume SID] [--model M]
  [--effort E] [--settings F]`，prompt 走 stdin，stdout 输出 JSONL 事件流。
- `--verbose` 是 stream-json 的强制搭配（CLI 否则拒绝）。
- 独立进程组（`Setpgid`），取消时 SIGKILL 整组（CLI 会派生 bash/git 等孙进程）。
- 并发信号量（默认 4）；`IsReady` 用 `<cli> --version` 10s 探活。

### 2.2 事件协议（stdout JSONL）
| line type | 处理 |
|---|---|
| `system` | subtype `init`（携带 session_id/model）、`task_*`（子代理生命周期） |
| `assistant`/`user` | `message.content[]` → text/thinking/tool_use/tool_result，一行可产多事件 |
| `result` | 终态：最终答案、cost、duration、turns、token 用量；坏行有宽松二次解析兜底 |
| 未知类型 | 原样转发（前向兼容），raw 保留 |

解析失败仅记录跳过，不致错；pump 保证关闭 channel 前必发终态事件
（EventResult 或合成的 EventError）。`LineSink` 可逐行旁路原始输出（归档用）。

### 2.3 会话模型
懒会话：首轮空 SessionID，从 `system/init` 捕获 session_id 由调用方持久化，
后续轮次 `--resume`。失效会话（"No conversation found"）的重试策略在消费侧，
不进 SDK。

### 2.4 耦合点（仅 3 处，全部可机械切除）
1. `config.Claude` → SDK 自有 `Options` 结构体
2. `internal/log.Logger` + `log.Field*` 常量 → `*slog.Logger`（已确认）
3. `strutil.Truncate`（32 行）→ 内联小函数

### 2.5 不进 SDK（留在 lark-bridge）
`claudebridge` 全部（protocol.Control 翻译、斜杠命令、交互选择器、中文格式化、
归档过滤、用量记录、失效会话重试）、`router`、`usage`、`streamarchive`、
`backendrpc`、`bridgebase` 的渲染辅助。

## 3. 已确认设计决策

| 决策点 | 结论 |
|---|---|
| Event API | **导出字段**，去掉 25 个 Get* 访问器 |
| 日志 | `*slog.Logger`，nil → discard |
| go 指令 | **go 1.22**（代码无 1.22+ 特性，实现时逐符号验证） |
| 文档 | README.md + doc.go + examples/basic |

## 4. SDK API 设计

```go
package claude // 仓库根，扁平布局（对齐 opencode-go-sdk-lite）

type Options struct {
    CLIPath            string        // 默认 "claude"
    PermissionMode     string        // 默认 "acceptEdits"（空值见 §6 风险 R3）
    AppendSystemPrompt string        // 默认空
    MaxConcurrent      int           // 默认 4
    SettingsDir        string        // 默认 ~/.claude（~ 展开）
    SettingsCacheTTL   time.Duration // 0 → 默认 1h；<0 禁用缓存
    Logger             *slog.Logger  // nil → 静默
}

func New(opts Options) *Client

type RunOptions struct {
    Prompt, Directory, SessionID, Model string
    PermissionMode, EffortLevel, SettingsFile string // 每轮覆盖，空用 Client 默认
    LineSink io.Writer // 非 nil 时逐行旁路原始 stream-json
}

func (c *Client) Run(ctx context.Context, opts RunOptions) (<-chan Event, error)
func (c *Client) IsReady(ctx context.Context) error
func (c *Client) ListSettings(ctx context.Context) ([]string, error)
func ParseEvent(line string) ([]Event, error) // 源码已导出，保持

type Event struct {
    Type, Subtype, SessionID, Model, Text     string
    ToolID, ToolName, ToolInput               string
    IsToolError                               bool
    TaskID, TaskType, TaskKind, TaskDesc      string
    TaskTokens, TaskSteps                     int
    TaskMs                                    int64
    Result                                    string
    CostUSD                                   float64
    DurationMs                                int64
    IsError                                   bool
    NumTurns, InputTokens, OutputTokens       int
    CacheRead, CacheCreation                  int
    Raw                                       string
}
// 事件类型常量 Event*/SubtypeInit、权限常量 PermissionMode* 原样保留
```

Run 契约：调用方必须 drain channel 至关闭；关闭前必有终态事件
（EventResult/EventError）；ctx 取消 → SIGKILL 进程组 → 合成 EventError。

## 5. 目录结构

```
go.mod                     module github.com/justphantom/claude-go-sdk, go 1.22, 零依赖
doc.go                     包注释 + 最小示例
client.go                  Options/New/Run/buildCommand（自 client.go）
stream.go                  pump/emitTerminal（原样）
event.go                   Event 导出字段 + 常量（去访问器）
event_parse.go             JSONL 解析（原样）
event_parse_content.go     content blocks 解析（原样）
settings.go                ListSettings + TTL 缓存（自 client_settings.go）
ready.go                   checkVersion（原样，backendName 参数保留）
permission.go              权限常量（原样）
*_test.go                  5 个测试文件平移适配
README.md                  安装/快速开始/事件模型/会话管理/平台约束
examples/basic/main.go     单轮 prompt 打印事件流
```

## 6. 风险与边界

- **R1 平台约束**：`Setpgid`/`syscall.Kill(-pid)` 为 Unix 专有，SDK 仅支持
  linux/darwin，README 声明；不做 Windows 抽象（无需求）。
- **R2 schema 漂移**：CLI stream-json 格式随 Claude Code 版本演进；现有
  「未知类型转发 + 宽松 result 兜底 + raw 保留」已具备前向兼容，SDK 原样继承。
- **R3 permission-mode 空值**：源码无条件传 `--permission-mode <v>`，依赖上游
  配置兜底 acceptEdits；CLI 的 `default` 模式在 `-p` 下会挂起。SDK 对策：
  Options.PermissionMode 为空时默认 `"acceptEdits"`，文档写明原因。
- **R4 破坏性预告**：导出字段后 lark-bridge 的 `claudeEvent` 接口（Get* 方法）
  不再匹配，后续迁移时需改消费代码（本轮不做，见 §8）。
- **R5 测试外部依赖**：stream_test 用 `sh`、ready_test 用 `bash`（linux 开发/
  部署环境均有）；真实 claude CLI 的集成测试用环境变量门槛，默认跳过。

## 7. 实施步骤

1. `go mod init github.com/justphantom/claude-go-sdk`（go 1.22，零依赖）
2. 平移 9 源文件：切除 3 处耦合（Options/slog/内联 truncate），其余原样
3. Event 导出字段改造：结构体、两个解析文件、event_test 同步改写
4. 平移 5 测试文件并适配；新增 Options 默认值测试、空 PermissionMode 行为测试
5. doc.go、README.md、examples/basic/main.go
6. 验证：`go build ./... && go vet ./... && go test ./...`；
   逐符号检查无 go1.23+ 标准库 API
7. 提交（2 个 commit）：
   - `Add Claude Code CLI client SDK core`（go.mod + 9 源文件 + 5 测试）
   - `Add package doc, README and basic example`

## 8. 后续（不在本轮）

lark-bridge 改用本 SDK 替换 `internal/claude`：适配 Event 字段访问、
`claudeAPI` 接口签名、日志器接入；单独提变更。
