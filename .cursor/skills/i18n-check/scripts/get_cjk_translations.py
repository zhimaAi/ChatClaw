#!/usr/bin/env python3
"""Get CJK translations from en-US."""
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

# Load en-US
with open(os.path.join(FRONTEND_LOCALES_DIR, 'en-US.ts'), 'r', encoding='utf-8') as f:
    en_us = parse_ts_file(f.read())

# Load translation_needed.json
with open(os.path.join(SCRIPT_DIR, 'translation_needed.json'), 'r', encoding='utf-8') as f:
    needed = json.load(f)

# Output translations for CJK
cjk = ['ja-JP.ts', 'ko-KR.ts', 'zh-TW.ts']
for lang_file in cjk:
    if lang_file in needed:
        print(f'\n=== {lang_file} ===')
        for k in needed[lang_file]:
            if k in en_us:
                print(f'{k}: {en_us[k]}')
