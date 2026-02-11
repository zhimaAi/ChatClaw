package define

// AppID 用于文件系统/配置目录等"标识用途"
const AppID = "chatclaw"

// SingleInstanceUniqueID 单实例唯一标识符（反向域名格式）
const SingleInstanceUniqueID = "com.sesame.chatclaw"

// AppDisplayName 用于 UI 展示
const AppDisplayName = "ChatClaw"

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

// IsServerMode 是否为 server（HTTP）运行模式
func IsServerMode() bool {
	return RunMode == "server"
}

// IsGUIMode 是否为 GUI（桌面）运行模式
func IsGUIMode() bool {
	return RunMode == "gui"
}

// DefaultAgentPromptForLocale returns the built-in default agent prompt for the given locale.
// Used by both seed migration and runtime agent creation to keep them in sync.
func DefaultAgentPromptForLocale(locale string) string {
	if locale == "zh-CN" {
		return "你扮演一名智能问答机器人，具备专业的产品知识和出色的沟通能力\n" +
			"你的回答应该使用自然的对话方式，简单直接地回答，不要解释你的答案；\n" +
			"- 如果用户的问题比较模糊，你应该引导用户明确的提出他的问题，不要贸然回复用户。\n" +
			"- 如果关联了知识库，所有回答都需要来自你的知识库，没有关联知识库也要从正确的方向回答\n" +
			"- 你要注意在知识库资料中，可能包含不相关的知识点，你需要认真分析用户的问题，选择最相关的知识点作为回答"
	}
	return "You are an intelligent Q&A assistant with professional product knowledge and excellent communication skills.\n" +
		"Your answers should be natural and conversational, simple and direct — do not explain your reasoning;\n" +
		"- If the user's question is vague, guide them to clarify before answering.\n" +
		"- If a knowledge base is linked, all answers must come from that knowledge base; if not linked, answer from the correct direction.\n" +
		"- Note that the knowledge base may contain unrelated information — carefully analyse the user's question and select the most relevant knowledge to answer."
}
