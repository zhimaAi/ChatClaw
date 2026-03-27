## Changelog

### 2026/03/27
1. **OpenClaw Runtime Install/Uninstall Optimization**: Replaced file copy with robocopy for efficiency, optimized process termination logic to prevent file lock issues, updated NSIS installer packaging format (tar/zip).
2. **OpenClaw Manager Standalone Page**: Implemented OpenClaw manager as a standalone page, unified chat assistant style in OpenClaw mode to match ChatClaw.
3. **Feishu Channel Integration**: Completed Feishu message sync functionality, support for group message replies.
4. **DingTalk Channel Integration**: Added new DingTalk Agent and DingTalk channel functionality.
5. **OpenClaw Skills Page**: Refactored `OpenClawSkillsPage.vue` component structure for Vue 3 single root element compliance, enhanced dialog for adding skills.
6. **License Update**: Restored and updated license to MTI agreement.

### 2026/03/26
1. **OpenClaw Skills Feature**: Implemented OpenClaw Skills page and backend service integration, added runtime bundling and NSIS installer support.
2. **OpenClaw Version Update**: Upgraded OpenClaw version to 2026.3.24, updated Docker command execution logic and layout configuration.
3. **Scheduled Task Channel Optimization**: Optimized scheduled task editing, added channel logic, improved immediate execution functionality, fixed required field timeout issues.
4. **History Task Optimization**: Optimized history task compatibility with task assistant conversations, list errors, and history record processing.
5. **Feishu Channel Integration**: Completed Feishu channel integration.
6. **Code Split**: Refactored localization files and MCP configuration management.

### 2026/03/25
1. **OpenClaw Mode Optimization**: Hidden thinking mode and task mode in OpenClaw mode's knowledge base interface, support for selecting knowledge base.
2. **Feishu Channel**: Completed Feishu integration, support for group message replies, optimized channel creation.
3. **Scheduled Task Refinement**: Fixed scheduled task editing and real-time channel configuration update issues, hidden CMD command popup.
4. **Image Model Error Handling**: Added image model error prompts.
5. **File Migration**: Implemented file migration functionality.
6. **History Task Compatibility**: History task compatible with task assistant conversations, fixed history conversation list loading.
7. **Agent Sync**: OpenClaw Agent streaming output, history message rendering and error handling.
8. **Upgrade Logic**: OpenClaw upgrade logic optimization.

### 2026/03/24
1. **OpenClaw Assistant Page**: Completed OpenClaw assistant page with Agent CRUD, streaming output, and chat functionality.
2. **OpenClaw Agent Streaming Rendering**: Implemented OpenClaw Agent streaming output and history message rendering.
3. **Memory Page Sync**: Memory page data synchronization optimization.
4. **OpenClaw Console**: Added OpenClaw console entry.
5. **Tools Page**: Completed tools page functionality.
6. **System Switch**: Support for system switching functionality.
7. **History Record Optimization**: Adjusted complete AI assistant in history records to read-only mode.
8. **Scheduled Task Optimization**: Optimized scheduled task list, fixed history task popup issues.

### 2026/03/23
1. **OpenClaw Agent Core Features**: Implemented OpenClaw Agent CRUD, streaming output, and chat functionality.
2. **OpenClaw Assistant Page**: Added OpenClaw assistant page.
3. **npm Global Library Copy**: Implemented npm global library copying functionality to OpenClaw bundle.
4. **Agent Sync**: OpenClaw Agent synchronization.
5. **Page Adjustments**: Various page style and layout adjustments.

### 2026/03/20
1. **Version 0.7.0 Release**: Updated application version to 0.7.0.
2. **OpenClaw Service Integration**: Integrated OpenClaw service, multi-agent configuration sync to OpenClaw.
3. **Memory System Refactoring**: Refactored memory system to be compatible with OpenClaw, switched to OpenClaw specification standards.
4. **Agent Synchronization**: OpenClaw Agent synchronization functionality.
5. **MiniMax Provider**: Added MiniMax provider integration and sync functionality.
6. **Windows Console Window Hiding**: Implemented `setCmdHideWindow` function to prevent console window from appearing on Windows when opening directories and files.
7. **Dark Mode Compatibility**: Improved dark mode compatibility across UI components.
8. **App Data Directory Refactoring**: Refactored app data directory handling to use unified `AppDataDir` function for better maintainability.
9. **UI Style Updates**: Multiple style adjustments for channel modal, scheduled task validation and styling, knowledge base page input field, folder styling, settings page, and knowledge library list.
10. **Documentation**: Completed development documentation updates.

### 2026/03/18
1. **README Refresh (Multi-language)**: Updated README files across multiple languages with new previews, clearer capability descriptions, and consistent image paths/structure.<br/>
2. **Channel Integrations & Messaging**: Improved messaging flows and guardrails for multiple channels (e.g., QQ config dedup & image sending, WeCom/Feishu streaming output, DingTalk checks and related config updates).<br/>
3. **Docs/Tooling**: Added `readme-from-docx` skill documentation for syncing README content from Word documents, including image extraction and localization guidelines.<br/>

### 2026/03/17
1. **Chat File Upload (End-to-End)**: Delivered file attachment support across chat UI and backend services, including type/size validation, message state integration, and consistent handling alongside image attachments.<br/>
2. **Chat Input UX Upgrade**: Enhanced `ChatInputArea` with a new conversation entry, dropdown integrations, a compact mode selector with icon toggles, and a bottom toolbar for better ergonomics.<br/>
3. **In-App Selection Context Menu**: Added an in-app text selection popup with actions and an option to disable selection search, with `TextSelectionService` and settings synchronization.<br/>
4. **MCP Defaults & Reliability**: Enabled MCP by default via SQLite migration and hardened MCP settings flows (tool add/remove state handling, validation, cleanup) to reduce edge-case failures.<br/>
5. **Model Config Validation**: Strengthened Azure chat/embedding configuration validation (endpoint/version requirements) and refined thinking feature initialization logic for safer defaults.<br/>
6. **Project Hygiene**: Added `think_docs/` convention + `.gitignore` entry, removed legacy Klingon locale artifacts, and applied small refactors for readability/maintainability (e.g., button handlers).<br/>

### 2026/03/16
1. **New Style / Knowledge UI Refactor**: Major refactor of `KnowledgePage` folder/library behaviors with debounced expansion, improved folder-tree synchronization, and upgraded team knowledge UI via `TeamFolderCard`/`TeamFileCard` components.<br/>
2. **Assistant MCP Feature Expansion**: Added an Assistant MCP detail view (edit + tool management) and improved service networking behavior (host binding checks, `127.0.0.1` usage, CORS/OPTIONS handling).<br/>
3. **i18n Loading Improvements**: Refactored i18n initialization to support async locale message loading and made message typing more flexible for dynamic locale structures.<br/>
4. **Performance & Build Optimizations**: Improved Vite bundling via manual chunking and async component loading in `App.vue`; added reusable `copyToClipboard` utility; tuned server-mode performance and tokenizer dictionary loading for Chinese segmentation.<br/>
5. **Build/Dev Workflow Updates**: Updated `development.md` and Dockerfiles for clearer, more reliable build steps (bindings generation, frontend deps), plus backend app init/systray refinements for stability.<br/>

### 2026/03/13
1. **Internationalization Overhaul & i18n Skill**: Added `i18n-check` skill, AI-powered translation scripts, and formatting/comparison utilities to auto-fill missing keys across frontend and backend locales, with improved key detection for CJK and multi-script strings.<br/>
2. **Locale Management Improvements**: Changed default UI language to English, added system locale detection, reworked language options and labels in settings, and cleaned up legacy languages while fixing escape and spacing issues in multiple translations.<br/>
3. **Assistant MCP & Server UX**: Refined MCP server management in `WorkspaceDrawer`, improved server selection and removal logic, and optimized how MCP servers are attached to agents and governed by global settings, along with list performance and debouncing fixes.<br/>
4. **Brand & Version Update**: Refreshed application icons and images with a new logo set and bumped application version to `0.5.0`, aligning related configuration and dependency versions.<br/>

### 2026/03/12
1. **Assistant MCP Integration**: Added Assistant MCP functionality with UI and backend support, including server control based on a global MCP setting and updated indicators to reflect MCP availability in the workspace.<br/>
2. **Rich Message Editing with Images**: Enhanced chat message editing to support attaching images and saving image payloads into the work directory, reusing the multimodal image upload pipeline for edited messages.<br/>
3. **Team Library Recall Chat**: Introduced team library recall chat support, enabling conversations that can recall and use team libraries within shared team sessions.<br/>
4. **MCP Command UX on Windows**: Implemented console window hiding for MCP command execution on Windows and registered the Windows URL scheme to improve deep-link and protocol handling.<br/>
5. **Model & Auth Tweaks**: Updated model detection to support additional Qwen types, refined ChatWiki bind/logout flows, and adjusted tool status reporting for MCP-related tools.<br/>

### 2026/03/11
1. **MCP Toolchain for Agents**: Enabled MCP tools to be directly exposed to the lead agent via `mcp__`-prefixed tools, and added per-agent MCP server enable/disable controls with quick navigation from the workspace drawer to MCP settings.<br/>
2. **Task & Cron Management**: Extended scheduled task features with execution history, improved failure statistics and success criteria, and new tools for querying runs, alongside a new task-creation dialog workflow.<br/>
3. **Toolchain Installation UX**: Added test installation features and download progress tracking for the toolchain, increased download/read timeouts for long-running installs, and improved GitHub proxy and mirror handling for version fetching.<br/>
4. **ChatWiki & Token Handling**: Improved ChatWiki token management with better reload behavior and local caching, and refined related editing tools and prompts with updated Chinese/English copy.<br/>

### 2026/03/10
1. **ChatWiki Account Binding**: Added ChatWiki account binding flow in settings (cloud/open-source selection, browser auth, deep-link callback, countdown + re-auth/unbind).<br/>
2. **ChatWiki Backend Service**: Introduced `ChatWikiService` with binding persistence, robot/library management APIs, and auth-expired handling.<br/>
3. **Team Chat Streaming**: Implemented team-mode SSE streaming with `dialogue_id` continuation support and conversation/message persistence for team sessions.<br/>
4. **DB Migrations**: Added SQLite migrations for ChatWiki binding storage and team conversation fields (`team_type`, `dialogue_id`).<br/>
5. **UI & i18n Updates**: Updated assistant/settings/knowledge pages and added new locales to support ChatWiki integration and related UI states.<br/>

### 2026/03/09
1. **Branding Assets Refresh**: Updated app icons and frontend logo assets across Windows/macOS builds and UI images.<br/>

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
