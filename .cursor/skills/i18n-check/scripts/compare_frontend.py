import os
import re
import json
import argparse

# Get the directory where this script is located
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
# Go up to project root, then to frontend/src/locales
FRONTEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'frontend', 'src', 'locales'))

# Default baseline language
DEFAULT_BASELINE = 'zh-CN'

# For CJK languages (ja-JP, ko-KR, zh-TW), use English as baseline
CJK_BASELINE = 'en-US'

# CJK languages that need special handling (compare against non-CJK languages instead of zh-CN)
CJK_LANGUAGES = ['ja-JP', 'ko-KR', 'zh-TW']


def get_baseline_for_lang(lang_code):
    """Get the appropriate baseline file for a given language"""
    if lang_code in CJK_LANGUAGES:
        return f"{CJK_BASELINE}.ts"
    return f"{DEFAULT_BASELINE}.ts"

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

        value_match = re.match(r"^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*:\s*[\"']([^\"']*)[\"']", line)
        if value_match:
            key = value_match.group(1)
            value = value_match.group(2)
            full_path = '.'.join(current_path + [key])
            result[full_path] = value
            continue

        if line.strip() == '}' or line.strip().startswith('},'):
            if current_path:
                current_path.pop()
            continue

    return result

def load_baseline(baseline_file):
    """Load baseline TS file"""
    filepath = os.path.join(FRONTEND_LOCALES_DIR, baseline_file)
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()
    return parse_ts_file(content)

def load_target(target_file):
    """Load target TS file"""
    filepath = os.path.join(FRONTEND_LOCALES_DIR, target_file)
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()
    return parse_ts_file(content)

def compare_keys(baseline_keys, target_keys):
    """Compare baseline and target keys, return missing keys"""
    baseline_set = set(baseline_keys.keys())
    target_set = set(target_keys.keys())
    
    missing = baseline_set - target_set
    extra = target_set - baseline_set
    
    return sorted(missing), sorted(extra)

def format_output(baseline_file, target_file, missing, extra):
    """Format comparison output"""
    output = []
    output.append(f"Comparing: {baseline_file} (baseline) vs {target_file}")
    output.append("=" * 60)
    
    if missing:
        output.append(f"\nMissing keys in {target_file} ({len(missing)}):")
        for key in missing:
            output.append(f"  - {key}")
    else:
        output.append(f"\nNo missing keys!")
    
    if extra:
        output.append(f"\nExtra keys in {target_file} ({len(extra)}):")
        for key in extra:
            output.append(f"  + {key}")
    
    return '\n'.join(output)

def compare_files(baseline_file, target_file):
    """Compare two TS locale files"""
    baseline_keys = load_baseline(baseline_file)
    target_keys = load_target(target_file)
    
    missing, extra = compare_keys(baseline_keys, target_keys)
    
    print(format_output(baseline_file, target_file, missing, extra))
    
    return missing, extra


def get_non_cjk_locales():
    """Get all non-CJK locale files (for CJK language comparison)"""
    files = []
    for f in os.listdir(FRONTEND_LOCALES_DIR):
        if f.endswith('.ts') and f != 'index.ts':
            lang_code = f.replace('.ts', '')
            if lang_code not in CJK_LANGUAGES:
                files.append(f)
    return sorted(files)


def compare_cjk_with_non_cjk(target_file, target_lang):
    """
    For CJK languages (ja-JP, ko-KR, zh-TW), compare with non-CJK languages.
    Find keys that exist in non-CJK languages but have the same value in target (untranslated).
    """
    target_keys = load_target(target_file)
    non_cjk_files = get_non_cjk_locales()
    
    if not non_cjk_files:
        print(f"Warning: No non-CJK locales found for comparison!")
        return [], []
    
    # Use the first non-CJK locale as reference (usually en-US)
    ref_file = non_cjk_files[0]
    ref_keys = load_target(ref_file)
    
    # Find keys that exist in both target and reference
    common_keys = set(target_keys.keys()) & set(ref_keys.keys())
    
    untranslated = []
    for key in sorted(common_keys):
        target_value = target_keys[key]
        ref_value = ref_keys[key]
        
        # If target value equals reference value (or is empty), it's likely untranslated
        if target_value == ref_value or not target_value.strip():
            untranslated.append(key)
    
    return untranslated, ref_file


def format_cjk_output(target_file, target_lang, untranslated, ref_file):
    """Format output for CJK language comparison"""
    output = []
    output.append(f"CJK Comparison for {target_file}")
    output.append(f"Reference: {ref_file} (non-CJK)")
    output.append("=" * 60)
    
    if untranslated:
        output.append(f"\nUntranslated keys (same as {ref_file}) in {target_file} ({len(untranslated)}):")
        for key in untranslated[:20]:  # Show first 20
            output.append(f"  - {key}")
        if len(untranslated) > 20:
            output.append(f"  ... and {len(untranslated) - 20} more")
    else:
        output.append(f"\nNo untranslated keys found!")
    
    return '\n'.join(output)

def get_all_locales():
    """Get all locale files except index.ts"""
    files = []
    for f in os.listdir(FRONTEND_LOCALES_DIR):
        if f.endswith('.ts') and f != 'index.ts':
            files.append(f)
    return sorted(files)

def main():
    import argparse
    
    parser = argparse.ArgumentParser(description='Compare frontend i18n TS files')
    parser.add_argument('--baseline', '-b', default=None, 
                        help=f'Baseline language file (auto-detect for CJK)')
    parser.add_argument('--target', '-t', 
                        help='Target language file to compare (optional, compares all if not specified)')
    parser.add_argument('--list', '-l', action='store_true',
                        help='List all available locale files')
    parser.add_argument('--cjk', action='store_true',
                        help='For CJK languages (ja-JP, ko-KR, zh-TW), compare with non-CJK locales instead of zh-CN')
    parser.add_argument('--cjk-only', action='store_true',
                        help='Only compare CJK languages with non-CJK locales')
    
    args = parser.parse_args()
    
    print(f"Frontend locales dir: {FRONTEND_LOCALES_DIR}")
    
    if args.list:
        locales = get_all_locales()
        print(f"\nAvailable locales ({len(locales)}):")
        for f in locales:
            print(f"  - {f}")
        return
    
    if args.cjk_only:
        # Only compare CJK languages
        for cjk_lang in CJK_LANGUAGES:
            target_file = f"{cjk_lang}.ts"
            if target_file in os.listdir(FRONTEND_LOCALES_DIR):
                print()
                untranslated, ref_file = compare_cjk_with_non_cjk(target_file, cjk_lang)
                print(format_cjk_output(target_file, cjk_lang, untranslated, ref_file))
                print()
        return
    
    # Determine baseline file for each target
    if args.target:
        target_file = args.target
        if not target_file.endswith('.ts'):
            target_file += '.ts'
        
        target_lang = target_file.replace('.ts', '')
        
        # Auto-detect baseline based on target language
        if args.baseline:
            baseline_file = args.baseline
            if not baseline_file.endswith('.ts'):
                baseline_file += '.ts'
        else:
            baseline_file = get_baseline_for_lang(target_lang)
        
        # Check baseline exists
        if baseline_file not in os.listdir(FRONTEND_LOCALES_DIR):
            print(f"Error: Baseline file {baseline_file} not found!")
            return
        
        # Use CJK comparison if target is CJK language
        if args.cjk and target_lang in CJK_LANGUAGES:
            untranslated, ref_file = compare_cjk_with_non_cjk(target_file, target_lang)
            print(format_cjk_output(target_file, target_lang, untranslated, ref_file))
        else:
            compare_files(baseline_file, target_file)
    else:
        # Compare all files
        locales = get_all_locales()
        for target_file in locales:
            target_lang = target_file.replace('.ts', '')
            
            # Skip if same as baseline (auto-detect)
            if args.baseline:
                baseline_file = args.baseline
                if not baseline_file.endswith('.ts'):
                    baseline_file += '.ts'
            else:
                baseline_file = get_baseline_for_lang(target_lang)
            
            # Check baseline exists
            if baseline_file not in os.listdir(FRONTEND_LOCALES_DIR):
                print(f"Warning: Baseline file {baseline_file} not found, skipping {target_file}")
                continue
            
            if target_file == baseline_file:
                continue
            
            print()
            if args.cjk and target_lang in CJK_LANGUAGES:
                untranslated, ref_file = compare_cjk_with_non_cjk(target_file, target_lang)
                print(format_cjk_output(target_file, target_lang, untranslated, ref_file))
            else:
                compare_files(baseline_file, target_file)
            print()

if __name__ == '__main__':
    main()
