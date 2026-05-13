## Changelog

### 2026/05/13
1. **Localization Updates**: Updated localization files for multiple languages; added new error messages for plugin installation failures in English, Spanish, and French.

### 2026/05/11
1. **Chat File Upload (Major)**: Added comprehensive file upload functionality to chat; support for multiple file types with size limits; enhanced message sending logic to include file attachments.
2. **OpenClaw Version Management**: Downgraded OpenClaw version to 2026.3.24 in runtime configuration to maintain compatibility.
3. **Build Script Cleanup**: Removed `start-chatclaw.ps1` script as it is no longer needed.

### 2026/05/09
1. **OpenClaw Factory Reset**: Implemented factory reset functionality in OpenClaw runtime settings with confirmation dialog and full localization support in English and Chinese.
2. **Development Documentation**: Updated Node.js and pnpm installation instructions; enhanced frontend testing setup with Playwright CDP debugging; modified Dockerfile for cross-compilation; streamlined Taskfile for frontend dependency management; improved global setup and teardown scripts for testing.

### 2026/05/08
1. **Frontend Build System**: Updated `.gitignore` to exclude test results and `package-lock.json`; enhanced development documentation with automated testing setup; removed `.npmrc`; updated `package.json` with new test scripts and dependencies.
2. **Vite Configuration**: Modified Vite config for better chunking and optimized bundling.
3. **WebView2 Debugging**: Enabled WebView2 debugging in Windows app initialization for improved development experience.

### 2026/05/07
1. **OpenClaw Version Rollback**: Downgraded OpenClaw version to 2026.3.24 in runtime configuration for stability.
2. **Multi-Ask Optimization**: Enhanced multi-ask functionality with improved performance and reliability.

### 2026/04/30
1. **OpenClaw Version Update**: Bumped OpenClaw version to 2026.4.27; implemented temporary workaround to clear WhatsApp channel config for startup errors to address potential gateway startup issues due to incomplete WhatsApp config validation.

### 2026/04/28
1. **OpenClaw Runtime Settings Enhancement**: Enhanced OpenClaw runtime settings and cleaned up deprecated identity fields; updated gateway connection status to include last error message during reconnection attempts.
2. **Identity Field Removal**: Removed identity handling from agent entries as OpenClaw 4.26+ no longer supports it via RPC, ensuring cleaner configuration management.
3. **Last OpenClaw Support**: Updated to use the latest OpenClaw runtime by default.

### 2026/04/24
1. **Multi-Language Localization Update**: Updated localization files for Arabic, Bengali, German, Spanish, French, Hindi, and Italian, including translations for app titles, common phrases, tool descriptions, and various UI elements.
2. **WhatsApp Channel Hotfix**: Fixed WhatsApp channel initialization error configuration issue.
3. **Skill Market Synchronization Enhancement**: Enhanced skill synchronization logic with remote metadata for improved skill data management.

### 2026/04/22
1. **ChatWiki Server URL Version Resolution**: Implemented server URL-based version resolution and enhanced binding logic for ChatWiki integration.
2. **AGENTS.md Documentation**: Updated AGENTS.md with language conventions, Codex superpowers, archiving and iteration guidelines for comprehensive AI development guidelines.

### 2026/04/21
1. **ChatWiki UI Enhancements**: Enhanced account card and provider detail components for improved ChatWiki user experience.

### 2026/04/20
1. **Model Selection Logging**: Enhanced model selection logging and decision-making for better debugging and user feedback.
2. **ChatWiki Default Model Support**: Added default use model support in ChatWiki integration.
3. **OpenClaw Runtime Update**: Updated OpenClaw version to 2026.4.15 with adjusted development environment URLs.

### 2026/04/17
1. **Skill Market Page Refactor (Major)**: Complete overhaul of `SkillMarketPage.vue` including enhanced skill loading and caching mechanisms, introduced `cachedBrowseSkills` and `browseLibraryCount` for improved performance, refactored load logic for agents and install targets with `loadAgentsWithTargets`, and improved category handling built from cached skills.
2. **Skill Market Caching & Synchronization**: Implemented a full caching and synchronization system for skill data including new database migration tables, `cache.go` and `sync.go` service modules with background sync logic, `parseFlexibleTime` function for robust multi-format time comparison, and enhanced `ScopeRoots` mapping for accurate skill file operations across agent workspaces.
3. **Skill Market Scope Support**: Enhanced skill file handling with scope-aware logic using `ScopeRoots`, refined skill filtering based on selected agent workspace scope, and updated service layer to list agents along with workspace and shared targets.
4. **Default Model Selection**: Added `SetDefaultModelDialog` component for setting agent default LLM provider and model, introduced `DefaultLLMProviderID` and `DefaultLLMModelID` fields in agent creation, and added corresponding localization strings in English and Chinese.
5. **OpenClaw Agent Synchronization**: Enhanced agent synchronization with reconciliation during Gateway connection, improved `AgentService` to ensure agents match database state, added `ensureSkillsExtraDirs` for `extraDirs` configuration management, and fixed agent creation to trigger config sync after RPC.
6. **Install Confirmation i18n**: Added `installConfirmDescription` translation key across all supported languages (en-US, ja-JP, ko-KR, zh-CN, zh-TW) with updated SkillMarketPage integration.
7. **SyncService Time Comparison**: Replaced direct time parsing with `parseFlexibleTime` for handling multiple time formats with improved robustness and whitespace trimming.

### 2026/04/16
1. **Skill Market Uninstall Confirmation**: Added dialog-based uninstall confirmation to prevent accidental removals, replaced `AlertDialog` with `Dialog` component for consistent UI, and updated state management to prevent concurrent uninstall actions.
2. **Skill Market Agent Selection Refactor**: Simplified agent selection dropdown by removing 'None' option, switched to `OpenClawAgentsService` for better integration, added method to retrieve agents by OpenClaw ID, and improved agent workspace directory resolution.
3. **Style Updates**: Applied `chatclaw` system styling to the skills navigation item in SideNav.
4. **Clawhub Registry Fix**: Updated default Clawhub registry URL from `https://clawhub.ai` to `https://cn.clawhub-mirror.com` for improved regional accessibility.

### 2026/04/15
1. **Skill Market Page (Major Feature)**: Complete Skill Market page implementation with fallback mechanism for Clawhub API requests, enhanced skill filtering logic for agent workspaces, loading indicators and hints for agent workspace selection, improved skill loading experience with updated response structure, refined skill tab labels, and skill status display with 'added' label.
2. **CLI Tools for Skill Management**: Added `clawhub` and `skillhub` command-line tools for skill management and search functionality, with default registry URL fix for Clawhub.
3. **NSIS Build System Refactor**: Refactored NSIS build process using PowerShell scripts, simplified build parameters, and enhanced extra skills handling for improved Windows installer builds.
4. **Sidebar Navigation Enhancement**: Added 'chatclaw' system to skills navigation item in SideNav for improved navigation.
5. **Development Documentation**: Updated development documentation and build configurations.

### 2026/04/14
1. **Skill Market Page (Initial Release)**: Introduced Skill Market page and related services for centralized skill discovery, browsing, and management within the application.
2. **Skill Management CLI**: Added `clawhub` and `skillhub` command-line tools for skill management and search functionality.

### 2026/04/10
1. **OpenClaw Gateway Startup Enhancements**: Implemented a new heartbeat mechanism for the Gateway offline banner to ensure responsiveness to backend status changes. Added detailed startup steps in the runtime manager for better tracking of the gateway's initialization process.
2. **Toast Duration Customization**: Introduced `TOAST_DURATION_HINT` for longer informational messages and updated toast functionality to accept custom durations.
3. **Default Configuration for New Users**: Added default configuration generation for new users to prevent startup errors when first launching the application.
4. **Default Mode for First Launch**: Enhanced first-launch experience with ChatClaw mode as the default entry point and runtime environment automatically opened.
5. **OpenClaw Auto-Start**: Implemented auto-start functionality in OpenClaw settings, allowing users to enable or disable automatic startup of the gateway with UI toggle and backend process management.
6. **Channel Auto-Close on Connection Failure**: Added automatic channel closure when connection fails to improve reliability and user feedback.
7. **Localization Updates**: Updated localization files for English, Japanese, Korean, Simplified Chinese, and Traditional Chinese to include new messages related to the gateway startup process.
8. **README Multi-Language Enhancement**: Revised readme-from-docx skill documentation and updated README files across multiple languages with improved clarity and structure.
9. **i18n Rule Refactor**: Removed double quotes locale rule and introduced single quotes locale rule for consistent string escaping in TypeScript files.
10. **Style & UI Refinements**: Various text adjustments, component extraction, and UI polish improvements.

### 2026/04/09
1. **OpenClaw Auto-Start Feature**: Implemented auto-start feature in OpenClaw settings, allowing users to enable or disable automatic startup of the gateway with improved process management on Windows.
2. **Gateway Status Race Condition Fix**: Introduced a 4-second delay before checking the gateway port after process exit to prevent race conditions during the gateway restart process.
3. **Gateway Restart Logic Refinement**: Replaced gateway restart logic with explicit stop and start commands using ExecCLI for improved control over the gateway lifecycle.
4. **Channel Integration Optimizations**: Enhanced Feishu (Lark) channel integration with improved message hook functionality; completed DingTalk agent and channel integration; optimized WeChat channel features.
5. **Build System Enhancement**: Enhanced Taskfile for Windows build with `SHOW_CMD` variable to control console window visibility during development and production builds.
6. **README Updates**: Updated README files with improved clarity and structure; introduced new `README_en.md` for English documentation with enhanced AI feature descriptions.
7. **UI/UX Improvements**: Channel display optimization; login popup style refinements; various text adjustments.
8. **Code Cleanup**: Removed nil checks for openclawManager in restart method for simplified logic; updated logging and comments for better clarity.

### 2026/04/08
1. **OpenClaw In-Memory Models Cache**: Implemented in-memory models cache for improved efficiency in model registration and synchronization. The cache is populated from openclaw.json at startup and refreshed during config sync.
2. **OpenClaw Real-Time Status Updates**: Enhanced real-time status updates by subscribing to backend events for gateway state and runtime status, replacing the previous polling mechanism for immediate frontend updates.
3. **OpenClaw Gateway Polling & Restart Logic**: Implemented polling mechanism for gateway status after restart to ensure accurate state detection; simplified gateway restart logic by removing redundant port status checks and enhancing error handling.
4. **OpenClaw Readiness Diagnostics**: Added DebugIsReadyState method for enhanced diagnostics of gateway readiness; refactored to simplify readiness checks relying on the gateway's connection status.
5. **OpenClaw Upgrade Enhancement**: Added upgrade cancellation and continuation features; updated runtime upgrade logic to use a candidate staging directory for improved reliability.
6. **OpenClaw Staged Installation**: Integrated OpenClaw runtime management with system mode synchronization, auto-start based on sidebar mode, and bundled binaries support from the full installer.
7. **Gateway Doctor Auto-Trigger**: Implemented auto-trigger for doctor diagnostics on consecutive WebSocket failures with UI updates and enhanced authentication support.
8. **ChatWiki Model Enhancements**: Implemented cleanup for corrupted chatwiki model entries; enhanced model catalog item parsing with numeric ID detection and improved model ID resolution.
9. **UI/UX Improvements**: Gateway style updates; fixed floating window login issue; ChatWiki model switch now disabled when not logged in; assistant session switch shows latest messages; batch operation button display optimization; knowledge base card English word wrap support; icon replacement with SVG.
10. **Localization Updates**: Updated 'updatesAvailableToast' message across multiple languages; multi-language README support enhancements.

### 2026/04/07
1. **OpenClaw NSIS Installer Refactoring**: Complete overhaul of NSIS project file including custom uninstaller macro, removal of file-level progress display, enhanced extraction feedback with dynamic progress updates, and improved uninstallation process with detail prints for process termination and cleanup.
2. **OpenClaw Gateway Authentication**: Added authentication configuration with fields for pairing and auto-approval; implemented background polling loop to monitor gateway status and handle WebSocket reconnections.
3. **Gateway Startup Optimization**: Simplified gateway startup logic by removing port occupation checks, relying on reconcileLocked to handle existing instances.
4. **Gateway Device Management**: Enhanced approvePendingDevices function with improved JSON handling, logging, and verification for successful approvals.
5. **Configuration Management**: Updated ensureOpenClawStateDir and ensureGatewayAuthConfig to accept gateway port as parameter for consistent port usage.
6. **Knowledge Base Learning Tasks**: Application restart now continues learning tasks; added learning document limits; fixed issue where failed learning tasks still occupied document count.
7. **Knowledge Base UI/UX**: Added knowledge base settings translations; folder selection and batch operations support; document selection and batch operations.
8. **Style Updates**: Input number optimization; knowledge base settings styling improvements.

### 2026/04/03
1. **OpenClaw Doctor Console Enhancement**: Integrated `ansi-to-html` for enhanced console output rendering with ANSI color support in both light and dark themes. Added streaming support for doctor command output with improved stdout/stderr handling and event emission for UI updates.
2. **OpenClaw Runtime Upgrade System**: Implemented comprehensive upgrade status management with "Upgrading" state across components and localization files. Enhanced npm package installation with progress updates and improved error handling, including rollback mechanisms.
3. **Gateway Port Management**: Added port occupation checks with user feedback for gateway operations. Implemented `ensurePortClean` method to detect and gracefully stop stale processes using the gateway port. Added stop port functionality.
4. **ChatWiki Sync Enhancement**: Enhanced ChatWiki sync data fetching to support forced refresh for up-to-date model catalog. Implemented model catalog refresh mechanism for OpenClaw sync to ensure models stay current.
5. **OpenClaw Auto-Start**: Implemented auto-start functionality for OpenClaw gateway with UI toggle and localization support. Configuration persists auto-start preference with toast notifications for state changes.
6. **Chat Session Sync**: Implemented OpenClaw chat session synchronization functionality.
7. **WhatsApp Integration**: Enhanced WhatsApp binding functionality.
8. **QQ/Enterprise WeChat Optimization**: Resolved download plugin rate limiting issues. Fixed Chinese/English prompts and login redirect issues.
9. **Style Updates**: Removed team features from OpenClaw mode. Updated settings component layout and styling for improved responsiveness.
10. **Version Bump**: Application version updated to 0.9.1. OpenClaw version updated to 2026.4.2.

### 2026/04/02
1. **OpenClaw Doctor Integration**: Added gateway status handling in SideNav with localized messages. Updated ChatInputArea to reflect gateway state with relevant UI components.
2. **i18n Enhancement**: Added translation files for new gateway and doctor messages across multiple languages. Added toast notifications for toolchain updates.
3. **OpenClaw Version Update**: Updated OpenClaw version to 2026.4.1 with enhanced gateway integration in frontend.


### 2026/04/01
1. **Version 0.9.0 Release**: Updated application version to 0.9.0 in build configuration.
2. **OpenClaw Installer Bundling**: Enhanced Taskfile.yml and NSIS installer with conditional OpenClaw runtime bundling support, including zip file existence checks before bundling.
3. **OpenClaw Version Update**: Upgraded OpenClaw runtime to version 2026.3.31.
4. **Multi-Language Translations**: Updated translations across Arabic, Bengali, German, English, Spanish, and French locales with new keys and improved consistency.
5. **Scheduled Task Bug Fixes**: Fixed cron display anomalies and edited scheduled task issues.
6. **WeChat Text Updates**: Updated WeChat-related text copy and default naming conventions.

### 2026/03/31
1. **WeChat/WeCom/QQ Channel Conflict Resolution**: Merged WeChat, WeCom, and QQ channel support with conflict resolution and QR code login for WeChat bots.
2. **Feishu Message Hook**: Added Feishu (Lark) message webhook integration.
3. **Tool Selection**: Available tools list now supports selection and configuration.
4. **Eino Framework Upgrade**: Upgraded Eino framework to version 0.8.5.
5. **Console Window Optimization**: Updated BUILD_FLAGS to improve DEV environment debugging and suppress console flash in production builds.
6. **Channel Delivery Bug Fixes**: Fixed channel push exceptions, delivery anomalies, and assistant/channel sync delay issues.
7. **Scheduled Task Refinements**: Fixed task editing, filter issues, and partial save/edit failures.
8. **Memory Optimization**: Improved memory handling and last_sender_id tracking.
9. **Multi-Language Support**: Added Traditional Chinese (zh-TW) locale support.

### 2026/03/30
1. **OpenClaw QQ Integration Complete**: Completed QQ channel integration for OpenClaw, with style adjustments for knowledge base selection and account management display.
2. **WeChat Bot QR Code Login**: Implemented QR code login for creating WeChat bots via scan.
3. **DingTalk Enhancements**: Added DingTalk plugin installation, session synchronization, and agent binding functionality.
4. **Linux Server OpenClaw Runtime Export**: Added OpenClaw runtime export support for Linux servers.
5. **Streaming Output**: Enhanced streaming output for OpenClaw agents.
6. **Scheduled Task Adjustments**: Translation updates and scheduled task configuration refinements.

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
