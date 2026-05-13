#!/usr/bin/env python3
"""Import CJK translations from English."""
import os, re, json

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
FRONTEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'frontend', 'src', 'locales'))

def parse_ts_file(content):
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
                value = parse_dq(rest)
                if value is not None:
                    result['.'.join(current_path + [key])] = value
                    continue
            elif rest.startswith("'"):
                value = parse_sq(rest)
                if value is not None:
                    result['.'.join(current_path + [key])] = value
                    continue
        if line.strip() in ('}', '},'):
            if current_path:
                current_path.pop()
    return result

def parse_dq(s):
    if not s.startswith('"'): return None
    result = []
    s = s[1:]
    i = 0
    while i < len(s):
        if s[i] == '\\' and i + 1 < len(s):
            n = s[i+1]
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

def parse_sq(s):
    if not s.startswith("'"): return None
    result = []
    s = s[1:]
    i = 0
    while i < len(s):
        if s[i] == '\\' and i + 1 < len(s):
            n = s[i+1]
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
            if part not in current: current[part] = {}
            if isinstance(current[part], str): current[part] = {'_value': current[part]}
            current = current[part]
        if parts[-1] in current and isinstance(current[parts[-1]], dict) and not isinstance(value, dict):
            current[parts[-1]]['_value'] = value
        else:
            current[parts[-1]] = value
    def esc(v): return str(v).replace("'", '"')
    def po(obj, indent=1):
        is2 = '  ' * indent
        for k, v in obj.items():
            if k == '_value': continue
            if isinstance(v, dict):
                if '_value' in v and len(v) == 1:
                    lines.append(f"{is2}{k}: '{esc(v['_value'])}',")
                else:
                    lines.append(f"{is2}{k}: {{")
                    po(v, indent+1)
                    lines.append(f"{is2}}},")
            else:
                lines.append(f"{is2}{k}: '{esc(v)}',")
    po(nested)
    lines.append('}')
    lines.append('')
    return '\n'.join(lines)

def load_ts_file(p):
    with open(p, 'r', encoding='utf-8') as f:
        return parse_ts_file(f.read())

def save_ts_file(p, obj):
    with open(p, 'w', encoding='utf-8') as f:
        f.write(to_nested_format(obj))

# Japanese translations
ja_JP = {
    "app.title": "ChatClaw",
    "winsnap.title": "ChatClaw",
    "nav.systemChatClaw": "ChatClaw",
    "settings.openclawRuntime.sidebarGatewayLabelSeparator": ": ",
    "settings.openclawRuntime.statusBadge.error": "エラー",
    "settings.openclawRuntime.statusBadge.running": "実行中",
    "settings.openclawRuntime.statusBadge.starting": "起動中",
    "settings.openclawRuntime.statusBadge.stop": "停止",
    "settings.general.toolchain.testInstall.customProxyPlaceholder": "https://your-proxy.com/",
    "settings.mcp.title": "MCP",
    "settings.mcp.tabServers": "MCPサーバー",
    "settings.snap.apps.qq": "QQ",
    "settings.chatwiki.title": "ChatWiki",
    "settings.about.appName": "ChatClaw",
    "settings.skillMarket.selectAgent": "エージェント",
    "settings.skillMarket.sourceClawhub": "ClawHub",
    "settings.skillMarket.sourceSkillhub": "SkillHub",
    "settings.skillMarket.sourceChatclaw": "ChatClaw",
    "assistant.chat.chatwikiSection": "ChatWiki",
    "assistant.settings.model.topP": "Top-P",
    "assistant.settings.advanced.groupChatMentionPatternsPlaceholder": "\"@\"assistant, \"@\"bot",
    "channels.platforms.whatsapp": "WhatsApp",
    "channels.platforms.qq": "QQ",
    "channels.meta.feishu.name": "飛書 / Lark",
    "channels.meta.telegram.name": "Telegram",
    "channels.meta.telegram.botName": "Telegram",
    "channels.meta.discord.name": "Discord",
    "channels.meta.discord.botName": "Discord",
    "channels.meta.whatsapp.name": "WhatsApp",
    "channels.meta.whatsapp.botName": "WhatsApp",
    "channels.meta.whatsapp.description": "WhatsApp ビジネスAPI",
    "channels.meta.qq.name": "QQ",
    "channels.meta.qq.botName": "QQ",
    "scheduledTasks.form.calendarTitle": "{year}年 {month}月",
    "scheduledTasks.form.yearOption": "{year}年",
    "scheduledTasks.form.monthOption": "{month}月",
}

# Korean translations
ko_KR = {
    "settings.openclawRuntime.sidebarGatewayLabelSeparator": ": ",
    "settings.general.toolchain.testInstall.customProxyPlaceholder": "https://your-proxy.com/",
    "settings.mcp.title": "MCP",
    "settings.mcp.tabServers": "MCP 서버",
    "settings.snap.apps.qq": "QQ",
    "settings.skillMarket.selectAgent": "에이전트",
    "settings.skillMarket.sourceClawhub": "ClawHub",
    "settings.skillMarket.sourceSkillhub": "SkillHub",
    "settings.skillMarket.sourceChatclaw": "ChatClaw",
    "assistant.settings.model.topP": "Top-P",
    "channels.platforms.qq": "QQ",
    "channels.meta.qq.name": "QQ",
    "channels.meta.qq.botName": "QQ",
}

# Chinese Traditional translations
zh_TW = {
    "app.title": "ChatClaw",
    "winsnap.title": "ChatClaw",
    "nav.systemChatClaw": "ChatClaw",
    "settings.openclawRuntime.sidebarGatewayLabelSeparator": "：",
    "settings.general.toolchain.testInstall.customProxyPlaceholder": "https://your-proxy.com/",
    "settings.mcp.title": "MCP",
    "settings.mcp.tabServers": "MCP 伺服器",
    "settings.snap.apps.qq": "QQ",
    "settings.chatwiki.title": "ChatWiki",
    "settings.about.appName": "ChatClaw",
    "settings.skillMarket.selectAgent": "代理",
    "settings.skillMarket.sourceClawhub": "ClawHub",
    "settings.skillMarket.sourceSkillhub": "SkillHub",
    "settings.skillMarket.sourceChatclaw": "ChatClaw",
    "assistant.chat.chatwikiSection": "ChatWiki",
    "assistant.settings.model.topP": "Top-P",
    "assistant.settings.advanced.groupChatMentionPatternsPlaceholder": "\"@\"assistant, \"@\"bot",
    "channels.platforms.whatsapp": "WhatsApp",
    "channels.platforms.qq": "QQ",
    "channels.meta.telegram.name": "Telegram",
    "channels.meta.telegram.botName": "Telegram",
    "channels.meta.discord.name": "Discord",
    "channels.meta.discord.botName": "Discord",
    "channels.meta.whatsapp.name": "WhatsApp",
    "channels.meta.whatsapp.botName": "WhatsApp",
    "channels.meta.qq.name": "QQ",
    "channels.meta.qq.botName": "QQ",
}

# Import Japanese
target_path = os.path.join(FRONTEND_LOCALES_DIR, 'ja-JP.ts')
data = load_ts_file(target_path)
count = 0
for key, value in ja_JP.items():
    if key in data:
        data[key] = value
        count += 1
save_ts_file(target_path, data)
print(f"Imported {count} translations to ja-JP.ts")

# Import Korean
target_path = os.path.join(FRONTEND_LOCALES_DIR, 'ko-KR.ts')
data = load_ts_file(target_path)
count = 0
for key, value in ko_KR.items():
    if key in data:
        data[key] = value
        count += 1
save_ts_file(target_path, data)
print(f"Imported {count} translations to ko-KR.ts")

# Import Chinese Traditional
target_path = os.path.join(FRONTEND_LOCALES_DIR, 'zh-TW.ts')
data = load_ts_file(target_path)
count = 0
for key, value in zh_TW.items():
    if key in data:
        data[key] = value
        count += 1
save_ts_file(target_path, data)
print(f"Imported {count} translations to zh-TW.ts")
