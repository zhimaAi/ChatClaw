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

4. **导出待翻译内容**:
   ```bash
   # 导出前端所有语言
   python .cursor/skills/i18n-check/scripts/export_translations.py --type frontend

   # 导出特定语言
   python .cursor/skills/i18n-check/scripts/export_translations.py --type frontend --target en-US
   ```

5. **AI 翻译**: 将导出的翻译文件发送给 AI 进行翻译

6. **导入翻译结果**:
   ```bash
   python .cursor/skills/i18n-check/scripts/import_translations.py --type frontend --target en-US.ts --file translation_export_en-US.txt
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

# Step 4: 导出待翻译内容
python .cursor/skills/i18n-check/scripts/export_translations.py --type frontend

# Step 5: AI 翻译
# - 打开生成的 translation_export_*.txt 文件
# - 将内容发送给 AI 进行翻译
# - AI 翻译完成后，复制翻译结果

# Step 6: 导入翻译结果
# - 创建新的翻译文件，格式: key = translated_value
# - 运行导入脚本
python .cursor/skills/i18n-check/scripts/import_translations.py --type frontend --target en-US.ts --file <翻译文件>
```

## 脚本说明

### 脚本位置
所有脚本位于 `.cursor/skills/i18n-check/scripts/` 目录：

| 脚本 | 用途 |
|------|------|
| `format_frontend.py` | 格式化前端 TS 翻译文件 |
| `compare_frontend.py` | 对比前端翻译差异 |
| `fill_frontend.py` | 补全前端缺失的 key |
| `export_translations.py` | 导出待翻译的中文内容 |
| `import_translations.py` | 导入 AI 翻译结果 |
| `format_backend.py` | 格式化后端 JSON 翻译文件 |
| `compare_backend.py` | 对比后端翻译差异 |
| `fill_backend.py` | 补全后端缺失的 key |

### 使用示例

**导出脚本**
```bash
# 导出前端所有语言
python export_translations.py --type frontend

# 导出后端所有语言
python export_translations.py --type backend

# 导出特定语言
python export_translations.py --type frontend --target en-US
python export_translations.py --type backend --target ja-JP
```

**导入脚本**
```bash
# 导入前端翻译结果
python import_translations.py --type frontend --target en-US.ts --file translation_en-US.txt

# 导入后端翻译结果
python import_translations.py --type backend --target en-US.json --file translation_en-US.txt
```

### 导出文件格式

导出的文件格式如下：

```
# Translation Export: en-US.ts
# Total: 15 items

## Translations (key = value)

assistant.settings.workspace.nativeDesc = 直接在本机执行命令，无沙箱隔离。命令拥有当前用户的完整权限。
assistant.settings.workspace.workDirHint = 结构：{basePath}{sep}sessions{sep}<agent_hash>{sep}<conversation_hash>{sep}
...
```

### AI 翻译格式

将上述导出内容发送给 AI，AI 翻译完成后，按以下格式返回：

```
assistant.settings.workspace.nativeDesc = Execute commands directly on the native machine without sandbox isolation. Commands have full permissions of the current user.
assistant.settings.workspace.workDirHint = Structure: {basePath}{sep}sessions{sep}<agent_hash>{sep}<conversation_hash>{sep}
...
```

将 AI 翻译结果保存为 `translation_en-US.txt`，然后运行导入脚本。

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
- **AI 翻译**: 补全 key 后，导出待翻译内容，AI 翻译完成后导入
