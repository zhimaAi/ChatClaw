#!/usr/bin/env python3
"""
Auto-translate frontend i18n files using deep-translator.
"""

import os
import re
import json
import argparse
import time
from concurrent.futures import ThreadPoolExecutor, as_completed

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
FRONTEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'frontend', 'src', 'locales'))

DEFAULT_BASELINE = 'zh-CN.ts'
CJK_LANGUAGES = ['ja-JP', 'ko-KR', 'zh-TW']

# Language codes for translation
LANG_MAP = {
    'ar-SA': 'ar', 'bn-BD': 'bn', 'de-DE': 'de', 'es-ES': 'es', 'fr-FR': 'fr',
    'hi-IN': 'hi', 'it-IT': 'it', 'ja-JP': 'ja', 'ko-KR': 'ko', 'pt-BR': 'pt',
    'sl-SI': 'sl', 'tr-TR': 'tr', 'vi-VN': 'vi', 'zh-TW': 'zh-TW'
}

TRANSLATION_TARGETS = {
    'de-DE': ('de', 'German'),
    'fr-FR': ('fr', 'French'),
    'es-ES': ('es', 'Spanish'),
    'it-IT': ('it', 'Italian'),
    'pt-BR': ('pt', 'Portuguese'),
    'ru-RU': ('ru', 'Russian'),
    'ja-JP': ('ja', 'Japanese'),
    'ko-KR': ('ko', 'Korean'),
    'ar-SA': ('ar', 'Arabic'),
    'hi-IN': ('hi', 'Hindi'),
    'vi-VN': ('vi', 'Vietnamese'),
    'tr-TR': ('tr', 'Turkish'),
    'bn-BD': ('bn', 'Bengali'),
    'sl-SI': ('sl', 'Slovenian'),
    'zh-TW': ('zh-TW', 'Chinese (Traditional)'),
}


def parse_ts_file(content):
    """Parse TS file to flat dict with dot-notation keys"""
    code = re.sub(r'^export\s+default\s*', '', content.strip())
    code = re.sub(r';\s*$', '', code)

    result = {}
    lines = code.split('\n')
    current_path = []

    for line in lines:
        indent = len(line) - len(line.lstrip())
        if not line.strip():
            continue
        if line.strip().startswith('//'):
            continue

        rel_indent = indent // 2
        while len(current_path) > rel_indent:
            current_path.pop()

        obj_match = re.match(r'^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*:\s*\{', line)
        if obj_match:
            key = obj_match.group(1)
            current_path.append(key)
            continue

        value_match = re.match(r'^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*:\s*', line)
        if value_match:
            key = value_match.group(1)
            rest = line[value_match.end():].strip()

            if rest.startswith('"'):
                value = parse_double_quoted_string(rest)
                if value is not None:
                    full_path = '.'.join(current_path + [key])
                    result[full_path] = value
                    continue
            elif rest.startswith("'"):
                value = parse_single_quoted_string(rest)
                if value is not None:
                    full_path = '.'.join(current_path + [key])
                    result[full_path] = value
                    continue

        if line.strip() == '}' or line.strip().startswith('},'):
            if current_path:
                current_path.pop()
            continue

    return result


def parse_double_quoted_string(s):
    """Parse a double-quoted string"""
    if not s.startswith('"'):
        return None
    result = []
    s = s[1:]
    i = 0
    while i < len(s):
        if s[i] == '\\' and i + 1 < len(s):
            next_char = s[i + 1]
            if next_char == 'n':
                result.append('\n')
            elif next_char == 'r':
                result.append('\r')
            elif next_char == 't':
                result.append('\t')
            elif next_char == '\\':
                result.append('\\')
            elif next_char == '"':
                result.append('"')
            elif next_char == "'":
                result.append("'")
            else:
                result.append(s[i:i+2])
            i += 2
        elif s[i] == '"':
            return ''.join(result)
        else:
            result.append(s[i])
            i += 1
    return ''.join(result)


def parse_single_quoted_string(s):
    """Parse a single-quoted string"""
    if not s.startswith("'"):
        return None
    result = []
    s = s[1:]
    i = 0
    while i < len(s):
        if s[i] == '\\' and i + 1 < len(s):
            next_char = s[i + 1]
            if next_char == "'":
                result.append("'")
            elif next_char == '\\':
                result.append('\\')
            else:
                result.append(s[i:i+2])
            i += 2
        elif s[i] == "'":
            return ''.join(result)
        else:
            result.append(s[i])
            i += 1
    return ''.join(result)


def to_nested_format(flat_obj):
    """Convert flat dict to nested TS format"""
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
        return value.replace('\\', '\\\\').replace("'", '"')

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
                    lines.append(f'{indent_str}{key}: {{')
                    print_object(value, indent + 1)
                    lines.append(f'{indent_str}}},')
            else:
                escaped_value = escape_ts_string(value)
                lines.append(f"{indent_str}{key}: '{escaped_value}',")

    print_object(nested)
    lines.append('}')
    lines.append('')
    return '\n'.join(lines)


def detect_chinese(text):
    """Check if text contains Chinese characters"""
    return bool(re.search(r'[\u4e00-\u9fff]', text))


def load_ts_file(filepath):
    with open(filepath, 'r', encoding='utf-8') as f:
        return parse_ts_file(f.read())


def save_ts_file(filepath, flat_obj):
    output = to_nested_format(flat_obj)
    with open(filepath, 'w', encoding='utf-8') as f:
        f.write(output)


def get_keys_needing_translation(target_keys, lang_code):
    """Get keys that contain Chinese text and need translation"""
    needs_trans = []
    for key, value in target_keys.items():
        if detect_chinese(value):
            needs_trans.append((key, value))
    return needs_trans


def translate_text(text, target_lang):
    """Translate text using deep-translator"""
    try:
        from deep_translator import GoogleTranslator
        translator = GoogleTranslator(source='zh-CN', target=target_lang)
        result = translator.translate(text)
        return result
    except Exception as e:
        print(f"Translation error: {e}")
        return None


def translate_file(target_file, target_lang_code):
    """Translate a single locale file"""
    print(f"\nTranslating {target_file} to {target_lang_code}...")

    target_path = os.path.join(FRONTEND_LOCALES_DIR, target_file)
    if not os.path.exists(target_path):
        print(f"  File not found: {target_path}")
        return 0

    target_data = load_ts_file(target_path)
    keys_to_translate = get_keys_needing_translation(target_data, target_lang_code)

    if not keys_to_translate:
        print(f"  No keys need translation")
        return 0

    print(f"  Found {len(keys_to_translate)} keys needing translation")

    translated_count = 0
    for key, original in keys_to_translate:
        # Skip if already properly translated or is a brand name
        if original in ['ChatClaw', 'OpenClaw', 'ChatWiki', 'Codex', 'ClawHub', 'SkillHub']:
            continue

        result = translate_text(original, target_lang_code)
        if result:
            target_data[key] = result
            translated_count += 1
            print(f"  Translated: {key[:40]}...")
            time.sleep(0.3)  # Rate limiting
        else:
            print(f"  Failed: {key}")

    if translated_count > 0:
        save_ts_file(target_path, target_data)
        print(f"  Saved {translated_count} translations")

    return translated_count


def main():
    parser = argparse.ArgumentParser(description='Auto-translate frontend i18n files')
    parser.add_argument('--target', '-t', help='Specific target file (e.g., de-DE.ts)')
    parser.add_argument('--all', '-a', action='store_true', help='Translate all languages')
    args = parser.parse_args()

    print(f"Frontend locales dir: {FRONTEND_LOCALES_DIR}")

    if args.target:
        target_file = args.target
        if not target_file.endswith('.ts'):
            target_file += '.ts'
        lang_code = target_file.replace('.ts', '')
        if lang_code in LANG_MAP:
            translate_file(target_file, LANG_MAP[lang_code])
    elif args.all:
        total = 0
        for target_file in sorted(os.listdir(FRONTEND_LOCALES_DIR)):
            if target_file.endswith('.ts') and target_file != 'index.ts' and target_file != 'zh-CN.ts' and target_file != 'en-US.ts':
                lang_code = target_file.replace('.ts', '')
                if lang_code in LANG_MAP:
                    total += translate_file(target_file, LANG_MAP[lang_code])
        print(f"\nTotal translations: {total}")


if __name__ == '__main__':
    main()
