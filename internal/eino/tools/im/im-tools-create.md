# 添加新的 Agent Tool 流程

本文档描述如何为 ChatClaw Agent 添加新的工具（以 IM 发送类工具为例，如 `feishu_sender`、`wecom_sender`）。

## 目录结构

```
internal/eino/tools/
├── config.go              # 工具 ID 常量定义
├── im/
│   ├── feishu/
│   │   └── feishu_sender.go   # 飞书发送工具
│   └── wecom/
│       └── wecom_sender.go    # 企业微信发送工具
└── ...
```

## 步骤 1：定义工具 ID

在 `internal/eino/tools/config.go` 中添加工具 ID 常量：

```go
const (
    // ... 其他工具 ID
    ToolIDFeishuSender = "feishu_sender"
    ToolIDWeComSender  = "wecom_sender"
    ToolIDNewTool      = "new_tool"  // 新工具 ID
)
```

## 步骤 2：实现工具

在 `internal/eino/tools/` 下创建工具实现文件，需要实现 `tool.BaseTool` 接口：

```go
package newtool

import (
    "context"
    "encoding/json"
    "fmt"

    "chatclaw/internal/eino/tools"
    "chatclaw/internal/services/i18n"

    "github.com/cloudwego/eino/components/tool"
    "github.com/cloudwego/eino/schema"
)

// selectDesc returns i18n description based on locale
func selectDesc(eng, zh string) string {
    if i18n.GetLocale() == i18n.LocaleZhCN {
        return zh
    }
    return eng
}

// NewToolConfig configures the new tool
type NewToolConfig struct {
    // Required dependencies
    Gateway *channels.Gateway
    // Optional defaults (auto-filled from context)
    DefaultChannelID int64
    DefaultTargetID  string
}

// NewNewTool creates the tool instance
func NewNewTool(config *NewToolConfig) (tool.BaseTool, error) {
    if config == nil || config.Gateway == nil {
        return nil, fmt.Errorf("Gateway is required for new_tool")
    }
    return &newTool{
        gateway:          config.Gateway,
        defaultChannelID: config.DefaultChannelID,
        defaultTargetID:  config.DefaultTargetID,
    }, nil
}

type newTool struct {
    gateway          *channels.Gateway
    defaultChannelID int64
    defaultTargetID  string
}

type newToolInput struct {
    ChannelID int64  `json:"channel_id"`
    TargetID  string `json:"target_id"`
    Content   string `json:"content"`
}

// Info returns tool metadata (name, description, parameters)
func (t *newTool) Info(_ context.Context) (*schema.ToolInfo, error) {
    return &schema.ToolInfo{
        Name: tools.ToolIDNewTool,
        Desc: selectDesc(
            "English description of the tool",
            "工具的中文描述",
        ),
        ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
            "channel_id": {
                Type:     schema.Integer,
                Desc:     selectDesc("Channel ID", "渠道 ID"),
                Required: true,
            },
            "target_id": {
                Type:     schema.String,
                Desc:     selectDesc("Target ID", "目标 ID"),
                Required: true,
            },
            "content": {
                Type:     schema.String,
                Desc:     selectDesc("Message content", "消息内容"),
                Required: true,
            },
        }),
    }, nil
}

// InvokableRun executes the tool logic
func (t *newTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
    var in newToolInput
    if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
        return "", fmt.Errorf("parse arguments: %w", err)
    }

    // Apply defaults if not provided
    if in.ChannelID <= 0 && t.defaultChannelID > 0 {
        in.ChannelID = t.defaultChannelID
    }
    if in.TargetID == "" && t.defaultTargetID != "" {
        in.TargetID = t.defaultTargetID
    }

    // Validate required fields
    if in.ChannelID <= 0 {
        return "Error: channel_id is required", nil
    }

    // Execute tool logic...
    // Return success message or error string (not error type)
    return "Success message", nil
}
```

### 关键点

1. **Info() 方法**：返回工具名称、描述、参数 schema，LLM 依赖这些信息决定何时/如何调用工具
2. **InvokableRun() 方法**：实际执行逻辑，接收 JSON 参数字符串
3. **错误处理**：业务错误返回 `("Error: ...", nil)`，让 Agent 继续；系统错误返回 `("", err)`
4. **i18n 支持**：使用 `selectDesc()` 根据语言环境返回中/英文描述

## 步骤 3：注册到 Agent

在 `internal/services/chat/generation.go` 中注册工具：

### 3.1 添加导入

```go
import (
    // ... 其他导入
    newtool "chatclaw/internal/eino/tools/path/to/newtool"
)
```

### 3.2 在 buildExtraToolsAndHandlers 中创建并添加工具

```go
func (s *ChatService) buildExtraToolsAndHandlers(ctx context.Context, gc *generationContext, agentConfig einoagent.Config) ([]tool.BaseTool, []adk.ChatModelAgentMiddleware) {
    var extraTools []tool.BaseTool
    var extraHandlers []adk.ChatModelAgentMiddleware

    // ... 其他工具 ...

    // 添加新工具（需要 gateway 的情况）
    if s.gateway != nil {
        chID, tgtID, hasChannelSource := s.resolveChannelSource(ctx, gc.db, gc.conversationID)

        // 创建新工具
        newToolCfg := &newtool.NewToolConfig{Gateway: s.gateway}
        if hasChannelSource {
            newToolCfg.DefaultChannelID = chID
            newToolCfg.DefaultTargetID = tgtID
        }
        newToolInstance, toolErr := newtool.NewNewTool(newToolCfg)
        if toolErr != nil {
            s.app.Logger.Warn("[chat] failed to create new_tool", "error", toolErr)
        } else {
            extraTools = append(extraTools, newToolInstance)
            s.app.Logger.Info("[chat] new_tool added", "default_channel", newToolCfg.DefaultChannelID)
        }
    }

    return extraTools, extraHandlers
}
```

### 3.3 不依赖 gateway 的工具

如果工具不需要 gateway（如纯功能性工具），直接在函数开头添加：

```go
func (s *ChatService) buildExtraToolsAndHandlers(...) (...) {
    var extraTools []tool.BaseTool

    // 无依赖的工具直接创建
    myTool, err := newtool.NewMyTool(&newtool.Config{})
    if err == nil {
        extraTools = append(extraTools, myTool)
    }

    // ...
}
```

## 步骤 4：验证

```bash
# 编译检查
go build ./internal/services/chat/...

# 运行应用后，在日志中确认工具已加载
# [chat] new_tool added default_channel=xxx
```

## 参考实现

- 飞书发送工具：`feishu/feishu_sender.go`
- 企业微信发送工具：`wecom/wecom_sender.go`
- 注册位置：`internal/services/chat/generation.go` → `buildExtraToolsAndHandlers()`
