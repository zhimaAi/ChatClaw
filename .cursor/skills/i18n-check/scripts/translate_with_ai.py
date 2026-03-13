import os
import re
import json
import argparse
import sys

# Set UTF-8 encoding for output
if sys.platform == 'win32':
    import io
    sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8', errors='replace')

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
FRONTEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'frontend', 'src', 'locales'))
BACKEND_LOCALES_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'internal', 'services', 'i18n', 'locales'))

DEFAULT_BASELINE = 'zh-CN'

# Language code to language name mapping
LANGUAGE_NAMES = {
    'en-US': 'English',
    'ar-SA': 'Arabic',
    'bn-BD': 'Bengali',
    'de-DE': 'German',
    'es-ES': 'Spanish',
    'fr-FR': 'French',
    'hi-IN': 'Hindi',
    'it-IT': 'Italian',
    'ja-JP': 'Japanese',
    'ko-KR': 'Korean',
    'pt-BR': 'Portuguese (Brazil)',
    'sl-SI': 'Slovenian',
    'tlh': 'Klingon',
    'tr-TR': 'Turkish',
    'vi-VN': 'Vietnamese',
    'zh-CN': 'Chinese (Simplified)',
    'zh-TW': 'Chinese (Traditional)',
}

# Languages that use Chinese characters but need different handling
# (zh-TW uses Traditional Chinese, ja-JP uses Kanji, ko-KR uses Hanja)
CJK_LANGUAGES = ['zh-TW', 'ja-JP', 'ko-KR']

def detect_chinese(text):
    """Check if text contains Simplified or Traditional Chinese characters"""
    return bool(re.search(r'[\u4e00-\u9fff]', text))

def parse_ts_file(content):
    """Parse TS file to flat dict with dot-notation keys"""
    if isinstance(content, bytes):
        content = content.decode('utf-8')
    
    code = re.sub(r'^export\s+default\s*', '', content.strip())
    code = re.sub(r';\s*$', '', code)

    result = {}
    lines = code.split('\n')
    current_path = []

    for line in lines:
        if not line.strip():
            continue
        if line.strip().startswith('//'):
            continue

        indent = len(line) - len(line.lstrip())
        rel_indent = indent // 2

        while len(current_path) > rel_indent:
            current_path.pop()

        obj_match = re.match(r'^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*:\s*\{', line)
        if obj_match:
            key = obj_match.group(1)
            current_path.append(key)
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

def load_baseline(locale_type='frontend'):
    """Load baseline (zh-CN) file"""
    if locale_type == 'frontend':
        filepath = os.path.join(FRONTEND_LOCALES_DIR, f'{DEFAULT_BASELINE}.ts')
        with open(filepath, 'r', encoding='utf-8') as f:
            return parse_ts_file(f.read())
    else:
        filepath = os.path.join(BACKEND_LOCALES_DIR, f'{DEFAULT_BASELINE}.json')
        with open(filepath, 'r', encoding='utf-8') as f:
            return json.load(f)

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


def get_needs_translation_cjk(target_file, baseline_data, locale_type='frontend'):
    """
    Get keys that need translation for CJK languages (ja-JP, ko-KR, zh-TW).
    Compares with non-CJK languages to find untranslated content.
    """
    lang_code = target_file.replace('.ts', '').replace('.json', '')
    
    if lang_code not in CJK_LANGUAGES:
        return {}
    
    if locale_type == 'frontend':
        filepath = os.path.join(FRONTEND_LOCALES_DIR, target_file)
        target_data = parse_ts_file(open(filepath, 'r', encoding='utf-8').read())
        non_cjk_files = get_non_cjk_locales('frontend')
    else:
        filepath = os.path.join(BACKEND_LOCALES_DIR, target_file)
        target_data = json.load(open(filepath, 'r', encoding='utf-8'))
        non_cjk_files = get_non_cjk_locales('backend')
    
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
    
    needs_translation = {}
    
    for key, target_value in target_data.items():
        if key in ref_data:
            ref_value = ref_data[key]
            # If target equals reference (untranslated), mark for translation
            if target_value == ref_value or not target_value.strip():
                # Also get the baseline (zh-CN) value for reference
                if key in baseline_data:
                    needs_translation[key] = {
                        'target': target_value,
                        'baseline': baseline_data[key],
                        'reference': ref_value
                    }
    
    return needs_translation


def get_needs_translation(target_file, baseline_data, locale_type='frontend', cjk_mode=False):
    """
    Get keys that need translation.
    
    For non-CJK languages: keys with Chinese values need translation
    For CJK languages (zh-TW, ja, ko): compare with non-CJK to find untranslated
    """
    lang_code = target_file.replace('.ts', '').replace('.json', '')
    
    # For CJK languages, use separate detection
    if lang_code in CJK_LANGUAGES:
        if cjk_mode:
            return get_needs_translation_cjk(target_file, baseline_data, locale_type)
        return {}
    
    if locale_type == 'frontend':
        filepath = os.path.join(FRONTEND_LOCALES_DIR, target_file)
        target_data = parse_ts_file(open(filepath, 'r', encoding='utf-8').read())
    else:
        filepath = os.path.join(BACKEND_LOCALES_DIR, target_file)
        target_data = json.load(open(filepath, 'r', encoding='utf-8'))
    
    needs_translation = {}
    
    for key, target_value in target_data.items():
        # For non-CJK: if value contains Chinese, it needs translation
        if detect_chinese(target_value):
            needs_translation[key] = target_value
    
    return needs_translation

def translate_with_ai(translations, target_lang, target_lang_name, source_lang='zh-CN', is_cjk=False):
    """Generate AI translation prompt"""
    if is_cjk:
        # For CJK languages, show baseline, reference, and target
        prompt = f"""Translate the following texts from {source_lang} to {target_lang_name}.
Keep the same meaning and tone. Preserve any placeholders like {{name}}, {{count}}, etc.

Note: Some texts may appear similar to the reference language (non-CJK), but should be properly translated to {target_lang_name}.

Translations:
"""
        for i, (key, data) in enumerate(translations.items(), 1):
            if isinstance(data, dict):
                prompt += f"{i}. [{key}]\n"
                prompt += f"   Baseline ({source_lang}): {data.get('baseline', '')}\n"
                prompt += f"   Reference (non-CJK): {data.get('reference', '')}\n"
                prompt += f"   Current: {data.get('target', '')}\n"
            else:
                prompt += f"{i}. {data}\n"
    else:
        prompt = f"""Translate the following {source_lang} texts to {target_lang_name}. 
Keep the same meaning and tone. Preserve any placeholders like {{name}}, {{count}}, etc.

Translations:
"""
        for i, (key, text) in enumerate(translations.items(), 1):
            prompt += f"{i}. {text}\n"
    
    prompt += f"""

Please respond in the following JSON format:
{{
  "translations": [
    {{"key": "key_name", "original": "original text", "translated": "translated text"}}
  ]
}}

Only respond with JSON, no other text."""

    print("\n" + "="*60)
    print(f"AI TRANSLATION PROMPT FOR {target_lang_name}")
    print("="*60)
    print(prompt)
    print("="*60 + "\n")

def main():
    parser = argparse.ArgumentParser(description='Translate missing i18n keys using AI')
    parser.add_argument('--type', choices=['frontend', 'backend'], default='frontend',
                        help='Type of locale (frontend or backend)')
    parser.add_argument('--target', 
                        help='Target language file (e.g., en-US.ts)')
    parser.add_argument('--all', '-a', action='store_true',
                        help='Translate all languages with Chinese values')
    parser.add_argument('--cjk', action='store_true',
                        help='Also check CJK languages (ja-JP, ko-KR, zh-TW) for untranslated keys')
    
    args = parser.parse_args()
    
    if args.type == 'frontend':
        locale_dir = FRONTEND_LOCALES_DIR
        ext = '.ts'
    else:
        locale_dir = BACKEND_LOCALES_DIR
        ext = '.json'
    
    baseline_data = load_baseline(args.type)
    
    if args.target:
        target_file = args.target
        if not target_file.endswith(ext):
            target_file = target_file + ext
        
        lang_code = target_file.replace(ext, '')
        lang_name = LANGUAGE_NAMES.get(lang_code, lang_code)
        is_cjk = lang_code in CJK_LANGUAGES
        
        needs_trans = get_needs_translation(target_file, baseline_data, args.type, args.cjk)
        
        if needs_trans:
            print(f"Found {len(needs_trans)} keys need translation in {target_file}")
            translate_with_ai(needs_trans, lang_code, lang_name, is_cjk=is_cjk)
        else:
            print(f"No keys need translation in {target_file}")
    else:
        # Check all languages
        files = [f for f in os.listdir(locale_dir) if f.endswith(ext) and f != f'{DEFAULT_BASELINE}{ext}']
        
        all_needs_trans = {}
        for target_file in files:
            lang_code = target_file.replace(ext, '')
            needs_trans = get_needs_translation(target_file, baseline_data, args.type, args.cjk)
            if needs_trans:
                all_needs_trans[target_file] = {
                    'data': needs_trans,
                    'is_cjk': lang_code in CJK_LANGUAGES
                }
        
        if all_needs_trans:
            print(f"Languages needing translation:")
            total = 0
            for target_file, info in all_needs_trans.items():
                lang_code = target_file.replace(ext, '')
                lang_name = LANGUAGE_NAMES.get(lang_code, lang_code)
                print(f"  {lang_name}: {len(info['data'])} items")
                total += len(info['data'])
            print(f"\nTotal: {total} keys need translation")
            
            if args.all:
                for target_file, info in all_needs_trans.items():
                    lang_code = target_file.replace(ext, '')
                    lang_name = LANGUAGE_NAMES.get(lang_code, lang_code)
                    print(f"\n--- Translating {lang_name} ---")
                    translate_with_ai(info['data'], lang_code, lang_name, is_cjk=info['is_cjk'])
        else:
            print("No keys need translation in any locale files.")

if __name__ == '__main__':
    main()
