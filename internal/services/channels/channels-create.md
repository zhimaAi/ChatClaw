# 频道（Channels）模块架构文档

## 一、整体架构概览

```
┌─────────────────────────────────────────────────────────────────────┐
│                         ChannelService                              │
│                    (对外暴露的服务层 API)                            │
├─────────────────────────────────────────────────────────────────────┤
│                            Gateway                                  │
│              (统一管理所有平台适配器的网关层)                         │
├───────────────┬───────────────┬───────────────┬────────────────────┤
│ FeishuAdapter │ DingTalkAdapter│ WeComAdapter │   ...更多平台      │
│   (飞书适配器) │  (钉钉适配器)  │  (企微适配器) │                    │
└───────────────┴───────────────┴───────────────┴────────────────────┘
         ▲                ▲              ▲
         │                │              │
         └────────────────┴──────────────┴──── PlatformAdapter 接口
```

### 文件结构

| 文件 | 职责 |
|------|------|
| `model.go` | 数据模型、DTO、常量定义 |
| `adapter.go` | 平台适配器接口与注册机制 |
| `gateway.go` | 网关层，统一管理所有平台连接 |
| `service.go` | 服务层，对外暴露 CRUD 和控制 API |
| `feishu_adapter.go` | 飞书平台具体实现 |

---

## 二、平台接入处理（Adapter 机制）

### 2.1 核心接口定义

```go
type PlatformAdapter interface {
    // 返回平台标识 (e.g. "feishu")
    Platform() string

    // 建立长连接，configJSON 包含平台特定凭证
    Connect(ctx context.Context, channelID int64, configJSON string, handler MessageHandler) error

    // 断开连接
    Disconnect(ctx context.Context) error

    // 检查连接状态
    IsConnected() bool

    // 发送消息到平台
    SendMessage(ctx context.Context, targetID string, content string) error
}
```

### 2.2 平台注册机制

采用 **工厂模式 + 全局注册表**，新平台只需在 `init()` 中注册即可：

```go
var registry = map[string]AdapterFactory{}

func RegisterAdapter(platform string, factory AdapterFactory) {
    registry[platform] = factory
}

func NewAdapter(platform string) PlatformAdapter {
    factory, ok := registry[platform]
    if !ok {
        return nil
    }
    return factory()
}
```

### 2.3 已支持的平台

| 平台标识 | 名称 | 认证方式 | 实现状态 |
|---------|------|---------|---------|
| `feishu` | 飞书 | token | ✅ 已实现 |
| `dingtalk` | 钉钉 | token | ✅ 已实现 |
| `wecom` | 企业微信 | token | ⏳ 待实现 |
| `qq` | QQ | token | ⏳ 待实现 |
| `twitter` | X(Twitter) | token | ⏳ 待实现 |

---

## 三、网关管理（Gateway）

Gateway 是所有平台连接的 **统一管理层**。

### 3.1 核心数据结构

```go
type Gateway struct {
    mu       sync.RWMutex
    adapters map[int64]PlatformAdapter  // channelID -> adapter 映射
    logger   *slog.Logger
    handler  MessageHandler             // 消息回调处理器
}
```

### 3.2 主要功能

| 方法 | 功能描述 |
|------|---------|
| `ConnectChannel()` | 创建适配器并建立连接 |
| `DisconnectChannel()` | 断开并移除适配器 |
| `IsConnected()` | 检查指定频道连接状态 |
| `GetAdapter()` | 获取频道对应的适配器（用于发送消息） |
| `StartAll()` | 启动时自动连接所有已启用的频道 |
| `StopAll()` | 停止所有活跃连接 |
| `RefreshStatuses()` | 刷新并同步所有连接状态到数据库 |

### 3.3 状态同步机制

Gateway 自动将连接状态同步到数据库：

- 连接成功 → `status = "online"`, 更新 `last_connected_at`
- 连接失败 → `status = "error"`
- 断开连接 → `status = "offline"`

---

## 四、频道状态管理

### 4.1 状态常量

```go
const (
    StatusOnline  = "online"   // 在线/已连接
    StatusOffline = "offline"  // 离线/未连接
    StatusError   = "error"    // 连接错误
)
```

### 4.2 连接类型

```go
const (
    ConnTypeGateway = "gateway"  // 长连接模式（WebSocket）
    ConnTypeWebhook = "webhook"  // Webhook 回调模式
)
```

### 4.3 状态流转图

```
创建频道 → StatusOffline
    │
    ▼
绑定 Agent → (仍为 StatusOffline)
    │
    ▼
连接频道 → StatusOnline (enabled=true)
    │
    ├─────► 连接失败 → StatusError
    │
    ▼
断开连接 → StatusOffline (enabled=false)
    │
    ▼
删除频道 → 自动断开并删除
```

---

## 五、扩展新平台指南

### 5.1 步骤一：创建适配器文件

创建 `internal/services/channels/<platform>_adapter.go`：

```go
package channels

import (
    "context"
    "encoding/json"
    "sync/atomic"
)

func init() {
    // 关键：在 init 中注册适配器
    RegisterAdapter(Platform<Name>, func() PlatformAdapter {
        return &<Name>Adapter{}
    })
}

type <Name>Config struct {
    // 平台特定的配置字段
    AppKey    string `json:"app_key"`
    AppSecret string `json:"app_secret"`
}

type <Name>Adapter struct {
    connected atomic.Bool
    channelID int64
    handler   MessageHandler
    config    <Name>Config
}

func (a *<Name>Adapter) Platform() string { return Platform<Name> }

func (a *<Name>Adapter) Connect(ctx context.Context, channelID int64, configJSON string, handler MessageHandler) error {
    // 1. 解析配置 JSON
    var cfg <Name>Config
    if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
        return err
    }

    // 2. 初始化平台 SDK
    // 3. 建立连接
    // 4. 设置消息回调

    a.connected.Store(true)
    return nil
}

func (a *<Name>Adapter) Disconnect(ctx context.Context) error {
    // 关闭连接
    a.connected.Store(false)
    return nil
}

func (a *<Name>Adapter) IsConnected() bool {
    return a.connected.Load()
}

func (a *<Name>Adapter) SendMessage(ctx context.Context, targetID string, content string) error {
    // 调用平台 API 发送消息
    return nil
}
```

### 5.2 步骤二：添加平台常量

在 `model.go` 中添加：

```go
const (
    // ...existing platforms...
    Platform<Name> = "<name>"
)
```

### 5.3 步骤三：注册平台元信息（可选）

在 `service.go` 的 `GetSupportedPlatforms()` 中添加：

```go
{ID: Platform<Name>, Name: "<显示名称>", AuthType: "token"},
```

### 5.4 完成

由于使用了 `init()` 自动注册机制，新适配器编译后自动生效，无需修改 Gateway 或 Service 层代码。

---

## 六、已实现功能清单

### 6.1 Service 层 API

| 方法 | 功能 |
|------|------|
| `ListChannels()` | 获取所有频道列表（含实时状态） |
| `GetChannelStats()` | 获取频道统计（总数/在线/离线） |
| `GetSupportedPlatforms()` | 获取支持的平台列表 |
| `CreateChannel()` | 创建新频道 |
| `UpdateChannel()` | 更新频道信息 |
| `DeleteChannel()` | 删除频道（自动断开连接） |
| `BindAgent()` | 绑定 AI Agent |
| `UnbindAgent()` | 解绑 AI Agent |
| `EnsureAgentForChannel()` | 自动创建并绑定 Agent |
| `ConnectChannel()` | 连接频道（启动网关） |
| `DisconnectChannel()` | 断开频道连接 |
| `RefreshChannels()` | 刷新所有频道状态 |

### 6.2 飞书适配器功能

| 功能 | 说明 |
|------|------|
| WebSocket 长连接 | 使用飞书 SDK 官方 WebSocket 模式 |
| 消息接收 | 监听 `P2MessageReceiveV1` 事件 |
| 消息去重 | 基于 `message_id` 去重（5分钟 TTL） |
| 发送者过滤 | 自动过滤机器人自己的消息 |
| 用户名解析 | 通过 Contact API 获取发送者名称（带缓存） |
| 群聊名解析 | 通过 IM API 获取群聊名称（带缓存） |
| 消息发送 | 支持向个人(`open_id`)和群聊(`chat_id`)发送 |

### 6.3 数据模型

```go
type Channel struct {
    ID              int64      // 主键
    Platform        string     // 平台标识
    Name            string     // 频道名称
    Avatar          string     // 头像
    Enabled         bool       // 是否启用
    ConnectionType  string     // 连接类型 (gateway/webhook)
    ExtraConfig     string     // 平台特定配置(JSON)
    AgentID         int64      // 关联的 AI Agent ID
    Status          string     // 连接状态
    LastConnectedAt *time.Time // 最后连接时间
    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

---

## 七、消息流转流程

```
[平台消息]
    │
    ▼
[PlatformAdapter.onMessageReceive]
    │
    ├── 过滤非用户消息
    ├── 消息去重检查
    ├── 解析发送者/群聊信息
    │
    ▼
[MessageHandler 回调]
    │
    ▼
[IncomingMessage 结构体]
    ├── ChannelID   (频道ID)
    ├── Platform    (平台标识)
    ├── MessageID   (消息ID)
    ├── SenderID    (发送者ID)
    ├── SenderName  (发送者名称)
    ├── ChatID      (群聊ID)
    ├── ChatName    (群聊名称)
    ├── Content     (消息内容)
    ├── MsgType     (消息类型)
    └── RawData     (原始数据)
    │
    ▼
[业务层处理（AI Agent）]
    │
    ▼
[PlatformAdapter.SendMessage]
    │
    ▼
[回复到平台]
```

---

## 八、关键设计特点

1. **松耦合设计**：通过接口抽象和工厂模式，新平台接入无需修改核心代码
2. **自动注册**：适配器在 `init()` 中自动注册，编译即生效
3. **状态同步**：Gateway 自动维护数据库与运行时状态一致性
4. **优雅启停**：支持 `StartAll()` / `StopAll()` 批量管理
5. **缓存优化**：用户名/群聊名等高频查询带本地缓存
6. **消息去重**：防止平台重复投递导致的重复处理

---

## 九、配置示例

### 飞书配置 (ExtraConfig JSON)

```json
{
  "app_id": "cli_xxxxxxxx",
  "app_secret": "xxxxxxxxxxxxxxxx"
}
```

### 钉钉配置（已实现，Stream 长连接）

```json
{
  "app_id": "xxxxxxxx",
  "app_secret": "xxxxxxxxxxxxxxxx"
}
```
（与飞书一致使用 app_id/app_secret 字段名，对应钉钉 ClientID/ClientSecret）

---

## 十、依赖

- `github.com/larksuite/oapi-sdk-go/v3` - 飞书官方 SDK
- `github.com/open-dingtalk/dingtalk-stream-sdk-go` - 钉钉 Stream 官方 SDK
- `github.com/uptrace/bun` - 数据库 ORM
- `chatclaw/internal/sqlite` - SQLite 封装
- `chatclaw/internal/errs` - 错误处理
