# 提交代码

查看当前更改并生成规范的提交信息。

使用方法: /project:commit

---

请执行以下步骤：

1. 运行 `git status` 和 `git diff` 查看当前更改
2. 分析更改内容，生成符合以下格式的提交信息：

```
<type>: <简短描述>

<详细说明（可选）>
```

类型说明：
- `feat`: 新功能
- `fix`: Bug 修复
- `docs`: 文档更新
- `refactor`: 代码重构
- `perf`: 性能优化
- `test`: 测试相关
- `chore`: 构建/工具相关

3. 更新REAME.md & 升级README.md中的版本号

4. 使用 HEREDOC 格式提交：
```bash
git commit -m "$(cat <<'EOF'
提交信息
EOF
)"
```

5. 提交后运行 `git status` 确认状态
