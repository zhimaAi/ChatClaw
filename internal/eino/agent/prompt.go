package agent

import (
	"fmt"
	"runtime"

	"chatclaw/internal/services/i18n"
	"chatclaw/internal/services/toolchain"
)

func isZhCN() bool {
	return i18n.GetLocale() == i18n.LocaleZhCN
}

// buildFilesystemSystemPrompt generates a system prompt that tells the LLM about
// the OS environment, working directory, sandbox constraints, and available tools.
func buildFilesystemSystemPrompt(homeDir, workDir, sessionsDir, toolchainBinDir string, sandboxEnabled, sandboxNetworkEnabled bool) string {
	osName := runtime.GOOS
	shell := "/bin/bash"
	switch osName {
	case "windows":
		shell = "powershell"
	case "darwin":
		shell = "/bin/zsh"
	}

	zh := isZhCN()

	var prompt string
	if zh {
		prompt = fmt.Sprintf(`
# 文件系统与执行工具 — 环境信息

- 操作系统: %s
- Shell: %s
- 用户主目录: %s
- **工作目录: %s**
- 所有工具使用操作系统的绝对路径。
- 当用户提到"工作目录"或要求写入/创建文件时，**始终使用工作目录**作为基础路径。例如: write_file(file_path="%s/foo.txt"), ls(path="%s")。
- 当用户提到"用户目录"或"主目录"时，指的是: %s
`, osName, shell, homeDir, workDir, workDir, workDir, homeDir)
	} else {
		prompt = fmt.Sprintf(`
# Filesystem & Execute Tools — Environment Info

- Operating System: %s
- Shell: %s
- Home directory: %s
- **Working directory: %s**
- All tools use real OS absolute paths.
- When the user mentions "working directory" or asks to write/create files, **always use the working directory** as the base path. For example: write_file(file_path="%s/foo.txt"), ls(path="%s").
- When the user mentions "user directory" or "home directory", it refers to: %s
`, osName, shell, homeDir, workDir, workDir, workDir, homeDir)
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

	if sandboxEnabled {
		if zh {
			networkDesc := "网络访问已**禁用**。curl、npm install、pip install 等命令将无法使用。"
			if sandboxNetworkEnabled {
				networkDesc = "网络访问已**启用**（例如 npm install、curl、pip install 等命令可以正常使用）。"
			}
			prompt += fmt.Sprintf(`
# 沙箱模式

你正在操作系统级沙箱中运行。在选择命令**之前**请了解以下限制:

## 写入限制
- 所有写入操作仅限于工作目录: %s
- write_file、edit_file、patch_file 只能写入工作目录内的路径。
- execute 以工作目录为 cwd 运行命令；写入工作目录之外的路径将被操作系统拒绝。
- read_file、ls、glob、grep 可以读取文件系统上的任何路径（读取不受限制）。

## 网络
- %s

## 沙箱最佳实践
- **不要使用全局安装**（例如 "npm install -g"、"pip install --user"）。全局路径在工作目录之外，写入将被拒绝。请使用本地/项目级安装（例如在项目目录中使用 "npm install"、"pip install --target ."）。
- **使用 npx / bunx** 运行 CLI 工具，无需全局安装（例如 "npx create-vue@latest my-app" 而不是全局安装 @vue/cli）。
- **始终传递非交互标志**以避免命令在 stdin 上挂起: 使用 "--yes"、"--default"、"-y"，或根据需要使用管道 "echo"（例如 "npx create-vue@latest my-app --default"、"npm init -y"）。
- **所有项目文件必须在工作目录内创建。** 不要尝试在其他地方创建文件。
- 如果命令因权限被拒绝而失败，可能是在尝试写入工作目录之外的路径。请使用本地/项目范围的替代方案重试。
`, workDir, networkDesc)
		} else {
			networkDesc := "Network access is **disabled** for executed commands. Commands like curl, npm install, pip install will fail."
			if sandboxNetworkEnabled {
				networkDesc = "Network access is **enabled** for executed commands (e.g. npm install, curl, pip install will work)."
			}
			prompt += fmt.Sprintf(`
# Sandbox Mode

You are running inside an OS-level sandbox. Understand these constraints **before** choosing commands:

## Write Restrictions
- All write operations are restricted to the working directory: %s
- write_file, edit_file, patch_file can only write to paths within the working directory.
- execute runs commands with the working directory as cwd; writing to paths outside it will be denied by the OS.
- read_file, ls, glob, grep can read any path on the filesystem (read is unrestricted).

## Network
- %s

## Best Practices in Sandbox
- **Never use global installs** (e.g. "npm install -g", "pip install --user"). Global paths are outside the working directory and writes will be rejected. Use local/project-level installs instead (e.g. "npm install" in the project directory, "pip install --target .").
- **Use npx / bunx** to run CLI tools without global installs (e.g. "npx create-vue@latest my-app" instead of installing @vue/cli globally).
- **Always pass non-interactive flags** to avoid commands hanging on stdin: use "--yes", "--default", "-y", or pipe "echo" as needed (e.g. "npx create-vue@latest my-app --default", "npm init -y").
- **All project files must be created inside the working directory.** Do not attempt to create files elsewhere.
- If a command fails due to permission denied, it is likely trying to write outside the working directory. Retry with a local/project-scoped alternative.
`, workDir, networkDesc)
		}
	}

	if osName == "windows" {
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
		var toolSections string

		if installed["uv"] {
			if zh {
				toolSections += `
## uv — 快速 Python 包管理器与运行器
- **始终优先使用 uv 而非系统 python/pip/pip3/python3。** 即使用户已安装 Python，也使用 uv 以获得更好的可复现性和速度。
- 创建新 Python 项目: ` + "`uv init my-project`" + `
- 运行 Python 脚本（自动安装依赖）: ` + "`uv run script.py`" + `
- 添加依赖: ` + "`uv add requests`" + `
- 创建虚拟环境: ` + "`uv venv`" + `
- 从 requirements.txt 安装: ` + "`uv pip install -r requirements.txt`" + `
`
			} else {
				toolSections += `
## uv — Fast Python Package Manager & Runner
- **Always prefer uv over system python/pip/pip3/python3.** Even if the user has Python installed, use uv for better reproducibility and speed.
- Create a new Python project: ` + "`uv init my-project`" + `
- Run a Python script (auto-installs dependencies): ` + "`uv run script.py`" + `
- Add a dependency: ` + "`uv add requests`" + `
- Create a virtual environment: ` + "`uv venv`" + `
- Install from requirements.txt: ` + "`uv pip install -r requirements.txt`" + `
`
			}
		}

		if installed["bun"] {
			if zh {
				toolSections += `
## bun — 快速 JavaScript 运行时与包管理器
- **始终优先使用 bun 而非系统 node/npm/npx。** 即使用户已安装 Node.js，也使用 bun 以获得更快的执行和安装速度。
- 初始化项目: ` + "`bun init`" + `
- 安装依赖: ` + "`bun install`" + `
- 运行脚本: ` + "`bun run script.ts`" + `（原生支持 TypeScript）
- 添加依赖: ` + "`bun add express`" + `
- 执行包二进制文件: ` + "`bunx create-vite my-app`" + `
`
			} else {
				toolSections += `
## bun — Fast JavaScript Runtime & Package Manager
- **Always prefer bun over system node/npm/npx.** Even if the user has Node.js installed, use bun for faster execution and installs.
- Initialize a project: ` + "`bun init`" + `
- Install dependencies: ` + "`bun install`" + `
- Run a script: ` + "`bun run script.ts`" + ` (supports TypeScript natively)
- Add a dependency: ` + "`bun add express`" + `
- Execute a package binary: ` + "`bunx create-vite my-app`" + `
`
			}
		}

		if toolSections != "" {
			if zh {
				prompt += fmt.Sprintf(`
# 预装开发工具

以下工具已**预装并在 PATH 中**（位于 %s）。你可以直接按名称调用它们。
%s
## 重要提示
- 这些工具由应用程序管理，保证可用。不要要求用户安装 Python、Node.js、pip 或 npm — 请使用 uv 和 bun。
- 如果任务需要 Python 工作，默认使用 uv。如果需要 JavaScript/TypeScript 工作，默认使用 bun。
`, toolchainBinDir, toolSections)
			} else {
				prompt += fmt.Sprintf(`
# Pre-installed Development Tools

The following tools are **pre-installed and already on PATH** (in %s). You can call them directly by name.
%s
## Important
- These tools are managed by the application and guaranteed to be available. Do NOT ask the user to install Python, Node.js, pip, or npm — use uv and bun instead.
- If a task requires Python work, default to uv. If it requires JavaScript/TypeScript work, default to bun.
`, toolchainBinDir, toolSections)
			}
		}
	}

	return prompt
}
