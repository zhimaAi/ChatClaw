# Git Commit（自动 stage + 自动 commit）

目标：自动把“可安全提交的更改”加入暂存区，生成符合仓库习惯的 commit message，并执行 `git commit`。

提示：建议在 Auto/便宜模型下运行此命令；如果你当前在昂贵模型，请手动切回 Auto 再执行。

## 执行要求

**重要：整个过程保持简洁，减少不必要的输出！**

## 执行步骤（按顺序执行）

1. **读取当前工作区状态**（无需回复，直接执行）：
   - `git status --porcelain=v1`
   - 如无任何更改，仅输出：`❌ 没有可提交的更改` 并停止

2. **安全过滤（防止提交敏感信息）**（无需回复，内部判断）：
   - 任何路径/文件名匹配下列模式都视为“疑似敏感”，**禁止**自动提交：
     - `.env`、`.env.*`
     - `*.pem`、`*.key`、`*.p12`、`*.pfx`
     - `id_rsa*`、`id_ed25519*`
     - `credentials*.json`、`*secret*`、`*token*`
     - `.npmrc`、`.pypirc`
   - 若存在疑似敏感文件处于 modified / staged / untracked：
     - 若已 staged：先执行 `git restore --staged -- <file>`
     - 然后仅输出：`❌ 检测到疑似敏感文件，已阻止提交：<file1>, <file2>...` 并停止

3. **自动加入暂存区**（无需回复，直接执行）：
   - 对所有非敏感变更（包含 untracked/new/modified/deleted/renamed）执行 stage：
     - 推荐逐个文件执行：`git add -A -- <file>`
   - 完成后验证：
     - `git diff --cached --stat`
     - 如果暂存区仍为空，仅输出：`❌ 没有可提交的更改` 并停止

4. **分析变更并生成 commit message**（无需回复，内部处理）：
   - 运行（用于分析 + 参考历史风格）：
     - `git diff --cached --stat`
     - `git diff --cached`
     - `git log -10 --oneline`
   - 判断变更类型：`feat` | `fix` | `docs` | `style` | `refactor` | `build` | `chore` | `test` | `perf`
   - 生成格式：`type(scope): description`（scope 可省略为 `type: description`）
   - 规则：
     - 英文
     - 小写 type/scope
     - 祈使语气
     - 72 字符内
     - 末尾不加句号
     - commit message 不要包含双引号 `"`（避免 PowerShell 引号问题）

5. **执行提交**（仅输出结果）：
   - 执行 `git commit -m "<message>"`（**禁止**添加 `--trailer` 参数）
   - 成功后运行 `git log -1 --oneline` 读取 `<commit hash> <commit message>`

6. **若提交后仍有自动修复产物**（例如 hook 格式化导致工作区变脏）：
   - `git status --porcelain=v1`
   - 若存在非敏感更改，自动 stage 并再提交一次：
     - message 固定为：`chore: apply auto-fixes`
   - 最终以最后一次 `git log -1 --oneline` 作为输出

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

## 类型参考

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
