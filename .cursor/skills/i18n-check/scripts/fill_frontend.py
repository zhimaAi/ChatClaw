import os
import re
import json
import argparse

# Get the directory where this script is located
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
FRONTEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'frontend', 'src', 'locales'))

DEFAULT_BASELINE = 'zh-CN.ts'

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
            # If the current path is already a string, convert to dict
            if isinstance(current[part], str):
                current[part] = {'_value': current[part]}
            current = current[part]
        # If the final key already exists as a dict, we need to handle it
        if parts[-1] in current and isinstance(current[parts[-1]], dict) and not isinstance(value, dict):
            # Merge: keep existing nested keys and add new value
            current[parts[-1]]['_value'] = value
        else:
            current[parts[-1]] = value

    def print_object(obj, indent=1):
        indent_str = '  ' * indent
        for key, value in obj.items():
            # Skip _value keys (they were used for conflict resolution)
            if key == '_value':
                continue
            if isinstance(value, dict):
                # Check if it's a wrapper dict with _value
                if '_value' in value and len(value) == 1:
                    escaped_value = str(value['_value']).replace('\\', '\\\\').replace('"', '\\"')
                    lines.append(f'{indent_str}{key}: "{escaped_value}",')
                else:
                    lines.append(f'{indent_str}{key}: {{')
                    print_object(value, indent + 1)
                    lines.append(f'{indent_str}}},')
            else:
                escaped_value = str(value).replace('\\', '\\\\').replace('"', '\\"')
                lines.append(f'{indent_str}{key}: "{escaped_value}",')

    print_object(nested)
    lines.append('}')
    lines.append('')
    return '\n'.join(lines)

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

def save_target(target_file, flat_obj):
    """Save target TS file with missing keys filled"""
    output = to_nested_format(flat_obj)
    filepath = os.path.join(FRONTEND_LOCALES_DIR, target_file)
    with open(filepath, 'w', encoding='utf-8') as f:
        f.write(output)

def get_missing_keys(baseline_keys, target_keys):
    """Get missing keys from target"""
    baseline_set = set(baseline_keys.keys())
    target_set = set(target_keys.keys())
    return sorted(baseline_set - target_set)

def get_all_locales():
    """Get all locale files except index.ts"""
    files = []
    for f in os.listdir(FRONTEND_LOCALES_DIR):
        if f.endswith('.ts') and f != 'index.ts':
            files.append(f)
    return sorted(files)

def fill_missing_keys(target_file, baseline_keys, target_keys, missing_keys):
    """Fill missing keys with baseline values as placeholder"""
    # Add missing keys with baseline values
    for key in missing_keys:
        target_keys[key] = baseline_keys[key]
    
    # Save updated target file
    save_target(target_file, target_keys)
    return missing_keys

def main():
    parser = argparse.ArgumentParser(description='Fill missing i18n keys in frontend TS files')
    parser.add_argument('--baseline', '-b', default=DEFAULT_BASELINE, 
                        help=f'Baseline language file (default: {DEFAULT_BASELINE})')
    parser.add_argument('--target', '-t', 
                        help='Target language file to fill (fills all if not specified)')
    parser.add_argument('--dry-run', '-n', action='store_true',
                        help='Show what would be filled without making changes')
    
    args = parser.parse_args()
    
    print(f"Frontend locales dir: {FRONTEND_LOCALES_DIR}")
    
    baseline_file = args.baseline
    if not baseline_file.endswith('.ts'):
        baseline_file += '.ts'
    
    if baseline_file not in os.listdir(FRONTEND_LOCALES_DIR):
        print(f"Error: Baseline file {baseline_file} not found!")
        return
    
    baseline_keys = load_baseline(baseline_file)
    print(f"Baseline: {baseline_file} ({len(baseline_keys)} keys)")
    
    if args.target:
        target_file = args.target
        if not target_file.endswith('.ts'):
            target_file += '.ts'
        
        if target_file not in os.listdir(FRONTEND_LOCALES_DIR):
            print(f"Error: Target file {target_file} not found!")
            return
        
        target_keys = load_target(target_file)
        missing = get_missing_keys(baseline_keys, target_keys)
        
        if missing:
            print(f"\n{target_file}: {len(missing)} missing keys")
            if args.dry_run:
                for key in missing:
                    print(f"  - {key}: {baseline_keys[key]}")
            else:
                fill_missing_keys(target_file, baseline_keys, target_keys, missing)
                print(f"  Filled {len(missing)} keys with baseline values")
        else:
            print(f"\n{target_file}: No missing keys")
    else:
        # Process all files except baseline
        locales = get_all_locales()
        total_filled = 0
        
        for target_file in locales:
            if target_file == baseline_file:
                continue
            
            target_keys = load_target(target_file)
            missing = get_missing_keys(baseline_keys, target_keys)
            
            if missing:
                print(f"\n{target_file}: {len(missing)} missing keys")
                if args.dry_run:
                    for key in missing:
                        print(f"  - {key}: {baseline_keys[key]}")
                else:
                    fill_missing_keys(target_file, baseline_keys, target_keys, missing)
                    print(f"  Filled {len(missing)} keys")
                    total_filled += len(missing)
        
        if not args.dry_run:
            print(f"\nTotal: Filled {total_filled} missing keys")

if __name__ == '__main__':
    main()
