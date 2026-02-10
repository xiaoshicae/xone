# 提交代码

查看当前更改并生成规范的提交信息。

---

请执行以下步骤：

## 1. 查看当前更改

运行 `git status` 和 `git diff` 查看当前更改，分析变更涉及的模块。

## 2. 运行质量检查

```bash
gofmt -w .
go vet ./...
go test -gcflags="all=-N -l" ./...
```

如果测试失败，停止提交流程并提示用户修复。

## 3. 检查增量代码覆盖率

对变更的模块运行覆盖率检查：

```bash
# 获取变更的模块目录
git diff --name-only | grep '\.go$' | xargs -I {} dirname {} | sort -u

# 对每个变更模块运行覆盖率
go test -gcflags="all=-N -l" -coverprofile=coverage.out ./<module>/...
go tool cover -func=coverage.out
```

要求：增量代码覆盖率需达到 **60%** 以上。如果覆盖率不足，提示用户补充测试。

## 4. 分析提交类型并更新版本号

根据更改内容确定提交类型：

| 类型 | 说明 | 版本号变更 |
|------|------|-----------|
| `feat` | 新功能 | minor +1, patch 归零 (如 v1.1.5 → v1.2.0) |
| `fix` | Bug 修复 | patch +1 (如 v1.1.5 → v1.1.6) |
| `refactor` | 代码重构 | patch +1 |
| `perf` | 性能优化 | patch +1 |
| `docs` | 文档更新 | 不变更版本号 |
| `test` | 测试相关 | 不变更版本号 |
| `chore` | 构建/工具相关 | 不变更版本号 |

**版本号更新步骤**（feat/fix/refactor/perf 类型时）：

1. 读取项目根目录 `version.go` 中的 `VERSION` 常量，获取当前版本号
2. 按上表规则计算新版本号
3. 更新 `version.go` 中的 `VERSION` 常量
4. 在 `README.md` 的"更新日志"部分顶部添加新版本记录：
   ```
   - **vX.Y.Z** (YYYY-MM-DD) - <type>: <简短描述>
   ```

## 5. 生成提交信息

生成符合以下格式的英文提交信息：

```
<type>: <短描述>

<详细说明（可选）>

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
```

## 6. 执行提交

```bash
git add <相关文件>
git commit -m "$(cat <<'EOF'
提交信息
EOF
)"
```

注意：
- 禁止使用 `git add -A`，只添加相关文件
- 不要提交 .env、credentials 等敏感文件
- 不要自动 push，除非用户明确要求

## 7. 确认提交状态

提交后运行 `git status` 确认状态。

---

## 提交前检查清单

- [ ] 质量检查全部通过（fmt + vet + test）
- [ ] 增量代码覆盖率 >= 60%
- [ ] version.go 版本号已正确更新（feat/fix/refactor/perf）
- [ ] README.md 更新日志已添加
- [ ] 提交信息格式规范（英文）