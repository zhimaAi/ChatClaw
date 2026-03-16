---
name: i18n-check
description: 检查并补充前端和后端的i18n翻译文件。以中文（zh-CN）为基准，检查其他语言翻译文件是否缺少key，并补充缺失的key和对应的翻译值。对于日语(ja-JP)、韩语(ko-KR)、繁体中文(zh-TW)，使用与非CJK语言对比的方式检测未翻译内容。
---

# i18n 翻译检查与补充

## 快速开始（推荐安全用法）

> **强制前提**：先用 Git 保证当前工作区是干净的（或至少 locales 相关改动可回滚），再运行下面任何脚本。

1. **只在必要范围格式化翻译文件**（建议从后端/英文开始）

   - 后端 JSON 一般是纯英文，占位少，**优先安全**：

   ```bash
   python .cursor/skills/i18n-check/scripts/format_frontend.py
   python .cursor/skills/i18n-check/scripts/format_backend.py
   ```

   - `format_frontend.py` 会**重写所有 `frontend/src/locales/*.ts`**：
     - 仅做语法级重排与缩进，不再对字符做任何再编码；
     - 仍然建议：**先只在当前分支本地运行，确认 diff 可接受后再提交**。

2. **对比翻译差异（只读，不改文件）**:
   ```bash
   python .cursor/skills/i18n-check/scripts/compare_frontend.py
   python .cursor/skills/i18n-check/scripts/compare_backend.py
   ```

3. **补全缺失的 key（使用中文作为占位符）**

   - **推荐做法：先只对英文和后端补全，再视情况扩展到其他语言。**

   ```bash
   # 仅补前端英文（安全范围小）
   python .cursor/skills/i18n-check/scripts/fill_frontend.py --target en-US

   # 补全所有前端语言（会改动所有 locales，务必在 Git 干净时使用）
   python .cursor/skills/i18n-check/scripts/fill_frontend.py

   # 补全所有后端语言（JSON，风险相对可控）
   python .cursor/skills/i18n-check/scripts/fill_backend.py
   ```

4. **AI 翻译**（自动检测需要翻译的内容，**不会直接改 TS/JSON 文件**）
   ```bash
   # 翻译特定语言
   python .cursor/skills/i18n-check/scripts/translate_with_ai.py --target en-US

   # 翻译所有语言（包括 CJK 语言）
   python .cursor/skills/i18n-check/scripts/translate_with_ai.py --all --cjk
   ```

5. **CJK 语言翻译后检测**（翻译完成后检查是否还有未翻译）
   ```bash
   # 导出 CJK 语言未翻译内容到文本文件
   python .cursor/skills/i18n-check/scripts/export_translations.py --target ja-JP --cjk
   
   # 填充后检测 CJK 语言未翻译 key
   python .cursor/skills/i18n-check/scripts/fill_frontend.py --check-cjk
   ```

## 完整工作流程（推荐顺序）

```bash
# Step 0: 确认 Git 状态
# - 确保 frontend/src/locales 和 internal/services/i18n/locales 内的改动都可回滚
# - 不要在未提交的重要改动上直接批量格式化/补全

# Step 1: 格式化（可选，但推荐先只在后端/英文上尝试）
python .cursor/skills/i18n-check/scripts/format_frontend.py
python .cursor/skills/i18n-check/scripts/format_backend.py

# Step 2: 对比（只读）
python .cursor/skills/i18n-check/scripts/compare_frontend.py
python .cursor/skills/i18n-check/scripts/compare_backend.py

# Step 3: 补全缺失 key（中文占位）
# 先补英文，再按需扩展其他语言
python .cursor/skills/i18n-check/scripts/fill_frontend.py --target en-US
python .cursor/skills/i18n-check/scripts/fill_backend.py

# Step 4: AI 翻译（前端 + 后端）
# 脚本会自动检测需要翻译的内容并生成翻译提示（只读，不改 TS/JSON）
python .cursor/skills/i18n-check/scripts/translate_with_ai.py --all
# 仅处理后端 JSON 时，可显式指定：
# python .cursor/skills/i18n-check/scripts/translate_with_ai.py --type backend --all

# Step 5: 翻译完成后检测 CJK 语言
# 对于 ja-JP, ko-KR, zh-TW，检测是否还有未翻译内容
python .cursor/skills/i18n-check/scripts/fill_frontend.py --check-cjk
python .cursor/skills/i18n-check/scripts/translate_with_ai.py --cjk

# Step 6: 导出未翻译内容到文本，统一翻译后再导入
python .cursor/skills/i18n-check/scripts/export_translations.py --target ja-JP --cjk
# 手动翻译文本文件中的内容
python .cursor/skills/i18n-check/scripts/import_translations.py --file translation_export_ja-JP.txt
```

## 脚本说明

### 脚本位置
所有脚本位于 `.cursor/skills/i18n-check/scripts/` 目录：

| 脚本 | 用途 |
|------|------|
| `format_frontend.py` | 格式化前端 TS 翻译文件 |
| `compare_frontend.py` | 对比前端翻译差异，支持 `--cjk` 检测 CJK 语言 |
| `fill_frontend.py` | 补全前端缺失的 key，支持 `--check-cjk` 检测 CJK 未翻译 |
| `translate_with_ai.py` | AI 翻译：自动检测需要翻译的内容并生成翻译提示，支持 `--cjk` |
| `export_translations.py` | 导出未翻译内容到文本文件，支持 `--cjk` |
| `import_translations.py` | 导入翻译结果 |
| `format_backend.py` | 格式化后端 JSON 翻译文件 |
| `compare_backend.py` | 对比后端翻译差异 |
| `fill_backend.py` | 补全后端缺失的 key |

### 使用示例

**对比 CJK 语言**
```bash
# 对比特定 CJK 语言与英文
python compare_frontend.py --target ja-JP --cjk

# 对比所有 CJK 语言
python compare_frontend.py --cjk-only
```

**填充 CJK 语言**
```bash
# 填充时使用 CJK 模式
python fill_frontend.py --cjk

# 检测 CJK 语言未翻译 key
python fill_frontend.py --check-cjk
```

**AI 翻译脚本**
```bash
# 翻译特定语言
python translate_with_ai.py --type frontend --target en-US

# 翻译 CJK 语言
python translate_with_ai.py --target ja-JP --cjk

# 翻译所有语言（包括 CJK）
python translate_with_ai.py --all --cjk
```

**导出翻译**
```bash
# 导出 CJK 语言未翻译内容
python export_translations.py --target ja-JP --cjk
```

### 翻译检测逻辑

- **非 CJK 语言** (en-US, de-DE, fr-FR 等): 检测含有中文的 key，需要翻译
- **CJK 语言** (zh-TW, ja-JP, ko-KR): 检测与非 CJK 语言（如 en-US）相同的 key，需要翻译成对应语言

### CJK 语言特殊处理

对于日语 (ja-JP)、韩语 (ko-KR)、繁体中文 (zh-TW)，采用以下检测逻辑：

1. **对比方式**: 不以 zh-CN 为基准，而是与英语等非 CJK 语言对比
2. **检测原理**: 如果某个 key 在目标语言中的值与英语相同，说明该 key 未翻译
3. **导出格式**: 显示 baseline(中文) | reference(英语) | current(当前值)，便于翻译

### AI 翻译流程

1. 运行 `translate_with_ai.py` 脚本
2. 脚本会自动：
   - 读取目标语言文件
   - 检测需要翻译的中文内容（或 CJK 未翻译内容）
   - 生成 AI 翻译提示 (prompt)
3. 将生成的提示复制给 AI 进行翻译
4. AI 返回 JSON 格式的翻译结果
5. 使用 `import_translations.py` 导入翻译结果

## 文件位置

| 类型 | 目录 | 格式 | 基准文件 | CJK 基准文件 |
|------|------|------|---------|-------------|
| 前端 | `frontend/src/locales/` | TypeScript `.ts` | `zh-CN.ts` | `en-US.ts` |
| 后端 | `internal/services/i18n/locales/` | JSON `.json` | `zh-CN.json` | `en-US.json` |

## 注意事项

- **保持 key 结构**: 必须与基准文件完全一致，使用相同的嵌套层级
- **不要删除任何内容**: 只能添加缺失的 key，不能删除现有的 key
- **变量占位符**: 后端 JSON 使用 `{{.xxx}}` 格式，前端使用 `{xxx}` 格式，必须保留
- **格式化后再对比**: 每次对比前先运行格式化脚本，确保格式统一
- **CJK 语言处理**: 繁体中文(zh-TW)、日语(ja-JP)、韩语(ko-KR)使用英语(en-US)作为基准文件进行对比和填充
