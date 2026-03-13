import os
import re
import json
import argparse

# Get the directory where this script is located
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
FRONTEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'frontend', 'src', 'locales'))
BACKEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'internal', 'services', 'i18n', 'locales'))

DEFAULT_BASELINE = 'zh-CN'

# CJK languages that need special handling
CJK_LANGUAGES = ['ja-JP', 'ko-KR', 'zh-TW']

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

def load_baseline_ts(baseline_file):
    """Load baseline TS file"""
    filepath = os.path.join(FRONTEND_LOCALES_DIR, baseline_file)
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()
    return parse_ts_file(content)

def load_target_ts(target_file):
    """Load target TS file"""
    filepath = os.path.join(FRONTEND_LOCALES_DIR, target_file)
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()
    return parse_ts_file(content)

def load_baseline_json(baseline_file):
    """Load baseline JSON file"""
    filepath = os.path.join(BACKEND_LOCALES_DIR, baseline_file)
    with open(filepath, 'r', encoding='utf-8') as f:
        return json.load(f)

def load_target_json(target_file):
    """Load target JSON file"""
    filepath = os.path.join(BACKEND_LOCALES_DIR, target_file)
    with open(filepath, 'r', encoding='utf-8') as f:
        return json.load(f)

def detect_chinese(text):
    """Check if text contains Chinese characters"""
    return bool(re.search(r'[\u4e00-\u9fff]', text))


def get_non_cjk_locales(locale_type='frontend'):
    """Get all non-CJK locale files"""
    if locale_type == 'frontend':
        locale_dir = FRONTEND_LOCALES_DIR
        ext = '.ts'
    else:
        locale_dir = BACKEND_LOCALES_DIR
        ext = '.json'
    
    files = []
    for f in os.listdir(locale_dir):
        if f.endswith(ext):
            lang_code = f.replace(ext, '')
            if lang_code not in CJK_LANGUAGES and lang_code != DEFAULT_BASELINE:
                files.append(f)
    return sorted(files)


def get_missing_with_chinese(baseline_data, target_data, baseline_keys):
    """Get missing keys that have Chinese values in baseline"""
    missing_chinese = {}
    for key in baseline_keys:
        if key not in target_data:
            # Key is missing, use baseline value
            if detect_chinese(baseline_data.get(key, '')):
                missing_chinese[key] = baseline_data[key]
        elif detect_chinese(target_data.get(key, '')):
            # Target has Chinese value, needs translation
            missing_chinese[key] = target_data[key]
    return missing_chinese


def get_cjk_untranslated(target_data, baseline_data, locale_type='frontend'):
    """
    For CJK languages, get keys that match reference (untranslated).
    Compares with non-CJK locales to find untranslated content.
    """
    non_cjk_files = get_non_cjk_locales(locale_type)
    
    if not non_cjk_files:
        return {}
    
    # Use first non-CJK as reference
    ref_file = non_cjk_files[0]
    if locale_type == 'frontend':
        ref_filepath = os.path.join(FRONTEND_LOCALES_DIR, ref_file)
        ref_data = parse_ts_file(open(ref_filepath, 'r', encoding='utf-8').read())
    else:
        ref_filepath = os.path.join(BACKEND_LOCALES_DIR, ref_file)
        ref_data = json.load(open(ref_filepath, 'r', encoding='utf-8'))
    
    untranslated = {}
    for key, target_value in target_data.items():
        if key in ref_data:
            ref_value = ref_data[key]
            # If target equals reference (or empty), mark as needing translation
            if target_value == ref_value or not target_value.strip():
                # Include baseline for reference
                baseline_value = baseline_data.get(key, '')
                untranslated[key] = {
                    'target': target_value,
                    'baseline': baseline_value,
                    'reference': ref_value
                }
    
    return untranslated

def export_frontend(target_file, output_file, cjk_mode=False):
    """Export missing translations for frontend"""
    baseline_file = f"{DEFAULT_BASELINE}.ts"
    baseline_keys = load_baseline_ts(baseline_file)
    target_keys = load_target_ts(target_file)
    
    lang_code = target_file.replace('.ts', '')
    
    if cjk_mode and lang_code in CJK_LANGUAGES:
        # For CJK languages, check against non-CJK reference
        missing_data = get_cjk_untranslated(target_keys, baseline_keys, 'frontend')
        
        if not missing_data:
            print(f"No untranslated keys found for {target_file}")
            return None
        
        # Generate output for CJK
        lines = []
        lines.append(f"# CJK Translation Export: {target_file}")
        lines.append(f"# Target: {lang_code}")
        lines.append(f"# Reference: non-CJK locale (en-US)")
        lines.append(f"# Total: {len(missing_data)} items")
        lines.append("")
        lines.append("## Translations (key = baseline | reference | current)")
        lines.append("")
        
        for key, data in sorted(missing_data.items()):
            lines.append(f"{key} = {data['baseline']} | {data['reference']} | {data['target']}")
    else:
        # Standard Chinese detection
        missing_data = get_missing_with_chinese(baseline_keys, target_keys, baseline_keys.keys())
        
        if not missing_data:
            print(f"No Chinese translations needed for {target_file}")
            return None
        
        # Generate output
        lines = []
        lines.append(f"# Translation Export: {target_file}")
        lines.append(f"# Total: {len(missing_data)} items")
        lines.append("")
        lines.append("## Translations (key = value)")
        lines.append("")
        
        for key, value in sorted(missing_data.items()):
            lines.append(f"{key} = {value}")
    
    output_path = os.path.join(SCRIPT_DIR, output_file)
    with open(output_path, 'w', encoding='utf-8') as f:
        f.write('\n'.join(lines))
    
    print(f"Exported {len(missing_data)} items to {output_file}")
    return missing_data


def export_backend(target_file, output_file, cjk_mode=False):
    """Export missing translations for backend"""
    baseline_file = f"{DEFAULT_BASELINE}.json"
    baseline_data = load_baseline_json(baseline_file)
    target_data = load_target_json(target_file)
    
    lang_code = target_file.replace('.json', '')
    
    if cjk_mode and lang_code in CJK_LANGUAGES:
        # For CJK languages, check against non-CJK reference
        missing_data = get_cjk_untranslated(target_data, baseline_data, 'backend')
        
        if not missing_data:
            print(f"No untranslated keys found for {target_file}")
            return None
        
        # Generate output for CJK
        lines = []
        lines.append(f"# CJK Translation Export: {target_file}")
        lines.append(f"# Target: {lang_code}")
        lines.append(f"# Reference: non-CJK locale")
        lines.append(f"# Total: {len(missing_data)} items")
        lines.append("")
        lines.append("## Translations (key = baseline | reference | current)")
        lines.append("")
        
        for key, data in sorted(missing_data.items()):
            lines.append(f"{key} = {data['baseline']} | {data['reference']} | {data['target']}")
    else:
        # Standard Chinese detection
        missing_data = get_missing_with_chinese(baseline_data, target_data, baseline_data.keys())
        
        if not missing_data:
            print(f"No Chinese translations needed for {target_file}")
            return None
        
        # Generate output
        lines = []
        lines.append(f"# Translation Export: {target_file}")
        lines.append(f"# Total: {len(missing_data)} items")
        lines.append("")
        lines.append("## Translations (key = value)")
        lines.append("")
        
        for key, value in sorted(missing_data.items()):
            lines.append(f"{key} = {value}")
    
    output_path = os.path.join(SCRIPT_DIR, output_file)
    with open(output_path, 'w', encoding='utf-8') as f:
        f.write('\n'.join(lines))
    
    print(f"Exported {len(missing_data)} items to {output_file}")
    return missing_data

def main():
    parser = argparse.ArgumentParser(description='Export missing translations for AI translation')
    parser.add_argument('--type', '-t', choices=['frontend', 'backend'], default='frontend',
                        help='Type of locale (frontend or backend)')
    parser.add_argument('--target', 
                        help='Target language file (e.g., en-US, ja-JP)')
    parser.add_argument('--output', '-o',
                        help='Output file name (default: translation_export_<lang>.txt)')
    parser.add_argument('--cjk', action='store_true',
                        help='For CJK languages (ja-JP, ko-KR, zh-TW), export by comparing with non-CJK locales')
    
    args = parser.parse_args()
    
    if args.type == 'frontend':
        if args.target:
            target_file = args.target if args.target.endswith('.ts') else f"{args.target}.ts"
            output_file = args.output or f"translation_export_{args.target}.txt"
            export_frontend(target_file, output_file, args.cjk)
        else:
            # Export for all languages
            locales = [f.replace('.ts', '') for f in os.listdir(FRONTEND_LOCALES_DIR) 
                      if f.endswith('.ts') and f != 'zh-CN.ts' and f != 'index.ts']
            for locale in locales:
                target_file = f"{locale}.ts"
                output_file = f"translation_export_{locale}.txt"
                # Auto-detect CJK for ja-JP, ko-KR, zh-TW
                cjk_mode = args.cjk or locale in CJK_LANGUAGES
                export_frontend(target_file, output_file, cjk_mode)
    else:
        if args.target:
            target_file = args.target if args.target.endswith('.json') else f"{args.target}.json"
            output_file = args.output or f"translation_export_{args.target}.txt"
            export_backend(target_file, output_file, args.cjk)
        else:
            # Export for all languages
            locales = [f.replace('.json', '') for f in os.listdir(BACKEND_LOCALES_DIR) 
                      if f.endswith('.json') and f != 'zh-CN.json']
            for locale in locales:
                target_file = f"{locale}.json"
                output_file = f"translation_export_{locale}.txt"
                # Auto-detect CJK for ja-JP, ko-KR, zh-TW
                cjk_mode = args.cjk or locale in CJK_LANGUAGES
                export_backend(target_file, output_file, cjk_mode)

if __name__ == '__main__':
    main()
