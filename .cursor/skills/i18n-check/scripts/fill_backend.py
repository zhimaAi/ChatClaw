import os
import json
import argparse

# Get the directory where this script is located
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
BACKEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'internal', 'services', 'i18n', 'locales'))

DEFAULT_BASELINE = 'zh-CN.json'

def load_json_file(filepath):
    """Load JSON file"""
    with open(filepath, 'r', encoding='utf-8') as f:
        return json.load(f)

def save_json_file(filepath, data):
    """Save JSON file"""
    with open(filepath, 'w', encoding='utf-8') as f:
        json.dump(data, f, ensure_ascii=False, indent=2)
        f.write('\n')

def load_baseline(baseline_file):
    """Load baseline JSON file"""
    filepath = os.path.join(BACKEND_LOCALES_DIR, baseline_file)
    return load_json_file(filepath)

def load_target(target_file):
    """Load target JSON file"""
    filepath = os.path.join(BACKEND_LOCALES_DIR, target_file)
    return load_json_file(filepath)

def save_target(target_file, data):
    """Save target JSON file"""
    filepath = os.path.join(BACKEND_LOCALES_DIR, target_file)
    save_json_file(filepath, data)

def get_missing_keys(baseline_data, target_data):
    """Get missing keys from target"""
    baseline_keys = set(baseline_data.keys())
    target_keys = set(target_data.keys())
    return sorted(baseline_keys - target_keys)

def get_all_locales():
    """Get all locale files"""
    files = []
    for f in os.listdir(BACKEND_LOCALES_DIR):
        if f.endswith('.json'):
            files.append(f)
    return sorted(files)

def fill_missing_keys(target_file, baseline_data, target_data, missing_keys):
    """Fill missing keys with baseline values"""
    for key in missing_keys:
        target_data[key] = baseline_data[key]
    
    save_target(target_file, target_data)
    return missing_keys

def main():
    parser = argparse.ArgumentParser(description='Fill missing i18n keys in backend JSON files')
    parser.add_argument('--baseline', '-b', default=DEFAULT_BASELINE, 
                        help=f'Baseline language file (default: {DEFAULT_BASELINE})')
    parser.add_argument('--target', '-t', 
                        help='Target language file to fill (fills all if not specified)')
    parser.add_argument('--dry-run', '-n', action='store_true',
                        help='Show what would be filled without making changes')
    
    args = parser.parse_args()
    
    print(f"Backend locales dir: {BACKEND_LOCALES_DIR}")
    
    baseline_file = args.baseline
    if not baseline_file.endswith('.json'):
        baseline_file += '.json'
    
    if baseline_file not in os.listdir(BACKEND_LOCALES_DIR):
        print(f"Error: Baseline file {baseline_file} not found!")
        return
    
    baseline_data = load_baseline(baseline_file)
    print(f"Baseline: {baseline_file} ({len(baseline_data)} keys)")
    
    if args.target:
        target_file = args.target
        if not target_file.endswith('.json'):
            target_file += '.json'
        
        if target_file not in os.listdir(BACKEND_LOCALES_DIR):
            print(f"Error: Target file {target_file} not found!")
            return
        
        target_data = load_target(target_file)
        missing = get_missing_keys(baseline_data, target_data)
        
        if missing:
            print(f"\n{target_file}: {len(missing)} missing keys")
            if args.dry_run:
                for key in missing:
                    print(f"  - {key}: {baseline_data[key]}")
            else:
                fill_missing_keys(target_file, baseline_data, target_data, missing)
                print(f"  Filled {len(missing)} keys with baseline values")
        else:
            print(f"\n{target_file}: No missing keys")
    else:
        locales = get_all_locales()
        total_filled = 0
        
        for target_file in locales:
            if target_file == baseline_file:
                continue
            
            target_data = load_target(target_file)
            missing = get_missing_keys(baseline_data, target_data)
            
            if missing:
                print(f"\n{target_file}: {len(missing)} missing keys")
                if args.dry_run:
                    for key in missing:
                        print(f"  - {key}: {baseline_data[key]}")
                else:
                    fill_missing_keys(target_file, baseline_data, target_data, missing)
                    print(f"  Filled {len(missing)} keys")
                    total_filled += len(missing)
        
        if not args.dry_run:
            print(f"\nTotal: Filled {total_filled} missing keys")

if __name__ == '__main__':
    main()
