#!/usr/bin/env python3
"""
Auto-translate frontend i18n files.
This script translates missing/untranslated keys in locale files.
"""

import os
import re
import json
import argparse
import sys

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
FRONTEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'frontend', 'src', 'locales'))

DEFAULT_BASELINE = 'zh-CN.ts'
CJK_LANGUAGES = ['ja-JP', 'ko-KR', 'zh-TW']
CJK_BASELINE = 'en-US.ts'


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


def get_baseline_for_lang(lang_code):
    if lang_code in CJK_LANGUAGES:
        return CJK_BASELINE
    return DEFAULT_BASELINE


def get_language_code(filename):
    return filename.replace('.ts', '')


def load_baseline(baseline_file):
    filepath = os.path.join(FRONTEND_LOCALES_DIR, baseline_file)
    with open(filepath, 'r', encoding='utf-8') as f:
        return parse_ts_file(f.read())


def load_target(target_file):
    filepath = os.path.join(FRONTEND_LOCALES_DIR, target_file)
    with open(filepath, 'r', encoding='utf-8') as f:
        return parse_ts_file(f.read())


def save_target(target_file, flat_obj):
    output = to_nested_format(flat_obj)
    filepath = os.path.join(FRONTEND_LOCALES_DIR, target_file)
    with open(filepath, 'w', encoding='utf-8') as f:
        f.write(output)


def get_all_locales():
    files = []
    for f in os.listdir(FRONTEND_LOCALES_DIR):
        if f.endswith('.ts') and f != 'index.ts':
            files.append(f)
    return sorted(files)


def get_missing_keys(baseline_keys, target_keys):
    baseline_set = set(baseline_keys.keys())
    target_set = set(target_keys.keys())
    return sorted(baseline_set - target_set)


def get_keys_needing_translation(target_keys, baseline_keys, lang_code, use_cjk_mode=False):
    """Get keys that contain Chinese text and need translation"""
    needs_trans = []
    check_translation = True

    # CJK languages don't need Chinese detection
    if lang_code in CJK_LANGUAGES:
        check_translation = False

    for key in baseline_keys:
        baseline_value = baseline_keys[key]

        if use_cjk_mode and lang_code in CJK_LANGUAGES:
            # For CJK, check if value differs from English reference
            if key in target_keys:
                target_value = target_keys[key]
                if target_value == baseline_value or not target_value.strip():
                    needs_trans.append(key)
        else:
            # For non-CJK, check if baseline has Chinese and target is Chinese
            if key in target_keys:
                target_value = target_keys[key]
                if detect_chinese(target_value):
                    needs_trans.append(key)
            else:
                # Missing key - will be filled but needs translation
                if check_translation and detect_chinese(baseline_value):
                    needs_trans.append(key)

    return needs_trans


def main():
    parser = argparse.ArgumentParser(description='Check and report translation status')
    parser.add_argument('--verbose', '-v', action='store_true', help='Show detailed output')
    args = parser.parse_args()

    print(f"Frontend locales dir: {FRONTEND_LOCALES_DIR}")

    baseline_keys = load_baseline(DEFAULT_BASELINE)
    en_baseline = load_baseline(CJK_BASELINE)

    locales = get_all_locales()
    total_needing_trans = 0

    for target_file in locales:
        lang_code = get_language_code(target_file)
        if lang_code == 'zh-CN':
            continue

        baseline_file = get_baseline_for_lang(lang_code)
        use_cjk_mode = lang_code in CJK_LANGUAGES

        if use_cjk_mode:
            lang_baseline = en_baseline
        else:
            lang_baseline = baseline_keys

        target_keys = load_target(target_file)
        missing = get_missing_keys(lang_baseline, target_keys)

        needing_trans = get_keys_needing_translation(target_keys, lang_baseline, lang_code, use_cjk_mode)

        if missing or needing_trans:
            print(f"\n{target_file}:")
            print(f"  Missing keys: {len(missing)}")
            print(f"  Need translation: {len(needing_trans)}")
            total_needing_trans += len(needing_trans)

            if args.verbose and needing_trans:
                for key in needing_trans[:5]:
                    baseline_val = lang_baseline.get(key, 'N/A')
                    target_val = target_keys.get(key, 'MISSING')
                    print(f"    - {key}: {baseline_val[:50]}...")

    print(f"\nTotal keys needing translation: {total_needing_trans}")


if __name__ == '__main__':
    main()
