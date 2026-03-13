---
name: i18n-check
description: 检查并补充前端和后端的i18n翻译文件。以中文（zh-CN）为基准，检查其他语言翻译文件是否缺少key，并补充缺失的key和对应的翻译值。
---

# i18n 翻译检查与补充

## 快速开始

1. **格式化翻译文件** (确保格式统一):
   ```bash
   python .cursor/skills/i18n-check/scripts/format_frontend.py
   python .cursor/skills/i18n-check/scripts/format_backend.py
   ```

2. **对比翻译差异**:
   ```bash
   python .cursor/skills/i18n-check/scripts/compare_frontend.py
   python .cursor/skills/i18n-check/scripts/compare_backend.py
   ```

3. **补全缺失的 key** (使用中文作为占位符):
   ```bash
   python .cursor/skills/i18n-check/scripts/fill_frontend.py
   python .cursor/skills/i18n-check/scripts/fill_backend.py
   ```

4. **AI 翻译** (自动检测需要翻译的内容):
   ```bash
   # 翻译特定语言
   python .cursor/skills/i18n-check/scripts/translate_with_ai.py --target en-US

   # 翻译所有语言
   python .cursor/skills/i18n-check/scripts/translate_with_ai.py --all
   ```

## 完整工作流程

```bash
# Step 1: 格式化
python .cursor/skills/i18n-check/scripts/format_frontend.py
python .cursor/skills/i18n-check/scripts/format_backend.py

# Step 2: 对比
python .cursor/skills/i18n-check/scripts/compare_frontend.py
python .cursor/skills/i18n-check/scripts/compare_backend.py

# Step 3: 补全缺失 key (中文占位)
python .cursor/skills/i18n-check/scripts/fill_frontend.py
python .cursor/skills/i18n-check/scripts/fill_backend.py

# Step 4: AI 翻译
# 脚本会自动检测需要翻译的内容并生成翻译提示
python .cursor/skills/i18n-check/scripts/translate_with_ai.py --all
```

## 脚本说明

### 脚本位置
所有脚本位于 `.cursor/skills/i18n-check/scripts/` 目录：

| 脚本 | 用途 |
|------|------|
| `format_frontend.py` | 格式化前端 TS 翻译文件 |
| `compare_frontend.py` | 对比前端翻译差异 |
| `fill_frontend.py` | 补全前端缺失的 key |
| `translate_with_ai.py` | AI 翻译：自动检测需要翻译的内容并生成翻译提示 |
| `import_translations.py` | 导入翻译结果 |
| `format_backend.py` | 格式化后端 JSON 翻译文件 |
| `compare_backend.py` | 对比后端翻译差异 |
| `fill_backend.py` | 补全后端缺失的 key |

### 使用示例

**AI 翻译脚本**
```bash
# 翻译特定语言
python translate_with_ai.py --type frontend --target en-US

# 翻译后端
python translate_with_ai.py --type backend --target ja-JP

# 翻译所有语言
python translate_with_ai.py --all
```

### 翻译检测逻辑

- **非 CJK 语言** (en-US, de-DE, fr-FR 等): 检测含有中文的 key，需要翻译
- **CJK 语言** (zh-TW, ja-JP, ko-KR): 检测与基准文件(zh-CN)相同的 key，需要翻译成对应语言

### AI 翻译流程

1. 运行 `translate_with_ai.py` 脚本
2. 脚本会自动：
   - 读取目标语言文件
   - 检测需要翻译的中文内容
   - 生成 AI 翻译提示 (prompt)
3. 将生成的提示复制给 AI 进行翻译
4. AI 返回 JSON 格式的翻译结果
5. 使用 `import_translations.py` 导入翻译结果

## 文件位置

| 类型 | 目录 | 格式 | 基准文件 |
|------|------|------|----------|
| 前端 | `frontend/src/locales/` | TypeScript `.ts` | `zh-CN.ts` |
| 后端 | `internal/services/i18n/locales/` | JSON `.json` | `zh-CN.json` |

## 注意事项

- **保持 key 结构**: 必须与基准文件完全一致，使用相同的嵌套层级
- **不要删除任何内容**: 只能添加缺失的 key，不能删除现有的 key
- **变量占位符**: 后端 JSON 使用 `{{.xxx}}` 格式，前端使用 `{xxx}` 格式，必须保留
- **格式化后再对比**: 每次对比前先运行格式化脚本，确保格式统一
- **CJK 语言处理**: 繁体中文(zh-TW)、日语(ja-JP)、韩语(ko-KR)使用单独的检测逻辑
