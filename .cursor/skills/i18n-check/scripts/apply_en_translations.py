#!/usr/bin/env python3
"""
Apply en-US translations to other locale files where Chinese placeholder exists.
Replaces Chinese text with English (from en-US) for non-CJK locales.
For zh-TW: converts Simplified Chinese to Traditional Chinese for filled keys.
"""
import os
import re
import sys

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
FRONTEND_LOCALES_DIR = os.path.normpath(
    os.path.join(SCRIPT_DIR, '..', '..', '..', '..', 'frontend', 'src', 'locales')
)

# zh-TW: Simplified to Traditional conversion for the 6 filled keys
ZH_TW_MAP = {
    "配置文档": "配置文檔",
    "钉钉开放平台": "釘釘開放平台",
    "创建机器人，按照": "建立機器人，依照",
    "登录": "登入",
    "完成配置": "完成配置",
    "\u786e\u5b9a\u8981\u5220\u9664\u4efb\u52a1\u201c{name}\u201d\u5417\uff1f\u6b64\u64cd\u4f5c\u65e0\u6cd5\u64a4\u9500\u3002": "\u78ba\u5b9a\u8981\u522a\u9664\u4efb\u52a1\u300c{name}\u300d\u55ce\uff1f\u6b64\u64cd\u4f5c\u7121\u6cd5\u5fa9\u539f\u3002",
}


def detect_chinese(text):
    return bool(re.search(r"[\u4e00-\u9fff]", text))


def parse_ts(content):
    """Simple parse to get flat key->value from TS."""
    sys.path.insert(0, SCRIPT_DIR)
    from fill_frontend import parse_ts_file
    return parse_ts_file(content)


def apply_translations():
    en_path = os.path.join(FRONTEND_LOCALES_DIR, "en-US.ts")
    with open(en_path, "r", encoding="utf-8") as f:
        en_data = parse_ts(f.read())

    skip = {"zh-CN.ts", "en-US.ts"}
    for fname in sorted(os.listdir(FRONTEND_LOCALES_DIR)):
        if not fname.endswith(".ts") or fname in skip or fname == "index.ts":
            continue
        filepath = os.path.join(FRONTEND_LOCALES_DIR, fname)
        with open(filepath, "r", encoding="utf-8") as f:
            content = f.read()
        target = parse_ts(content)
        changed = False
        for key, val in list(target.items()):
            if not detect_chinese(val):
                continue
            if key not in en_data:
                continue
            en_val = en_data[key]
            if fname == "zh-TW.ts":
                new_val = ZH_TW_MAP.get(val, en_val)
            else:
                new_val = en_val
            if new_val != val:
                target[key] = new_val
                changed = True
        if not changed:
            continue
        from fill_frontend import to_nested_format, save_target
        save_target(fname, target)
        print(f"Updated {fname}")


if __name__ == "__main__":
    apply_translations()
    print("Done")
