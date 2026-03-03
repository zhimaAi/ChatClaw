package agent

import (
	"fmt"
	"runtime"

	"chatclaw/internal/services/toolchain"
)

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

	prompt := fmt.Sprintf(`
# Filesystem & Execute Tools — Environment Info

- Operating System: %s
- Shell: %s
- Home directory: %s
- **Working directory: %s**
- All tools use real OS absolute paths.
- When the user mentions "working directory" or asks to write/create files, **always use the working directory** as the base path. For example: write_file(file_path="%s/foo.txt"), ls(path="%s").
- When the user mentions "user directory" or "home directory", it refers to: %s
`, osName, shell, homeDir, workDir, workDir, workDir, homeDir)

	if sessionsDir != "" {
		prompt += fmt.Sprintf(`
# Session Directory Structure

Each conversation has its own isolated working directory. The current conversation's directory is: %s
The parent directory (%s) contains sibling conversations from the same AI assistant. If the user mentions files or work from a **previous conversation**, you can use ls and read_file to browse sibling directories under "%s" to locate those files. Write operations should still target the current working directory.
`, workDir, sessionsDir, sessionsDir)
	}

	if sandboxEnabled {
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

	prompt += fmt.Sprintf(`
# Filesystem Tools

- ls: list files in a directory (use absolute path, e.g. "%s")
- read_file: read a file from the filesystem
- write_file: write/create a file (prefer this over shell echo for creating files with code). **Default to the working directory: %s**
- edit_file: edit a file in the filesystem (string replacement based)
- patch_file: apply line-based patch operations (insert/delete/replace by line numbers). More precise than edit_file for multi-line changes.
- glob: find files matching a pattern (e.g., "%s/**/*.py")
- grep: search for text within files (supports regex, context lines, case-insensitive, output modes)

# Execute Tool (synchronous)

- **action="run"** (default): execute a shell command synchronously in the working directory (%s).
  - Returns combined stdout/stderr output with exit code.
  - Default timeout: 60s. Set `+"`timeout`"+` (max 300s) for slow commands (e.g. npm install).
  - **For short-lived commands only**: build, test, install, compile, lint, etc.
  - **Do NOT use for dev servers or long-running processes** — use execute_background to start them.
  - Avoid using cat/head/tail (use read_file), find (use glob), grep command (use grep tool).
- **action="stop"**: synchronously kill a background process by pid and wait for it to fully exit. Returns success or error if the process cannot be killed within the timeout.
  - Pass `+"`pid`"+` (from execute_background) and optional `+"`timeout`"+` (default 10s, max 300s).
  - If the process does not exit in time, an error is returned so you can take further action.
- **action="status"**: check if a background process is still alive and read its latest output.
  - Pass `+"`pid`"+` (from execute_background).

# Execute Background Tool (asynchronous, start only)

- Use **only** for **starting** long-running processes like dev servers ("npm run dev", "python manage.py runserver", etc.).
- Returns pid and initial output. The process is auto-killed after timeout (default 300s, max 600s).
- After starting a dev server, use execute with action="status" to confirm it's running and check its output.
- To stop a background process, use execute with action="stop" (do NOT use execute_background for stopping).
`, workDir, workDir, workDir, workDir)

	if osName == "windows" {
		prompt += `
# PowerShell Notes

- Use semicolons to chain commands: "cd C:\path; command" (NOT "&&" which requires PowerShell 7+)
- Run executables in current directory with ".\" prefix: ".\app.exe" (NOT "app.exe")
- The working directory resets for each execute call — always use "cd targetDir; command" when running commands in a specific directory
`
	}

	if toolchainBinDir != "" {
		installed := toolchain.InstalledSnapshot()
		var toolSections string

		if installed["uv"] {
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

		if installed["bun"] {
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

		if toolSections != "" {
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

	return prompt
}
