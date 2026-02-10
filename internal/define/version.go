package define

// Version 应用版本号
// 开发模式下为 "dev"，生产构建时通过 ldflags 注入实际版本
// 构建命令示例: go build -ldflags="-X willclaw/internal/define.Version=1.0.0"
var Version = "dev"
