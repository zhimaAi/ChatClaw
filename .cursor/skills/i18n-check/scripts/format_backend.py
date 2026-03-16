import os
import json

# Get the directory where this script is located
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
# Go up to project root, then to backend locales
BACKEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'internal', 'services', 'i18n', 'locales'))

def format_json_file(filepath):
    """Format a JSON file with proper indentation"""
    with open(filepath, 'r', encoding='utf-8') as f:
        data = json.load(f)
    
    # Write back with proper formatting
    with open(filepath, 'w', encoding='utf-8') as f:
        json.dump(data, f, ensure_ascii=False, indent=2)
        f.write('\n')

def process_file(filepath):
    filename = os.path.basename(filepath)
    print(f'Processing: {filename}')
    try:
        format_json_file(filepath)
        print(f'  Formatted: {filename}')
    except Exception as e:
        print(f'  Error: {e}')

def main():
    print(f'Backend locales dir: {BACKEND_LOCALES_DIR}')
    files = [f for f in os.listdir(BACKEND_LOCALES_DIR) if f.endswith('.json')]
    
    for f in files:
        process_file(os.path.join(BACKEND_LOCALES_DIR, f))
    
    print('Done!')

if __name__ == '__main__':
    main()
