import os
import re

# Get the directory where this script is located
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
# Go up to project root, then to frontend/src/locales
FRONTEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'frontend', 'src', 'locales'))

def parse_ts_file(content):
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
    lines = ['export default {']

    nested = {}
    for key, value in flat_obj.items():
        parts = key.split('.')
        current = nested
        for i, part in enumerate(parts[:-1]):
            if part not in current:
                current[part] = {}
            current = current[part]
        current[parts[-1]] = value

    def print_object(obj, indent=1):
        indent_str = '  ' * indent
        for key, value in obj.items():
            if isinstance(value, dict):
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

def process_file(filepath):
    print(f'Processing: {os.path.basename(filepath)}')

    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()

    flat_obj = parse_ts_file(content)

    print(f'  Parsed keys: {len(flat_obj)}')

    if len(flat_obj) == 0:
        print(f'  Could not parse!')
        return

    output = to_nested_format(flat_obj)
    with open(filepath, 'w', encoding='utf-8') as f:
        f.write(output)

    print(f'  Converted: {len(flat_obj)} keys')

def main():
    print(f'Frontend locales dir: {FRONTEND_LOCALES_DIR}')
    files = [f for f in os.listdir(FRONTEND_LOCALES_DIR) if f.endswith('.ts') and f != 'index.ts']

    for f in files:
        process_file(os.path.join(FRONTEND_LOCALES_DIR, f))

    print('Done!')

if __name__ == '__main__':
    main()
