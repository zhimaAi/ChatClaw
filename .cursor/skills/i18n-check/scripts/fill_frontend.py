import os
import re
import json
import argparse

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
FRONTEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'frontend', 'src', 'locales'))

DEFAULT_BASELINE = 'zh-CN.ts'

# For CJK languages (ja-JP, ko-KR, zh-TW), use English as baseline
CJK_BASELINE = 'en-US.ts'

# Languages that use Chinese characters natively (don't need Chinese translation)
CHINESE_SCRIPT_LANGUAGES = ['zh-TW', 'zh-HK', 'zh-MO', 'ja-JP', 'ko-KR']

# CJK languages that need special handling (compare against non-CJK languages)
CJK_LANGUAGES = ['ja-JP', 'ko-KR', 'zh-TW']


def get_baseline_for_lang(lang_code):
    """Get the appropriate baseline file for a given language"""
    if lang_code in CJK_LANGUAGES:
        return CJK_BASELINE
    return DEFAULT_BASELINE

def detect_chinese(text):
    """Check if text contains Simplified or Traditional Chinese characters"""
    return bool(re.search(r'[\u4e00-\u9fff]', text))

def parse_ts_file(content):
    """Parse TS file to flat dict with dot-notation keys"""
    code = re.sub(r'^export\s+default\s*', '', content.strip())
    code = re.sub(r';\s*$', '', code)

    result = {}
    lines = code.split('\n')
    current_path = []

    for i, line in enumerate(lines):
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

            inline_match = re.search(r':\s*\{(.+)\}\s*,?\s*$', line)
            if inline_match:
                inner_content = inline_match.group(1)
                inner_pairs = re.findall(r"([a-zA-Z_][a-zA-Z0-9_]*)\s*:\s*[\"']([^\"']*)[\"']", inner_content)
                for inner_key, value in inner_pairs:
                    full_path = '.'.join(current_path + [inner_key])
                    result[full_path] = value
                current_path.pop()
            continue

        # Parse string value - properly handle escaped quotes
        value_match = re.match(r'^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*:\s*', line)
        if value_match:
            key = value_match.group(1)
            rest = line[value_match.end():].strip()

            if rest.startswith('"'):
                # Parse double-quoted string with escape handling
                value = parse_double_quoted_string(rest)
                if value is not None:
                    full_path = '.'.join(current_path + [key])
                    result[full_path] = value
                    continue
            elif rest.startswith("'"):
                # Parse single-quoted string
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
    """Parse a double-quoted string, handling escaped quotes and other escapes"""
    if not s.startswith('"'):
        return None

    result = []
    s = s[1:]  # Skip opening quote

    i = 0
    while i < len(s):
        if s[i] == '\\' and i + 1 < len(s):
            # Handle escape sequences
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
                # Keep the escape sequence as-is for unknown escapes
                result.append(s[i:i+2])
            i += 2
        elif s[i] == '"':
            # End of string
            return ''.join(result)
        else:
            result.append(s[i])
            i += 1

    # Unterminated string - return what we have
    return ''.join(result)


def parse_single_quoted_string(s):
    """Parse a single-quoted string, handling escaped quotes"""
    if not s.startswith("'"):
        return None

    result = []
    s = s[1:]  # Skip opening quote

    i = 0
    while i < len(s):
        if s[i] == '\\' and i + 1 < len(s):
            # Handle escape sequences
            next_char = s[i + 1]
            if next_char == "'":
                result.append("'")
            elif next_char == '\\':
                result.append('\\')
            else:
                result.append(s[i:i+2])
            i += 2
        elif s[i] == "'":
            # End of string
            return ''.join(result)
        else:
            result.append(s[i])
            i += 1

    # Unterminated string - return what we have
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

    def print_object(obj, indent=1):
        indent_str = '  ' * indent
        for key, value in obj.items():
            if key == '_value':
                continue
            if isinstance(value, dict):
                if '_value' in value and len(value) == 1:
                    try:
                        unescaped = value['_value'].encode('utf-8').decode('unicode_escape')
                    except:
                        unescaped = value['_value']
                    escaped_value = unescaped.replace('\\', '\\\\').replace('"', '\\"')
                    lines.append(f'{indent_str}{key}: "{escaped_value}",')
                else:
                    lines.append(f'{indent_str}{key}: {{')
                    print_object(value, indent + 1)
                    lines.append(f'{indent_str}}},')
            else:
                try:
                    unescaped = value.encode('utf-8').decode('unicode_escape')
                except:
                    unescaped = value
                escaped_value = unescaped.replace('\\', '\\\\').replace('"', '\\"')
                lines.append(f'{indent_str}{key}: "{escaped_value}",')

    print_object(nested)
    lines.append('}')
    lines.append('')
    return '\n'.join(lines)

def load_baseline(baseline_file):
    filepath = os.path.join(FRONTEND_LOCALES_DIR, baseline_file)
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()
    return parse_ts_file(content)

def load_target(target_file):
    filepath = os.path.join(FRONTEND_LOCALES_DIR, target_file)
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()
    return parse_ts_file(content)

def save_target(target_file, flat_obj):
    output = to_nested_format(flat_obj)
    filepath = os.path.join(FRONTEND_LOCALES_DIR, target_file)
    with open(filepath, 'w', encoding='utf-8') as f:
        f.write(output)

def get_missing_keys(baseline_keys, target_keys):
    baseline_set = set(baseline_keys.keys())
    target_set = set(target_keys.keys())
    return sorted(baseline_set - target_set)

def get_all_locales():
    files = []
    for f in os.listdir(FRONTEND_LOCALES_DIR):
        if f.endswith('.ts') and f != 'index.ts':
            files.append(f)
    return sorted(files)

def get_language_code(filename):
    """Extract language code from filename (e.g., en-US from en-US.ts)"""
    return filename.replace('.ts', '')

def get_non_cjk_locales():
    """Get all non-CJK locale files (for CJK language filling)"""
    files = []
    for f in os.listdir(FRONTEND_LOCALES_DIR):
        if f.endswith('.ts') and f != 'index.ts':
            lang_code = f.replace('.ts', '')
            if lang_code not in CJK_LANGUAGES:
                files.append(f)
    return sorted(files)

def detect_untranslated_for_cjk(target_keys, ref_keys):
    """
    For CJK languages, detect keys that need translation.
    If target value equals reference value, it's likely untranslated.
    """
    untranslated = []
    for key in target_keys:
        if key in ref_keys:
            target_value = target_keys[key]
            ref_value = ref_keys[key]
            # If target equals reference (or empty), mark as needing translation
            if target_value == ref_value or not target_value.strip():
                untranslated.append(key)
    return untranslated

def needs_translation_check(lang_code):
    """Check if this language needs Chinese translation detection"""
    # Chinese script languages (zh-TW, zh-HK, ja, ko) don't need Chinese detection
    # They have their own character systems
    for cjk_lang in CHINESE_SCRIPT_LANGUAGES:
        if lang_code.startswith(cjk_lang.replace('-', '')):
            return False
    return True

def fill_missing_keys(target_file, baseline_keys, target_keys, missing_keys, lang_code, use_cjk_mode=False):
    """Fill missing keys with baseline values as placeholder"""
    needs_translation = []

    # Check if this language needs Chinese detection
    check_translation = needs_translation_check(lang_code)

    # For CJK languages, get reference keys from non-CJK locales
    ref_keys = {}
    if use_cjk_mode and lang_code in CJK_LANGUAGES:
        non_cjk_files = get_non_cjk_locales()
        if non_cjk_files:
            ref_file = non_cjk_files[0]
            ref_keys = load_target(ref_file)

    for key in missing_keys:
        baseline_value = baseline_keys[key]

        # For CJK languages, check if the value from baseline equals the reference (English) value
        # If so, this key needs translation - DON'T replace with English, keep baseline value
        if use_cjk_mode and lang_code in CJK_LANGUAGES and key in ref_keys:
            ref_value = ref_keys[key]
            # If baseline equals reference (e.g., both are English like "ChatClaw"),
            # this key doesn't need translation - it's already correct
            if baseline_value == ref_value:
                # Value is the same in baseline and reference (e.g., brand name), keep it
                target_keys[key] = baseline_value
            else:
                # Value differs - baseline has Chinese, reference has English
                # Use baseline value (Chinese) as placeholder, mark for translation
                target_keys[key] = baseline_value
                if key not in needs_translation:
                    needs_translation.append(key)
        else:
            # Normal mode - use baseline value directly
            target_keys[key] = baseline_value

            # If key was newly added and contains Chinese, mark for translation
            if check_translation and detect_chinese(baseline_value):
                needs_translation.append(key)

    save_target(target_file, target_keys)
    return needs_translation


def check_cjk_untranslated(target_file, lang_code):
    """
    Check for untranslated keys in CJK language files.
    Returns keys that match reference (non-CJK) values.
    """
    if lang_code not in CJK_LANGUAGES:
        return []
    
    target_keys = load_target(target_file)
    non_cjk_files = get_non_cjk_locales()
    
    if not non_cjk_files:
        return []
    
    # Use first non-CJK as reference
    ref_file = non_cjk_files[0]
    ref_keys = load_target(ref_file)
    
    return detect_untranslated_for_cjk(target_keys, ref_keys)

def save_translation_needed(translation_needed):
    """Save the list of keys that need translation to a JSON file"""
    output_file = os.path.join(SCRIPT_DIR, 'translation_needed.json')
    with open(output_file, 'w', encoding='utf-8') as f:
        json.dump(translation_needed, f, ensure_ascii=False, indent=2)
    print(f"\nTranslation needed list saved to: {output_file}")

def main():
    parser = argparse.ArgumentParser(description='Fill missing i18n keys in frontend TS files')
    parser.add_argument('--baseline', '-b', default=None, 
                        help=f'Baseline language file (auto-detect for CJK)')
    parser.add_argument('--target', '-t', 
                        help='Target language file to fill (fills all if not specified)')
    parser.add_argument('--dry-run', '-n', action='store_true',
                        help='Show what would be filled without making changes')
    parser.add_argument('--cjk', action='store_true',
                        help='For CJK languages (ja-JP, ko-KR, zh-TW), use en-US as baseline')
    parser.add_argument('--check-cjk', action='store_true',
                        help='Check CJK languages for untranslated keys (after fill)')
    
    args = parser.parse_args()
    
    print(f"Frontend locales dir: {FRONTEND_LOCALES_DIR}")
    
    # Check CJK untranslated mode
    if args.check_cjk:
        print("\n=== Checking CJK languages for untranslated keys ===")
        translation_needed = {}
        for cjk_lang in CJK_LANGUAGES:
            target_file = f"{cjk_lang}.ts"
            if target_file in os.listdir(FRONTEND_LOCALES_DIR):
                untranslated = check_cjk_untranslated(target_file, cjk_lang)
                if untranslated:
                    print(f"\n{target_file}: {len(untranslated)} untranslated keys")
                    translation_needed[target_file] = untranslated
                    for key in untranslated[:10]:
                        print(f"  - {key}")
                    if len(untranslated) > 10:
                        print(f"  ... and {len(untranslated) - 10} more")
                else:
                    print(f"\n{target_file}: All keys translated!")
        if translation_needed:
            save_translation_needed(translation_needed)
        return
    
    translation_needed = {}
    
    if args.target:
        target_file = args.target
        if not target_file.endswith('.ts'):
            target_file += '.ts'
        
        if target_file not in os.listdir(FRONTEND_LOCALES_DIR):
            print(f"Error: Target file {target_file} not found!")
            return
        
        lang_code = get_language_code(target_file)
        
        # Auto-detect baseline based on target language
        if args.baseline:
            baseline_file = args.baseline
            if not baseline_file.endswith('.ts'):
                baseline_file += '.ts'
        else:
            baseline_file = get_baseline_for_lang(lang_code) if args.cjk else DEFAULT_BASELINE
        
        if baseline_file not in os.listdir(FRONTEND_LOCALES_DIR):
            print(f"Error: Baseline file {baseline_file} not found!")
            return
        
        use_cjk_mode = args.cjk and lang_code in CJK_LANGUAGES
        baseline_keys = load_baseline(baseline_file)
        print(f"Baseline: {baseline_file} ({len(baseline_keys)} keys)")
        
        target_keys = load_target(target_file)
        missing = get_missing_keys(baseline_keys, target_keys)
        
        if missing:
            print(f"\n{target_file}: {len(missing)} missing keys")
            if args.dry_run:
                for key in missing:
                    print(f"  - {key}: {baseline_keys[key]}")
            else:
                needs_trans = fill_missing_keys(target_file, baseline_keys, target_keys, missing, lang_code, use_cjk_mode)
                print(f"  Filled {len(missing)} keys with baseline values")
                if needs_trans:
                    translation_needed[target_file] = needs_trans
                    print(f"  {len(needs_trans)} keys need translation")
        else:
            print(f"\n{target_file}: No missing keys")
    else:
        locales = get_all_locales()
        total_filled = 0
        
        for target_file in locales:
            lang_code = get_language_code(target_file)
            
            # Auto-detect baseline based on target language
            if args.baseline:
                baseline_file = args.baseline
                if not baseline_file.endswith('.ts'):
                    baseline_file += '.ts'
            else:
                baseline_file = get_baseline_for_lang(lang_code) if args.cjk else DEFAULT_BASELINE
            
            if baseline_file not in os.listdir(FRONTEND_LOCALES_DIR):
                print(f"Warning: Baseline file {baseline_file} not found, skipping {target_file}")
                continue
            
            if target_file == baseline_file:
                continue
            
            use_cjk_mode = args.cjk and lang_code in CJK_LANGUAGES
            baseline_keys = load_baseline(baseline_file)
            target_keys = load_target(target_file)
            missing = get_missing_keys(baseline_keys, target_keys)
            
            if missing:
                print(f"\n{target_file} (baseline: {baseline_file}): {len(missing)} missing keys")
                if args.dry_run:
                    for key in missing:
                        print(f"  - {key}: {baseline_keys[key]}")
                else:
                    needs_trans = fill_missing_keys(target_file, baseline_keys, target_keys, missing, lang_code, use_cjk_mode)
                    print(f"  Filled {len(missing)} keys")
                    total_filled += len(missing)
                    if needs_trans:
                        translation_needed[target_file] = needs_trans
                        print(f"  {len(needs_trans)} keys need translation")
        
        if not args.dry_run:
            print(f"\nTotal: Filled {total_filled} missing keys")
            if translation_needed:
                save_translation_needed(translation_needed)

if __name__ == '__main__':
    main()
