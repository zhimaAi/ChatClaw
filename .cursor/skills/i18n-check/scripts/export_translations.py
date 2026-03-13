import os
import re
import json
import argparse

# Get the directory where this script is located
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
FRONTEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'frontend', 'src', 'locales'))
BACKEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'internal', 'services', 'i18n', 'locales'))

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

def export_frontend(target_file, output_file):
    """Export missing Chinese translations for frontend"""
    baseline_file = f"{DEFAULT_BASELINE}.ts"
    baseline_keys = load_baseline_ts(baseline_file)
    target_keys = load_target_ts(target_file)
    
    missing_chinese = get_missing_with_chinese(baseline_keys, target_keys, baseline_keys.keys())
    
    if not missing_chinese:
        print(f"No Chinese translations needed for {target_file}")
        return None
    
    # Generate output
    lines = []
    lines.append(f"# Translation Export: {target_file}")
    lines.append(f"# Total: {len(missing_chinese)} items")
    lines.append("")
    lines.append("## Translations (key = value)")
    lines.append("")
    
    for key, value in sorted(missing_chinese.items()):
        lines.append(f"{key} = {value}")
    
    output_path = os.path.join(SCRIPT_DIR, output_file)
    with open(output_path, 'w', encoding='utf-8') as f:
        f.write('\n'.join(lines))
    
    print(f"Exported {len(missing_chinese)} items to {output_file}")
    return missing_chinese

def export_backend(target_file, output_file):
    """Export missing Chinese translations for backend"""
    baseline_file = f"{DEFAULT_BASELINE}.json"
    baseline_data = load_baseline_json(baseline_file)
    target_data = load_target_json(target_file)
    
    missing_chinese = get_missing_with_chinese(baseline_data, target_data, baseline_data.keys())
    
    if not missing_chinese:
        print(f"No Chinese translations needed for {target_file}")
        return None
    
    # Generate output
    lines = []
    lines.append(f"# Translation Export: {target_file}")
    lines.append(f"# Total: {len(missing_chinese)} items")
    lines.append("")
    lines.append("## Translations (key = value)")
    lines.append("")
    
    for key, value in sorted(missing_chinese.items()):
        lines.append(f"{key} = {value}")
    
    output_path = os.path.join(SCRIPT_DIR, output_file)
    with open(output_path, 'w', encoding='utf-8') as f:
        f.write('\n'.join(lines))
    
    print(f"Exported {len(missing_chinese)} items to {output_file}")
    return missing_chinese

def main():
    parser = argparse.ArgumentParser(description='Export missing translations for AI translation')
    parser.add_argument('--type', '-t', choices=['frontend', 'backend'], default='frontend',
                        help='Type of locale (frontend or backend)')
    parser.add_argument('--target', 
                        help='Target language file (e.g., en-US, ja-JP)')
    parser.add_argument('--output', '-o',
                        help='Output file name (default: translation_export_<lang>.txt)')
    
    args = parser.parse_args()
    
    if args.type == 'frontend':
        if args.target:
            target_file = args.target if args.target.endswith('.ts') else f"{args.target}.ts"
            output_file = args.output or f"translation_export_{args.target}.txt"
            export_frontend(target_file, output_file)
        else:
            # Export for all languages
            locales = [f.replace('.ts', '') for f in os.listdir(FRONTEND_LOCALES_DIR) 
                      if f.endswith('.ts') and f != 'zh-CN.ts' and f != 'index.ts']
            for locale in locales:
                target_file = f"{locale}.ts"
                output_file = f"translation_export_{locale}.txt"
                export_frontend(target_file, output_file)
    else:
        if args.target:
            target_file = args.target if args.target.endswith('.json') else f"{args.target}.json"
            output_file = args.output or f"translation_export_{args.target}.txt"
            export_backend(target_file, output_file)
        else:
            # Export for all languages
            locales = [f.replace('.json', '') for f in os.listdir(BACKEND_LOCALES_DIR) 
                      if f.endswith('.json') and f != 'zh-CN.json']
            for locale in locales:
                target_file = f"{locale}.json"
                output_file = f"translation_export_{locale}.txt"
                export_backend(target_file, output_file)

if __name__ == '__main__':
    main()
