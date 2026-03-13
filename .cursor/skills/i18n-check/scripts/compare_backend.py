import os
import json
import argparse

# Get the directory where this script is located
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
# Go up to project root, then to backend locales
BACKEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'internal', 'services', 'i18n', 'locales'))

DEFAULT_BASELINE = 'zh-CN.json'

def load_json_file(filepath):
    """Load JSON file"""
    with open(filepath, 'r', encoding='utf-8') as f:
        return json.load(f)

def load_baseline(baseline_file):
    """Load baseline JSON file"""
    filepath = os.path.join(BACKEND_LOCALES_DIR, baseline_file)
    return load_json_file(filepath)

def load_target(target_file):
    """Load target JSON file"""
    filepath = os.path.join(BACKEND_LOCALES_DIR, target_file)
    return load_json_file(filepath)

def compare_keys(baseline_data, target_data):
    """Compare baseline and target keys, return missing keys"""
    baseline_keys = set(baseline_data.keys())
    target_keys = set(target_data.keys())
    
    missing = baseline_keys - target_keys
    extra = target_keys - baseline_keys
    
    return sorted(missing), sorted(extra)

def format_output(baseline_file, target_file, missing, extra):
    """Format comparison output"""
    output = []
    output.append(f"Comparing: {baseline_file} (baseline) vs {target_file}")
    output.append("=" * 60)
    
    if missing:
        output.append(f"\nMissing keys in {target_file} ({len(missing)}):")
        for key in missing:
            baseline_value = baseline_data.get(key, '')
            output.append(f"  - {key}: {baseline_value}")
    else:
        output.append(f"\nNo missing keys!")
    
    if extra:
        output.append(f"\nExtra keys in {target_file} ({len(extra)}):")
        for key in extra:
            output.append(f"  + {key}")
    
    return '\n'.join(output)

def compare_files(baseline_file, target_file):
    """Compare two JSON locale files"""
    global baseline_data
    baseline_data = load_baseline(baseline_file)
    target_data = load_target(target_file)
    
    missing, extra = compare_keys(baseline_data, target_data)
    
    print(format_output(baseline_file, target_file, missing, extra))
    
    return missing, extra

def get_all_locales():
    """Get all locale files"""
    files = []
    for f in os.listdir(BACKEND_LOCALES_DIR):
        if f.endswith('.json'):
            files.append(f)
    return sorted(files)

def main():
    parser = argparse.ArgumentParser(description='Compare backend i18n JSON files')
    parser.add_argument('--baseline', '-b', default=DEFAULT_BASELINE, 
                        help=f'Baseline language file (default: {DEFAULT_BASELINE})')
    parser.add_argument('--target', '-t', 
                        help='Target language file to compare (optional, compares all if not specified)')
    parser.add_argument('--list', '-l', action='store_true',
                        help='List all available locale files')
    
    args = parser.parse_args()
    
    print(f"Backend locales dir: {BACKEND_LOCALES_DIR}")
    
    if args.list:
        locales = get_all_locales()
        print(f"\nAvailable locales ({len(locales)}):")
        for f in locales:
            print(f"  - {f}")
        return
    
    baseline_file = args.baseline
    if not baseline_file.endswith('.json'):
        baseline_file += '.json'
    
    if baseline_file not in os.listdir(BACKEND_LOCALES_DIR):
        print(f"Error: Baseline file {baseline_file} not found!")
        return
    
    if args.target:
        target_file = args.target
        if not target_file.endswith('.json'):
            target_file += '.json'
        compare_files(baseline_file, target_file)
    else:
        locales = get_all_locales()
        for target_file in locales:
            if target_file != baseline_file:
                print()
                compare_files(baseline_file, target_file)
                print()

if __name__ == '__main__':
    main()
