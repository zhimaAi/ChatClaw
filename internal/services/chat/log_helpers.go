package chat

import (
	"strings"

	"github.com/cloudwego/eino/schema"
)

// llmLogMaxContent is the max rune length for message content in LLM context logs.
const llmLogMaxContent = 300

// llmLogMaxOutput is the max rune length for LLM output in completion logs.
const llmLogMaxOutput = 1000

// llmLogMaxInstruction is the max rune length for instruction in LLM start logs.
const llmLogMaxInstruction = 500

// llmLogMaxToolResult is the max rune length for tool result content in logs.
const llmLogMaxToolResult = 500

func truncateRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "...(truncated)"
}

func summarizeMessagesForLog(messages []*schema.Message, maxMsgs int, maxContent int) string {
	if len(messages) == 0 {
		return ""
	}
	start := 0
	if maxMsgs > 0 && len(messages) > maxMsgs {
		start = len(messages) - maxMsgs
	}
	var b strings.Builder
	for i := start; i < len(messages); i++ {
		m := messages[i]
		b.WriteString("[")
		b.WriteString(string(m.Role))
		b.WriteString("] ")
		b.WriteString(truncateRunes(m.Content, maxContent))
		if m.ToolCallID != "" {
			b.WriteString(" tool_call_id=")
			b.WriteString(m.ToolCallID)
		}
		if m.Name != "" {
			b.WriteString(" name=")
			b.WriteString(m.Name)
		}
		if i != len(messages)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
