---
name: i18n-check
description: 检查并补充前端和后端的i18n翻译文件。以中文（zh-CN）为基准，检查其他语言翻译文件是否缺少key，并补充缺失的key和对应的翻译值。
---

# i18n 翻译检查与补充

## 快速开始

1. **运行脚本进行格式化** (确保格式统一):
   ```bash
   # 格式化前端 TS 文件
   python .cursor/skills/i18n-check/scripts/format_frontend.py

   # 格式化后端 JSON 文件
   python .cursor/skills/i18n-check/scripts/format_backend.py
   ```

2. **对比翻译差异**:
   ```bash
   # 对比前端所有语言
   python .cursor/skills/i18n-check/scripts/compare_frontend.py

   # 对比后端所有语言
   python .cursor/skills/i18n-check/scripts/compare_backend.py

   # 对比特定语言
   python .cursor/skills/i18n-check/scripts/compare_frontend.py -t ja-JP
   python .cursor/skills/i18n-check/scripts/compare_backend.py -t ja-JP
   ```

## 脚本说明

### 脚本位置
所有脚本位于 `.cursor/skills/i18n-check/scripts/` 目录：

| 脚本 | 用途 |
|------|------|
| `format_frontend.py` | 格式化前端 TS 翻译文件 |
| `compare_frontend.py` | 对比前端 TS 翻译文件 |
| `format_backend.py` | 格式化后端 JSON 翻译文件 |
| `compare_backend.py` | 对比后端 JSON 翻译文件 |

### 格式化脚本

**format_frontend.py**
- 读取 `frontend/src/locales/*.ts` 文件
- 解析嵌套的 TypeScript 对象为扁平结构
- 重新格式化为每行一个 key-value 的格式
- 使用 2 空格缩进

**format_backend.py**
- 读取 `internal/services/i18n/locales/*.json` 文件
- 使用 `json.dump` 格式化，2 空格缩进

### 对比脚本

**compare_frontend.py**
```
用法: compare_frontend.py [选项]

选项:
  -b, --baseline BASELINE   基准语言文件 (默认: zh-CN.ts)
  -t, --target TARGET       目标语言文件 (不指定则对比所有)
  -l, --list               列出所有可用语言文件
```

**compare_backend.py**
```
用法: compare_backend.py [选项]

选项:
  -b, --baseline BASELINE   基准语言文件 (默认: zh-CN.json)
  -t, --target TARGET       目标语言文件 (不指定则对比所有)
  -l, --list               列出所有可用语言文件
```

## 工作流程

### Step 1: 格式化翻译文件

```bash
# 格式化前端
python .cursor/skills/i18n-check/scripts/format_frontend.py

# 格式化后端
python .cursor/skills/i18n-check/scripts/format_backend.py
```

### Step 2: 对比差异

```bash
# 对比前端所有语言 vs zh-CN
python .cursor/skills/i18n-check/scripts/compare_frontend.py

# 对比后端所有语言 vs zh-CN
python .cursor/skills/i18n-check/scripts/compare_backend.py

# 对比特定语言
python .cursor/skills/i18n-check/scripts/compare_frontend.py -t ja-JP
python .cursor/skills/i18n-check/scripts/compare_backend.py -t ja-JP
```

### Step 3: 补充缺失翻译

对于每个缺失的 key：
1. **如果 key 包含明确的可翻译内容**（如按钮文本、提示信息、错误消息等），使用机器翻译生成目标语言的值
2. **如果 key 是技术术语或占位符**，保持与英文版本一致或使用英文原值
3. **如果 value 是空字符串或未定义**，标记为需要人工审核

### Step 4: 验证

检查修改后的文件：
- 前端: 确保 TypeScript 语法有效
- 后端: 确保 JSON 格式有效

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
