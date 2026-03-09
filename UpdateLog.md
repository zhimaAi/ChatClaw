## Changelog

### 2026/03/06
1. **Multimodal Support**: Added image input capability to assistant and knowledge pages, with model capability checks to detect multimodal support.<br/>
2. **Model Configuration Updates**: Updated OpenAI, Anthropic, Zhipu (GLM), and Qwen model configurations with refined capabilities and new model additions.<br/>
3. **Thinking Mode Control**: Added `DisableThinking` option to `ProviderConfig` and streamlined enable-thinking logic; added toast notifications for thinking mode changes.<br/>
4. **Sandbox Security**: Implemented sensitive path protection in sandbox mode to prevent unauthorized file access.<br/>
5. **Build System Improvements**: Replaced shell commands with Go tools for directory creation, file existence checks, and platform detection in Taskfile.<br/>
6. **License Update**: Switched from GNU Affero GPL to GNU General Public License.<br/>

### 2026/03/05
1. **Multi-Agent Architecture**: Introduced Researcher, Worker, and SkillAdvisor sub-agents for enhanced task delegation and execution; implemented plan-execute subagent for complex multi-step task handling.<br/>
2. **Agent Enhancements**: Added skill marketplace prompt, current time display in tools prompt, improved plan execution parameters and error handling.<br/>
3. **Knowledge Base Navigation**: Added sidebar collapse/expand feature, auto-expand folders with children on click, improved breadcrumb visibility logic, and fixed overflow handling in tree components.<br/>
4. **Skills Page Improvements**: Enhanced skill deletion confirmation dialog, improved refresh handling and loading state.<br/>
5. **Application Icons**: Updated macOS and Windows application icons with new designs.<br/>

### 2026/03/04
1. **Task Delegation UI**: Implemented task delegation feature with new UI components and bilingual (English/Chinese) localization updates.<br/>
2. **Workspace Drawer**: Added workspace drawer for file management and environment settings with file tree depth limit hint.<br/>
3. **Skill Management Tools**: Added AI-driven tools for searching, installing, enabling, and disabling skills from the skill marketplace.<br/>
4. **Image Handling in Chat**: Added drag-and-drop and paste support for images in `ChatInputArea`; added image preview functionality.<br/>
5. **Document Viewer**: Implemented header validation for ZIP and PDF files; enhanced search functionality in `DocumentCard` and `DocumentViewer`.<br/>
6. **Folder Management**: Added move folder functionality with supporting UI components.<br/>
7. **License**: Updated license to GNU Affero General Public License v3.<br/>

### 2026/03/03
1. **Image Sending in Assistant**: Implemented image sending capability in the AI assistant with base64 encoding support.<br/>
2. **Eino Upgrade**: Upgraded Eino framework to v0.8.<br/>
3. **Agent Filesystem Backend**: Enhanced filesystem backend with structured directory management, disk backend for operations, and improved tool integration.<br/>
4. **Agent System Prompts**: Added Chinese language support for agent system prompts.<br/>
5. **Document Viewer Enhancements**: Added file-type icons and improved document tab handling with folder statistics.<br/>
6. **Unknown Tools Handler**: Added handler to manage non-existent tool calls with informative error messages.<br/>
7. **Logo / Icon Refresh**: Updated application and logo icons across multiple platforms.<br/>

### 2026/03/02
1. **Knowledge Base Folder Structure**: Added `library_folders` table and `folder_id` field to documents schema; implemented folder creation, drag-to-sort, and navigation breadcrumbs.<br/>
2. **Python Environment Toolchain**: Integrated `uv` and `bun` into the execution PATH; added support for manual installation of toolchain utilities.<br/>
3. **Skills Configuration**: Added skill settings page and skill directory configuration (stored at `$HOME/.chatclaw/skills`).<br/>
4. **Navigation Tab Improvements**: Updated assistant tab behavior to switch to an existing tab or create a new one.<br/>
5. **Bug Fixes**: Fixed message assembly format issue after chat cancellation; fixed async invocation hang in synchronous execution tools.<br/>

### 2026/03/01
1. **Built-in Provider Sync**: Implemented synchronization for built-in providers and models to keep configurations up to date.<br/>

### 2026/02/28
1. **Sandbox Environment**: Integrated `codex-cli` as the sandbox backend; isolated working directories per conversation session; added common cache directories and network access permissions.<br/>
2. **Command Execution Refactor**: Unified `execute` tool for synchronous command execution and background process management; added Windows command execution support; reduced default timeout from 60s to 30s.<br/>
3. **Version 0.2.1 Released**.<br/>
4. **Memory Pagination**: Added pagination to the memory list for improved performance with large datasets.<br/>
5. **UI Improvements**: Optimized workspace settings layout; refined `AgentSettingsDialog` component height and styling.<br/>

### 2026/02/27
1. **Long-Term Memory Module**: Added edit and delete operations for long-term memory entries; optimized memory retrieval prompts.<br/>
2. **Chat Mode Feature**: Added a dedicated chat mode that displays knowledge base search results and memory retrieval results inline in the chat.<br/>
3. **Service Code Refactoring**: Split chat-related backend code into multiple files for better maintainability.<br/>
4. **Wails Upgrade**: Updated Wails version to fix the macOS window minimize issue.<br/>
5. **Snap Window Improvements**: Enhanced top-most visible process detection, custom drag behavior, and foreground switching on both Windows and macOS.<br/>

### 2026/02/26
1. **Long-Term Memory**: Implemented the long-term memory module including memory page UI, memory search, and backend extraction logic.<br/>
2. **Snap Mode**: Added safety rules for `WakeAttached` behavior; enhanced `SnapSettings` with custom app management features.<br/>

### 2026/02/25
1. **Auto-Install Toolchain**: Added automatic installation of `bun` and `uv` tools on first launch.<br/>
2. **Custom App Adsorption**: Added support for custom application selection in snap mode; improved bundle ID validation and app activation logic; added support for `com.electron.lark`.<br/>

### 2026/02/12
1. **Version 0.1.0 Released**.<br/>
2. **Linux Server Build**: Added Linux server build support with Docker configuration.<br/>
3. **In-App Text Selection Search**: Implemented a text selection search feature with settings synchronization.<br/>
4. **Auto-Start on Windows**: Added auto-start on system login for Windows.<br/>
5. **Snap Mode Floating Button**: Implemented a draggable floating button for snap mode in the assistant page.<br/>
6. **License Added**.<br/>
