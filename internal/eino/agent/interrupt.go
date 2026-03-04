package agent

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func init() {
	schema.RegisterName[*InterruptInfo]("_chatclaw_interrupt_info")
}

// InterruptInfo is the payload attached to an interrupt signal so that
// the caller (processStream) can display a meaningful confirmation prompt.
type InterruptInfo struct {
	Command string `json:"command"`
}

// interruptApprovalOptions carries the user's approval decision when a
// dangerous command is resumed via tool.Option.
type interruptApprovalOptions struct {
	Approved *bool
}

// WithInterruptApproval creates a tool.Option that marks the dangerous command
// as approved by the user, allowing the rerun to proceed.
func WithInterruptApproval(approved bool) tool.Option {
	return tool.WrapImplSpecificOptFn(func(t *interruptApprovalOptions) {
		t.Approved = &approved
	})
}

// ConfirmExecutionInput is the input schema for the confirm_execution tool.
type ConfirmExecutionInput struct {
	Command string `json:"command" jsonschema:"description=The exact shell command that requires user confirmation before execution"`
}

// NewConfirmExecutionTool creates a tool that asks the user to confirm a
// dangerous shell command before it is executed. The AI should call this tool
// instead of directly executing commands that may be destructive.
//
// On first invocation the tool triggers an interrupt-and-rerun. When the user
// approves, the framework reruns the tool with WithInterruptApproval(true) and
// the tool returns the original command so the AI can proceed to execute it.
func NewConfirmExecutionTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"confirm_execution",
		"Call this tool BEFORE executing any potentially dangerous or destructive shell command. "+
			"Pass the exact command you intend to run. The tool will pause and ask the user for confirmation. "+
			"After the user responds, you will receive either the approved command (proceed to execute it) "+
			"or a rejection message (do NOT execute the command).",
		func(ctx context.Context, input *ConfirmExecutionInput, opts ...tool.Option) (string, error) {
			o := tool.GetImplSpecificOptions[interruptApprovalOptions](nil, opts...)
			if o.Approved != nil {
				if *o.Approved {
					return fmt.Sprintf("User approved. Proceed to execute: %s", input.Command), nil
				}
				return "User rejected the command. Do NOT execute it.", nil
			}

			return "", compose.NewInterruptAndRerunErr(&InterruptInfo{Command: input.Command})
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	return t
}

// dangerousPatterns are command substrings that should trigger confirmation.
var dangerousPatterns = []string{
	"rm -rf", "rm -r", "rmdir",
	"mkfs", "dd if=",
	"format c:", "format d:",
	":(){:|:&};:",
	"> /dev/",
	"chmod -R 777",
	"kill -9", "killall",
}

// dangerousCommands are exact first-token command names that should trigger confirmation.
var dangerousCommands = []string{
	"sudo",
	"shutdown",
	"reboot",
	"halt",
}

// IsDangerous reports whether the given shell command matches a known
// dangerous pattern. Exported for use in system prompt generation.
func IsDangerous(cmd string) bool {
	lower := strings.ToLower(cmd)
	for _, p := range dangerousPatterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	firstWord := strings.Fields(lower)
	if len(firstWord) > 0 {
		for _, c := range dangerousCommands {
			if firstWord[0] == c {
				return true
			}
		}
	}
	return false
}

// DefaultInterruptPrompt returns a generic confirmation prompt when no
// command information is available. The language matches the system locale.
func DefaultInterruptPrompt() string {
	if isZhCN() {
		return "检测到一个可能具有破坏性的操作，需要你的确认。请回复 **确认** 继续执行，或回复 **拒绝** 取消。"
	}
	return "A potentially dangerous operation requires your confirmation. Please reply **confirm** to proceed or **reject** to cancel."
}

// FormatInterruptPrompt creates the assistant message text shown to the user
// when a dangerous command is intercepted. The language matches the current
// system locale.
func FormatInterruptPrompt(info *InterruptInfo) string {
	if isZhCN() {
		return fmt.Sprintf(
			"即将执行一条可能具有破坏性的命令：\n\n```\n%s\n```\n\n请回复 **确认** 继续执行，或回复 **拒绝** 取消。",
			info.Command,
		)
	}
	return fmt.Sprintf(
		"I'm about to execute a potentially dangerous command:\n\n```\n%s\n```\n\nPlease reply **confirm** to proceed or **reject** to cancel.",
		info.Command,
	)
}
