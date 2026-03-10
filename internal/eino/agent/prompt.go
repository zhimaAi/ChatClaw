package agent

import (
	"fmt"
	"runtime"
	"time"

	"chatclaw/internal/services/i18n"
	"chatclaw/internal/services/toolchain"
)

func isZhCN() bool {
	return i18n.GetLocale() == i18n.LocaleZhCN
}

// buildCorePrompt generates the environment section: OS, shell, directories, time.
func buildCorePrompt(homeDir, workDir, sessionsDir string) string {
	osName := runtime.GOOS
	shell := "/bin/bash"
	switch osName {
	case "windows":
		shell = "powershell"
	case "darwin":
		shell = "/bin/zsh"
	}

	zh := isZhCN()
	now := time.Now().Format("2006-01-02 (Monday) 15:04")

	var prompt string
	if zh {
		prompt = fmt.Sprintf(`
# 环境信息

- 当前时间: %s
- 操作系统: %s | Shell: %s
- 主目录: %s
- **工作目录: %s**（所有文件操作默认基于此路径，使用绝对路径）
`, now, osName, shell, homeDir, workDir)
	} else {
		prompt = fmt.Sprintf(`
# Environment

- Time: %s
- OS: %s | Shell: %s
- Home: %s
- **Working directory: %s** (all file ops default to this path; use absolute paths)
`, now, osName, shell, homeDir, workDir)
	}

	if sessionsDir != "" {
		if zh {
			prompt += fmt.Sprintf(`
# 会话目录结构

每个对话都有自己独立的工作目录。当前对话的目录是: %s
父目录 (%s) 包含来自同一 AI 助手的其他对话。如果用户提到**之前对话**中的文件或工作，你可以使用 ls 和 read_file 浏览 "%s" 下的兄弟目录来定位这些文件。写入操作仍应指向当前工作目录。
`, workDir, sessionsDir, sessionsDir)
		} else {
			prompt += fmt.Sprintf(`
# Session Directory Structure

Each conversation has its own isolated working directory. The current conversation's directory is: %s
The parent directory (%s) contains sibling conversations from the same AI assistant. If the user mentions files or work from a **previous conversation**, you can use ls and read_file to browse sibling directories under "%s" to locate those files. Write operations should still target the current working directory.
`, workDir, sessionsDir, sessionsDir)
		}
	}

	return prompt
}

// buildToolsPrompt generates sandbox rules, toolchain info, dangerous command
// confirmation, and PowerShell notes.
func buildToolsPrompt(workDir string, sandboxEnabled, sandboxNetworkEnabled bool, toolchainBinDir string) string {
	zh := isZhCN()
	var prompt string

	if sandboxEnabled {
		if zh {
			networkDesc := "网络访问已**禁用**"
			if sandboxNetworkEnabled {
				networkDesc = "网络访问已**启用**"
			}
			prompt += fmt.Sprintf(`
# 沙箱模式

所有写入仅限工作目录 %s 内（读取不受限）。%s。

要点：
- 禁止全局安装（npm -g、pip --user），用本地安装或 npx/bunx
- 始终加非交互标志（--yes、-y、--default）避免 stdin 挂起
- 用 "sh script.sh" 运行脚本，不要用 "./script.sh"（无执行权限）
- 权限拒绝 = 可能在写工作目录外的路径，改用本地方案
`, workDir, networkDesc)
	} else {
			networkDesc := "Network access is **disabled**"
			if sandboxNetworkEnabled {
				networkDesc = "Network access is **enabled**"
			}
			prompt += fmt.Sprintf(`
# Sandbox Mode

All writes restricted to working directory %s (reads unrestricted). %s.

Key rules:
- No global installs (npm -g, pip --user) — use local installs or npx/bunx
- Always pass non-interactive flags (--yes, -y, --default) to avoid stdin hangs
- Run scripts with "sh script.sh", not "./script.sh" (no execute permission)
- Permission denied = likely writing outside working dir; use local alternative
`, workDir, networkDesc)
		}
	}

	if zh {
		prompt += `
# 危险命令确认

破坏性命令（rm -rf、sudo、mkfs、shutdown、kill -9、chmod -R 777 等）执行前**必须**先调用 confirm_execution，等用户确认后再 execute。
`
	} else {
		prompt += `
# Dangerous Command Confirmation

Destructive commands (rm -rf, sudo, mkfs, shutdown, kill -9, chmod -R 777, etc.) **must** call confirm_execution first, then execute only after user confirms.
`
	}

	if runtime.GOOS == "windows" {
		if zh {
			prompt += `
# PowerShell 注意事项

- 使用分号链接命令: "cd C:\path; command"（不要使用 "&&"，它需要 PowerShell 7+）
- 使用 ".\" 前缀运行当前目录中的可执行文件: ".\app.exe"（不要使用 "app.exe"）
- 每次 execute 调用时工作目录会重置 — 在特定目录中运行命令时始终使用 "cd targetDir; command"
`
		} else {
			prompt += `
# PowerShell Notes

- Use semicolons to chain commands: "cd C:\path; command" (NOT "&&" which requires PowerShell 7+)
- Run executables in current directory with ".\" prefix: ".\app.exe" (NOT "app.exe")
- The working directory resets for each execute call — always use "cd targetDir; command" when running commands in a specific directory
`
		}
	}

	if toolchainBinDir != "" {
		installed := toolchain.InstalledSnapshot()
		var tools []string
		if installed["uv"] {
			tools = append(tools, "uv")
		}
		if installed["bun"] {
			tools = append(tools, "bun")
		}

		if len(tools) > 0 {
			toolList := fmt.Sprintf("%v", tools)
			if zh {
				prompt += fmt.Sprintf(`
# 预装开发工具（%s，位于 %s）

- Python 任务优先用 uv（替代 python/pip），JS/TS 任务优先用 bun（替代 node/npm）
- 这些工具由应用管理，保证可用，不要要求用户另行安装
`, toolList, toolchainBinDir)
			} else {
				prompt += fmt.Sprintf(`
# Pre-installed Tools (%s, in %s)

- Prefer uv for Python (over python/pip), bun for JS/TS (over node/npm)
- These are managed by the app and guaranteed available — do not ask user to install separately
`, toolList, toolchainBinDir)
			}
		}
	}

	return prompt
}

// buildSubAgentPrompt generates orchestration guidance for the lead agent.
// Since the lead agent only has read-only tools, delegation is architecturally enforced.
func buildSubAgentPrompt() string {
	if isZhCN() {
		return `
# 任务委派

你是任务编排者。你只有只读工具（read_file、ls），所有执行类操作必须通过子代理完成。

## 可用子代理

### general_purpose（执行代理）
适用于：调研、搜索、写代码、文件操作、分析、多步执行 — 任何非琐碎任务。
- 拥有完整工具集：web_search、write_file、edit_file、execute、glob、grep 等
- 给它完整的任务描述（包含背景、目标、工作目录路径），因为它看不到你的对话历史

### bash（终端代理）
仅适用于：纯 bash 命令序列（git、npm、docker、构建/测试/部署）。
- 只有 execute、ls、read_file、write_file、edit_file
- **没有搜索、没有 web_search、没有 glob/grep**

## 选择规则（严格遵守）
- 需要搜索/调研 → general_purpose（bash 没有搜索能力）
- 需要写代码/编辑文件 → general_purpose
- 只需要运行命令 → bash
- 不确定时 → general_purpose（它能做 bash 能做的一切）

## 你自己可以做的
- **纯文本对话**：回答问题、翻译、解释概念
- **读文件**：用 read_file 快速查看文件内容
- **看目录**：用 ls 查看目录结构

## 编排规则
- **有依赖的任务串行**：等前一个子代理返回后再调用下一个，把前一步结果作为上下文传入。例："调研X再写文章" → 先调研，拿到结果后再委派写作
- **无依赖的任务可并行**：完全独立的子任务可同时委派多个 general_purpose
- 子代理返回后，综合结果再回复用户
`
	}
	return `
# Task Delegation

You are a task orchestrator. You only have read-only tools (read_file, ls) — all execution must go through sub-agents.

## Available Sub-agents

### general_purpose (Execution agent)
Use for: research, search, coding, file operations, analysis, multi-step execution — any non-trivial task.
- Has full toolset: web_search, write_file, edit_file, execute, glob, grep, etc.
- Provide complete task descriptions (background, goal, working directory path) — it cannot see your conversation history

### bash (Terminal agent)
Use ONLY for: pure bash command sequences (git, npm, docker, build/test/deploy).
- Only has execute, ls, read_file, write_file, edit_file
- **No search, no web_search, no glob/grep**

## Selection Rules (strict)
- Needs search/research → general_purpose (bash has no search capability)
- Needs coding/file editing → general_purpose
- Only needs to run commands → bash
- When in doubt → general_purpose (it can do everything bash can)

## What you CAN do yourself
- **Pure text conversation**: Answering questions, translating, explaining concepts
- **Read files**: Use read_file to quickly view file contents
- **List directories**: Use ls to view directory structure

## Orchestration Rules
- **Dependent tasks are sequential**: Wait for previous sub-agent result before calling the next; pass prior results as context. Example: "Research X then write article" → research first, get results, then delegate writing
- **Independent tasks can be parallel**: Fully independent sub-tasks can dispatch multiple general_purpose calls simultaneously
- Synthesize sub-agent results before responding to user
`
}

// buildSkillGuidancePrompt generates a concise prompt about the skill system.
// DeerFlow-style: skills are tools (skill_list, skill_search, skill_install, skill_enable, read_skill), not a separate sub-agent.
func buildSkillGuidancePrompt() string {
	if isZhCN() {
		return `
# 技能系统

已安装的技能会自动加载到你的能力中。需要更多技能支持时，用 skill_list、skill_search、skill_install、skill_enable、read_skill 等工具搜索市场、安装并读取技能内容。复杂任务开始前优先加载相关技能。
`
	}
	return `
# Skill System

Installed skills are automatically loaded into your capabilities. When you need more skill support, use skill_list, skill_search, skill_install, skill_enable, and read_skill to search the marketplace, install, and read skill content. Load relevant skills before starting complex tasks.
`
}
