import os
import re
import json

# Get the directory where this script is located
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
# Go up to project root, then to frontend/src/locales
FRONTEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'frontend', 'src', 'locales'))

# Default baseline language
DEFAULT_BASELINE = 'zh-CN'

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
    parser.add_argument('--baseline', '-b', default=DEFAULT_BASELINE, 
                        help=f'Baseline language file (default: {DEFAULT_BASELINE})')
    parser.add_argument('--target', '-t', 
                        help='Target language file to compare (optional, compares all if not specified)')
    parser.add_argument('--list', '-l', action='store_true',
                        help='List all available locale files')
    
    args = parser.parse_args()
    
    print(f"Frontend locales dir: {FRONTEND_LOCALES_DIR}")
    
    if args.list:
        locales = get_all_locales()
        print(f"\nAvailable locales ({len(locales)}):")
        for f in locales:
            print(f"  - {f}")
        return
    
    baseline_file = args.baseline
    if not baseline_file.endswith('.ts'):
        baseline_file += '.ts'
    
    # Check baseline exists
    if baseline_file not in os.listdir(FRONTEND_LOCALES_DIR):
        print(f"Error: Baseline file {baseline_file} not found!")
        return
    
    if args.target:
        target_file = args.target
        if not target_file.endswith('.ts'):
            target_file += '.ts'
        compare_files(baseline_file, target_file)
    else:
        # Compare all files except baseline
        locales = get_all_locales()
        for target_file in locales:
            if target_file != baseline_file:
                print()
                compare_files(baseline_file, target_file)
                print()

if __name__ == '__main__':
    main()
