#!/usr/bin/env python3
"""
Import translations to locale files in batch.
Reads translations from a directory of translation files.
"""

import os
import re
import json
import glob

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
FRONTEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'frontend', 'src', 'locales'))

def parse_ts_file(content):
    """Parse TS file to flat dict with dot-notation keys"""
    code = re.sub(r'^export\s+default\s*', '', content.strip())
    code = re.sub(r';\s*$', '', code)
    result = {}
    lines = code.split('\n')
    current_path = []
    for line in lines:
        indent = len(line) - len(line.lstrip())
        if not line.strip() or line.strip().startswith('//'):
            continue
        rel_indent = indent // 2
        while len(current_path) > rel_indent:
            current_path.pop()
        obj_match = re.match(r'^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*:\s*\{', line)
        if obj_match:
            current_path.append(obj_match.group(1))
            continue
        value_match = re.match(r'^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*:\s*', line)
        if value_match:
            key = value_match.group(1)
            rest = line[value_match.end():].strip()
            if rest.startswith('"'):
                value = parse_double_quoted_string(rest)
                if value is not None:
                    result['.'.join(current_path + [key])] = value
                    continue
            elif rest.startswith("'"):
                value = parse_single_quoted_string(rest)
                if value is not None:
                    result['.'.join(current_path + [key])] = value
                    continue
        if line.strip() == '}' or line.strip().startswith('},'):
            if current_path:
                current_path.pop()
    return result

def parse_double_quoted_string(s):
    if not s.startswith('"'):
        return None
    result = []
    s = s[1:]
    i = 0
    while i < len(s):
        if s[i] == '\\' and i + 1 < len(s):
            n = s[i + 1]
            if n == 'n': result.append('\n')
            elif n == 'r': result.append('\r')
            elif n == 't': result.append('\t')
            elif n == '\\': result.append('\\')
            elif n == '"': result.append('"')
            elif n == "'": result.append("'")
            else: result.append(s[i:i+2])
            i += 2
        elif s[i] == '"':
            return ''.join(result)
        else:
            result.append(s[i])
            i += 1
    return ''.join(result)

def parse_single_quoted_string(s):
    if not s.startswith("'"):
        return None
    result = []
    s = s[1:]
    i = 0
    while i < len(s):
        if s[i] == '\\' and i + 1 < len(s):
            n = s[i + 1]
            if n == "'": result.append("'")
            elif n == '\\': result.append('\\')
            else: result.append(s[i:i+2])
            i += 2
        elif s[i] == "'":
            return ''.join(result)
        else:
            result.append(s[i])
            i += 1
    return ''.join(result)

def to_nested_format(flat_obj):
    lines = ['export default {']
    nested = {}
    for key, value in flat_obj.items():
        parts = key.split('.')
        current = nested
        for i, part in enumerate(parts[:-1]):
            if part not in current:
                current[part] = {}
            if isinstance(current[part], str):
                current[part] = {'_value': current[part]}
            current = current[part]
        if parts[-1] in current and isinstance(current[parts[-1]], dict) and not isinstance(value, dict):
            current[parts[-1]]['_value'] = value
        else:
            current[parts[-1]] = value

    def escape_ts_string(value):
        return str(value).replace("'", '"')

    def print_object(obj, indent=1):
        indent_str = '  ' * indent
        for key, value in obj.items():
            if key == '_value':
                continue
            if isinstance(value, dict):
                if '_value' in value and len(value) == 1:
                    escaped_value = escape_ts_string(value['_value'])
                    lines.append(f"{indent_str}{key}: '{escaped_value}',")
                else:
                    lines.append(f"{indent_str}{key}: {{")
                    print_object(value, indent + 1)
                    lines.append(f"{indent_str}}},")
            else:
                escaped_value = escape_ts_string(value)
                lines.append(f"{indent_str}{key}: '{escaped_value}',")

    print_object(nested)
    lines.append('}')
    lines.append('')
    return '\n'.join(lines)

def load_ts_file(filepath):
    with open(filepath, 'r', encoding='utf-8') as f:
        return parse_ts_file(f.read())

def save_ts_file(filepath, flat_obj):
    output = to_nested_format(flat_obj)
    with open(filepath, 'w', encoding='utf-8') as f:
        f.write(output)

def import_translations(target_file, translations):
    """Import translations to target file"""
    target_path = os.path.join(FRONTEND_LOCALES_DIR, target_file)
    target_data = load_ts_file(target_path)

    count = 0
    for key, value in translations.items():
        if key in target_data:
            target_data[key] = value
            count += 1

    save_ts_file(target_path, target_data)
    print(f"Updated {count} translations in {target_file}")
    return count

def main():
    # Load zh-CN baseline
    zh_cn_path = os.path.join(FRONTEND_LOCALES_DIR, 'zh-CN.ts')
    with open(zh_cn_path, 'r', encoding='utf-8') as f:
        zh_cn = parse_ts_file(f.read())

    # Load translation_needed.json
    with open(os.path.join(SCRIPT_DIR, 'translation_needed.json'), 'r', encoding='utf-8') as f:
        needed = json.load(f)

    # Translations for en-US
    en_us_translations = {
        "settings.openclawRuntime.resetConfirmDesc": "This operation will delete all OpenClaw data storage folders, including plugins, conversation history, channel configurations, etc. This action cannot be undone. Continue?",
        "settings.runtimeEnvironment.installedHint": "OpenClaw runtime is installed. If not using temporarily, you can disable it by left-clicking. Later, you can go to",
        "settings.general.toolchain.uv.description": "Enables AI assistant to write and run Python scripts to complete complex tasks.",
        "settings.general.toolchain.bun.description": "Enables AI assistant to write and run JavaScript scripts to complete complex tasks.",
        "settings.general.toolchain.codex.description": "Execute commands in an isolated environment to protect system security and prevent accidental operations from affecting local files.",
        "settings.general.toolchain.openclaw.description": "One-click install and manage OpenClaw runtime environment, supporting agent workflows and toolchain capabilities.",
        "settings.general.toolchain.updatesAvailableToast": "New version detected for extension. You can manually update in \"Settings → General Settings\".",
        "settings.memory.embeddingModelHint": "Used to convert memory text to vectors for semantic retrieval in conversations.",
        "settings.memory.enableHint": "When enabled, AI will automatically extract and remember your preferences, habits, and important facts during conversations.",
        "settings.memory.extractModelHint": "Used to summarize and extract valuable memory information after each conversation.",
        "settings.memory.rebuildWarning": "After modifying the vector model or dimensions, all existing memory vector data will be asynchronously rebuilt.",
        "settings.skills.directoryHint": "Place downloaded skill folders in this directory, and AI will automatically recognize and load them during conversations.",
        "settings.skills.enableHint": "When enabled, AI assistant will automatically load and use installed skills during conversations.",
        "settings.skillMarket.addSkillChoosePackageDesc": "Open the corresponding directory and place downloaded skill packages into it.",
        "settings.skillMarket.addSkillChoosePackageGuideDesc": "Search for desired skills and download installation files to the corresponding directory.",
        "settings.skillMarket.addSkillHint": "In the opened directory, create a new folder and add a SKILL.md file to create a shared skill.",
        "settings.skillMarket.addSkillHintDesc": "Create a new folder in the {dir} directory and add a SKILL.md file to create a shared skill.",
        "settings.skillMarket.addSkillViaChatDesc": "Chat with AI and let it help you design skill name, description, and implementation plan.",
        "settings.skillMarket.addSkillViaChatGuideDesc": "Search for desired skills, copy the installation prompt, and send it to the Agent.",
        "settings.skillMarket.securityVerifiedHint": "Verified for security and compliance, no malicious code or data leakage risks.",
        "settings.mcp.directoryHint": "Place MCP service configuration files in this directory, and AI will automatically recognize and connect during conversations.",
        "settings.mcp.enableHint": "When enabled, AI assistant will automatically connect to and use configured MCP services during conversations.",
        "settings.chatwiki.libraryEnabledHint": "Sync ChatWiki knowledge base. When enabled, it can be displayed in team knowledge base when asking questions.",
        "settings.chatwiki.modelBoundHint": "This displays the list of available model configurations for your current ChatWiki account.",
        "settings.chatwiki.modelServiceDesc": "After binding ChatWiki, you can directly view available models and credits in model service.",
        "settings.chatwiki.unbindConfirmDesc": "After unbinding, you will not be able to use applications and knowledge bases provided by ChatWiki. Are you sure you want to continue?",
        "settings.chatwiki.openclawDescription": "Authorize and bind ChatWiki to use ChatWiki's own models and account credits.",
        "settings.chatwiki.switchBinding": "Switch Binding",
        "settings.modelService.deleteBlockedByAgent": "This model is being used as the default model by assistant \"{name}\". Please modify the assistant settings before deleting.",
        "settings.modelService.deleteConfirmMessage": "Are you sure you want to delete model \"{name}\"? This action cannot be undone.",
        "settings.modelService.disableBlockedByAgent": "This provider is being used as the default model by assistant \"{name}\". Please modify the assistant settings before disabling.",
        "assistant.channels.createDesc": "Copy the add process from the channel page. After creation, it will automatically bind to the current assistant.",
        "assistant.errors.modelNotSupportVision": "Current model does not support image recognition. Please switch to a multimodal model that supports vision.",
        "assistant.settings.workspace.nativeDesc": "Execute commands directly on this machine without sandbox isolation. Commands have full permissions of the current user.",
        "assistant.settings.workspace.workDirHint": "Structure: {basePath}{sep}sessions{sep}<agent_hash>{sep}<conversation_hash>{sep}",
        "assistant.settings.advanced.builtinToolsUnavailable": "Unable to read OpenClaw built-in tools directory for now. You can still manually enter tool names.",
        "assistant.settings.advanced.groupChatMentionPatternsHint": "Match mention patterns in messages to trigger assistant responses. Separate multiple patterns with commas.",
        "assistant.settings.advanced.sandboxModeHint": "Sandbox can isolate command execution environment and prevent assistant from directly manipulating the host system.",
        "assistant.settings.advanced.toolsHint": "Deny takes priority over Allow. Plugin tools or custom tools can still be manually entered. Press Enter to confirm after input.",
        "knowledge.help.batchMaxChunks": "During learning embedding stage, the maximum number of chunks included per vectorization request. Range: 1-20.",
        "knowledge.help.chunkOverlap": "Overlap size between adjacent chunks (0-1000 characters). Used to reduce information loss caused by cross-chunk sentence breaks.",
        "knowledge.help.chunkSize": "Chunk size (500-5000 characters). Larger chunks mean more complete context, but coarser recall granularity.",
        "knowledge.help.raptorLLMModel": "Language model used to generate hierarchical summaries. Not selecting means this capability is disabled.",
        "knowledge.folder.deleteDesc": "After deleting folder \"{name}\", documents under it will be moved to \"Uncategorized\". This action cannot be undone.",
        "knowledge.folder.parentFolderHelp": "Select a parent folder to create nested folders. Leave empty to create in the root directory.",
        "channels.provisioning.toastDescription": "Channel and gateway are being created or synced. Connection status will update automatically. Please wait.",
        "channels.provisioning.toastDescriptionWithAgent": "Channel, assistant, or gateway are being created or synced. Please wait.",
        "channels.toggle.dingtalkPluginNotReady": "DingTalk connector plugin is still installing or not ready. Please try again later.",
        "channels.toggle.disableDesc": "After closing, the connection to this channel will be disconnected and the system will no longer receive its messages.",
        "channels.toggle.enableDesc": "After enabling, the system will attempt to connect to this channel to receive and process messages.",
        "channels.wecomAdd.tipsIntro": "Connect WeCom via QR code through Tencent's official OpenClaw plugin, while also supporting connection of existing bots.",
        "assistant.channels.autoGenerateDesc": "When creating a new assistant, you need to manually fill in name and other information. After successful creation, it will automatically bind and refresh the connection.",
        "channels.wechat.emptyDesc": "Connect personal WeChat via QR code through Tencent's official OpenClaw plugin, and start receiving and processing WeChat messages.",
        "channels.wechat.missingChannelId": "Channel information not obtained. Please close and refresh the channel list to try again.",
        "channels.wechat.missingChannelIdHint": "Channel ID not obtained. Please manually bind the assistant in the channel list after closing this window.",
        "channels.wechat.pluginInstallTryLater": "Official WeChat plugin is being installed or enabled in the background. Please try again later.",
        "channels.wechat.qrExpiredHint": "QR code has expired or timed out. Please click \"Refresh\" below to get a new one.",
        "channels.whatsapp.emptyDesc": "Connect WhatsApp via QR code through OpenClaw's built-in WhatsApp channel (WhatsApp Web).",
        "channels.whatsapp.pluginInstallTryLater": "WhatsApp channel is enabling or not ready yet. Please try again later.",
        "channels.whatsapp.steps": "After clicking the button below, the app will first enable the built-in WhatsApp channel in OpenClaw (if not already enabled).",
        "channels.whatsapp.steps2": "Then the app will run the QR code login process in the background. Please scan the QR code with your phone's WhatsApp.",
        "channels.whatsapp.steps3": "After successful login, you can bind an assistant and manage the connection status on this page. Deleting the channel will log out and clear the binding.",
        "scheduledTasks.form.promptPlaceholder": "What should AI do? For example: give me today's news and weather summary.",
        "scheduledTasks.notification.hintSelected": "Multiple selection allowed. Results will be sent through these channels after task completion.",
        "scheduledTasks.notification.hintUnselected": "First select notification type, then select specific channels from the corresponding platform channels.",
        "scheduledTasks.form.expiresAtHint": "This task has expired and will not run again. To resume execution, please modify the expiration time to a future date.",
        "scheduledTasks.deleteConfirmDescription": "Are you sure you want to delete task \"{name}\"? This action cannot be undone.",
        "openclawGateway.banner.notInstalled": "OpenClaw runtime environment not detected. Please go to \"Settings → General Settings\" or \"OpenClaw Manager\" to install.",
        "openclawGateway.banner.scheduledTasks": "Gateway is not running. Scheduled tasks cannot be managed when the gateway is not enabled.",
        "openclawCron.history.detailLoadFailedDescription": "Failed to get conversation details for this run. Please check OpenClaw running status and try again.",
        "openclawCron.history.waitingTriggeredRun": "Task has been triggered, waiting for OpenClaw to write new run record...",
        "openclawCron.dialog.exactHint": "Enable OpenClaw exact mode to trigger as close to the scheduled time as possible.",
    }

    # Import en-US translations
    print("\n=== Importing en-US translations ===")
    count = import_translations('en-US.ts', en_us_translations)
    print(f"Imported {count} translations to en-US.ts")

if __name__ == '__main__':
    main()
