#!/usr/bin/env python3
"""Import translations to locale files."""
import os, re

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

# Translations for en-US
en_us = {
    "openclawGateway.banner.notInstalled": "OpenClaw runtime environment not detected. Please go to Settings > General Settings or OpenClaw Manager to install.",
    "openclawGateway.banner.scheduledTasks": "Gateway is not running. Scheduled tasks cannot be managed when the gateway is not enabled.",
    "settings.chatwiki.openclawDescription": "Authorize and bind ChatWiki to use ChatWiki's own models and account credits.",
    "settings.chatwiki.switchBinding": "Switch Binding",
    "settings.general.toolchain.openclaw.description": "One-click install and manage OpenClaw runtime environment, supporting agent workflows and toolchain capabilities.",
    "settings.openclawRuntime.resetConfirmDesc": "This operation will delete all OpenClaw data storage folders, including plugins, conversation history, channel configurations, etc. This action cannot be undone. Continue?",
    "settings.runtimeEnvironment.installedHint": "OpenClaw runtime is installed. If not using temporarily, you can disable it by left-clicking. Later, you can go to",
    "settings.skillMarket.addSkillChoosePackageDesc": "Open the corresponding directory and place downloaded skill packages into it.",
    "settings.skillMarket.addSkillChoosePackageGuideDesc": "Search for desired skills and download installation files to the corresponding directory.",
    "settings.skillMarket.addSkillHintDesc": "In the opened directory, create a new folder and add a SKILL.md file to create a shared skill.",
    "settings.skillMarket.addSkillHintDescShared": "Create a new folder in the {dir} directory and add a SKILL.md file to create a shared skill.",
    "settings.skillMarket.addSkillViaChatDesc": "Chat with AI and let it help you design skill name, description, and implementation plan.",
    "settings.skillMarket.addSkillViaChatGuideDesc": "Search for desired skills, copy the installation prompt, and send it to the Agent.",
    "settings.skillMarket.securityVerifiedHint": "Verified for security and compliance, no malicious code or data leakage risks.",
}

# Load and update en-US
p = os.path.join(FRONTEND_LOCALES_DIR, 'en-US.ts')
data = load_ts_file(p)
c = 0
for k, v in en_us.items():
    if k in data:
        data[k] = v
        c += 1
        print(f"Updated: {k}")
save_ts_file(p, data)
print(f"\nImported {c} translations to en-US.ts")
