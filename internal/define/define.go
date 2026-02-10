package define

// AppID 用于文件系统/配置目录等“标识用途”
const AppID = "willclaw"

// SingleInstanceUniqueID 单实例唯一标识符（反向域名格式）
const SingleInstanceUniqueID = "com.sesame.willclaw"

// AppDisplayName 用于 UI 展示
const AppDisplayName = "WillClaw"

// DefaultSQLiteFileName 默认 SQLite 数据库文件名
const DefaultSQLiteFileName = "data.sqlite"

// Env / ServerURL 的默认值由编译 tag 决定（见 env_dev.go / env_prod.go）

// IsDev 是否为开发环境
func IsDev() bool {
	return Env == "development"
}

// IsProd 是否为生产环境
func IsProd() bool {
	return Env == "production"
}
