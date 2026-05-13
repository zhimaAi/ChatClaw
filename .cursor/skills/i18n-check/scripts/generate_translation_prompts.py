#!/usr/bin/env python3
"""
Generate translation prompts with key-value mapping.
"""

import os
import re
import json

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
FRONTEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'frontend', 'src', 'locales'))
TRANSLATION_NEEDED_FILE = os.path.join(SCRIPT_DIR, 'translation_needed.json')

def parse_ts_file(content):
    """Parse TS file to flat dict"""
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
            elif rest.startswith("'"):
                value = parse_single_quoted_string(rest)
            else:
                continue
            if value is not None:
                result['.'.join(current_path + [key])] = value
            continue
        if line.strip() in ('}', '},'):
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
            n = s[i+1]
            if n == 'n': result.append('\n')
            elif n == 'r': result.append('\r')
            elif n == 't': result.append('\t')
            elif n in ('\\', '"', "'"): result.append(n)
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

def escape_json_str(s):
    return s.replace('\\', '\\\\').replace('"', '\\"').replace('\n', '\\n').replace('\r', '\\r').replace('\t', '\\t')

def main():
    # Load translation_needed.json
    with open(TRANSLATION_NEEDED_FILE, 'r', encoding='utf-8') as f:
        needed = json.load(f)
    
    # Load zh-CN baseline
    with open(os.path.join(FRONTEND_LOCALES_DIR, 'zh-CN.ts'), 'r', encoding='utf-8') as f:
        zh_cn = parse_ts_file(f.read())
    
    # Language names
    names = {
        'ar-SA': 'Arabic', 'bn-BD': 'Bengali', 'de-DE': 'German', 'en-US': 'English',
        'es-ES': 'Spanish', 'fr-FR': 'French', 'hi-IN': 'Hindi', 'it-IT': 'Italian',
        'ja-JP': 'Japanese', 'ko-KR': 'Korean', 'pt-BR': 'Portuguese (Brazil)',
        'sl-SI': 'Slovenian', 'tr-TR': 'Turkish', 'vi-VN': 'Vietnamese', 'zh-TW': 'Chinese (Traditional)'
    }
    
    targets = {
        'ar-SA': 'ar', 'bn-BD': 'bn', 'de-DE': 'de', 'en-US': 'en',
        'es-ES': 'es', 'fr-FR': 'fr', 'hi-IN': 'hi', 'it-IT': 'it',
        'ja-JP': 'ja', 'ko-KR': 'ko', 'pt-BR': 'pt',
        'sl-SI': 'sl', 'tr-TR': 'tr', 'vi-VN': 'vi', 'zh-TW': 'zh-TW'
    }
    
    output = []
    output.append("# Translation Prompts with Key-Value Mapping\n")
    
    for lang_file, keys in sorted(needed.items()):
        lang_code = lang_file.replace('.ts', '')
        if lang_code not in names:
            continue
        lang_name = names[lang_code]
        target = targets[lang_code]
        
        output.append(f"\n## {lang_name} ({lang_code})")
        output.append(f"Target code: {target}\n")
        output.append("```json")
        output.append("{")
        output.append('  "translations": [')
        
        translations = []
        for key in keys:
            if key in zh_cn:
                original = zh_cn[key]
                original_escaped = escape_json_str(original)
                translations.append(f'    {{"key": "{key}", "original": "{original_escaped}", "translated": "TRANSLATION_HERE"}}')
        
        output.append(',\n'.join(translations))
        output.append("  ]")
        output.append("}")
        output.append("```\n")
    
    # Write output
    output_file = os.path.join(SCRIPT_DIR, 'translation_prompts_with_keys.txt')
    with open(output_file, 'w', encoding='utf-8') as f:
        f.write('\n'.join(output))
    print(f"Written to: {output_file}")

if __name__ == '__main__':
    main()
