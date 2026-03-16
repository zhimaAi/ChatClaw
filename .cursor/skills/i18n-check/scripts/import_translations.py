import os
import re
import json
import argparse

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
FRONTEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'frontend', 'src', 'locales'))
BACKEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'internal', 'services', 'i18n', 'locales'))

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
    """Convert flat dict to nested TS format.

    NOTE:
    - This function assumes `value` is a correct Python str in UTF-8.
    - We ONLY escape double quotes for TS string literals.
    - We deliberately avoid blindly doubling backslashes to prevent turning
      `\"` into `\\\"` in the source file.
    """
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

    def escape_ts_string(value: str) -> str:
        # Only escape double quotes; keep backslashes as-is to avoid over-escaping
        return str(value).replace('"', '\\"')

    def print_object(obj, indent=1):
        indent_str = '  ' * indent
        for key, value in obj.items():
            if key == '_value':
                continue
            if isinstance(value, dict):
                if '_value' in value and len(value) == 1:
                    escaped_value = escape_ts_string(value['_value'])
                    lines.append(f'{indent_str}{key}: "{escaped_value}",')
                else:
                    lines.append(f'{indent_str}{key}: {{')
                    print_object(value, indent + 1)
                    lines.append(f'{indent_str}}},')
            else:
                escaped_value = escape_ts_string(value)
                lines.append(f'{indent_str}{key}: "{escaped_value}",')

    print_object(nested)
    lines.append('}')
    lines.append('')
    return '\n'.join(lines)

def load_ts_file(filepath):
    with open(filepath, 'r', encoding='utf-8') as f:
        return parse_ts_file(f.read())

def save_ts_file(filepath, flat_obj):
    output = to_nested_format(flat_obj)
    with open(filepath, 'w', encoding='utf-8') as f:
        f.write(output)

def load_json_file(filepath):
    with open(filepath, 'r', encoding='utf-8') as f:
        return json.load(f)

def save_json_file(filepath, data):
    with open(filepath, 'w', encoding='utf-8') as f:
        json.dump(data, f, ensure_ascii=False, indent=2)
        f.write('\n')

def parse_translation_file(filepath):
    """Parse translation file (key = value format)"""
    translations = {}
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()
    
    for line in content.split('\n'):
        line = line.strip()
        if not line or line.startswith('#') or line.startswith('##'):
            continue
        if '=' in line:
            parts = line.split('=', 1)
            key = parts[0].strip()
            value = parts[1].strip()
            translations[key] = value
    
    return translations

def import_frontend(target_file, translation_file):
    """Import translations to frontend TS file"""
    target_path = os.path.join(FRONTEND_LOCALES_DIR, target_file)
    target_data = load_ts_file(target_path)

    translations = parse_translation_file(translation_file)

    count = 0
    for key, value in translations.items():
        if key in target_data:
            target_data[key] = value
            count += 1

    save_ts_file(target_path, target_data)
    print(f"Updated {count} translations in {target_file}")

    # Delete the translation export file after successful import
    if os.path.exists(translation_file):
        os.remove(translation_file)
        print(f"Deleted temporary file: {translation_file}")

    return count


def import_backend(target_file, translation_file):
    """Import translations to backend JSON file"""
    target_path = os.path.join(BACKEND_LOCALES_DIR, target_file)
    target_data = load_json_file(target_path)

    translations = parse_translation_file(translation_file)

    count = 0
    for key, value in translations.items():
        if key in target_data:
            target_data[key] = value
            count += 1

    save_json_file(target_path, target_data)
    print(f"Updated {count} translations in {target_file}")

    # Delete the translation export file after successful import
    if os.path.exists(translation_file):
        os.remove(translation_file)
        print(f"Deleted temporary file: {translation_file}")

    return count

def main():
    parser = argparse.ArgumentParser(description='Import translated content to locale files')
    parser.add_argument('--type', '-t', choices=['frontend', 'backend'], default='frontend',
                        help='Type of locale (frontend or backend)')
    parser.add_argument('--target', required=True,
                        help='Target language file (e.g., en-US.ts, en-US.json)')
    parser.add_argument('--file', '-f', required=True,
                        help='Translation file (output from export_translations.py)')
    
    args = parser.parse_args()
    
    if args.type == 'frontend':
        import_frontend(args.target, args.file)
    else:
        import_backend(args.target, args.file)

if __name__ == '__main__':
    main()
