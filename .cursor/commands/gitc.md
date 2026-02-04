# Git Commit 消息生成器

根据 git 暂存区的更改自动生成符合规范的 commit 消息并执行提交。

## 执行要求

**重要：整个过程保持简洁，减少不必要的输出！**

## 执行步骤

1. **检查暂存区变更**（无需回复，直接执行）：
   - 运行 `git diff --cached --stat` 和 `git diff --cached`
   - 如果没有暂存的更改，仅输出："❌ 没有暂存的更改" 并停止

2. **分析并生成消息**（无需回复，内部处理）：
   - 判断变更类型：`feat`|`fix`|`docs`|`style`|`refactor`|`build`|`chore`|`test`|`perf`
   - 生成格式：`type(scope): description`
   - 规则：英文、小写type/scope、祈使语气、72字符内、无句号

3. **执行提交**（仅输出结果）：
   - 执行 `git commit -m "..."`（**禁止**添加 `--trailer` 参数）
   - 若在 PowerShell 下遇到参数/引号解析问题，改用消息文件：`git commit -F <msg_file>`
   - 成功后运行 `git log -1 --oneline` 验证

## 输出格式

**仅输出以下内容，不要多余解释：**

成功时：
```
✅ <commit hash> <commit message>
```

失败时：
```
❌ <错误原因>
```

## 消息规范

| 类型 | 说明 |
|------|------|
| `feat` | 新功能 |
| `fix` | Bug 修复 |
| `docs` | 文档 |
| `style` | 格式 |
| `refactor` | 重构 |
| `build` | 构建 |
| `chore` | 杂务 |
| `test` | 测试 |
| `perf` | 性能 |
