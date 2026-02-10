---
name: publish
description: 发布 Go module 到 pkg.go.dev，发布前自动检查测试、覆盖率和工作区状态
user-invocable: true
allowed-tools: Bash, Read, Grep, Glob, AskUserQuestion
---

## 发布 Go Module

### 执行步骤

#### 1. 检查 Git 工作区
- 运行 `git status --porcelain` 检查工作区是否干净
- 如果有未提交的变更或未跟踪文件，显示状态并询问用户是否继续（如果用户选择取消则终止）

#### 2. 检查版本 Tag
- 从 `version.go` 读取 `VERSION` 常量获取版本号
- 检查对应的 git tag 是否已存在（`git tag -l <version>`）
- 如果 tag 已存在，提示用户并终止发布

#### 3. 运行质量检查
- 执行 `gofmt -l .`，如果有未格式化文件则输出并终止
- 执行 `go vet ./...`，如果有警告则输出并终止
- 执行 `go test -gcflags="all=-N -l" ./...`，如果失败则输出失败信息并终止

#### 4. 检查增量代码覆盖率
- 获取最近 tag 作为基线
- 获取变更的 Go 模块目录：`git diff <baseline-tag>..HEAD --name-only | grep '\.go$' | xargs -I {} dirname {} | sort -u`
- 对每个变更模块运行覆盖率：`go test -gcflags="all=-N -l" -coverprofile=coverage.out ./<module>/...`
- 如果增量覆盖率 < 60%，输出低覆盖模块并终止

#### 5. 检查 go.mod 一致性
- 运行 `go mod tidy`
- 检查 `go.mod` 和 `go.sum` 是否有变更（`git diff --name-only`）
- 如果有变更，提示用户 go.mod 不一致并终止

#### 6. 检查 README 版本号一致性
- 读取 `README.md` 更新日志的最新版本号
- 与 `version.go` 中的版本号比较
- 如果不一致，提示用户并终止

#### 7. 确认发布
- 使用 `AskUserQuestion` 显示以下信息并询问用户是否确认：
  - 版本号
  - 变更的模块列表
  - 最近的 commit 摘要

#### 8. 创建 Tag 并推送
- 创建 annotated tag：`git tag -a <version> -m "<version>"`
- 推送 tag：`git push origin <version>`
- 推送分支：`git push origin <current-branch>`

#### 9. 触发 pkg.go.dev 索引
- 运行 `GOPROXY=https://proxy.golang.org GO111MODULE=on go list -m github.com/xiaoshicae/xone/v2@<version>` 触发模块索引
- 如果命令失败，提示用户手动访问 pkg.go.dev 页面

#### 10. 创建 GitHub Release
- 运行 `gh release create <version> --title "<version>" --generate-notes` 创建 Release
- 自动根据 PR 和 commit 生成变更说明
- 如果失败（如 tag 已有 Release），提示用户并继续

#### 11. 输出结果
- 显示发布成功信息
- 输出 pkg.go.dev 链接：`https://pkg.go.dev/github.com/xiaoshicae/xone/v2@<version>`
- 输出 GitHub Release 链接：`https://github.com/xiaoshicae/xone/releases/tag/<version>`

### 用法
```
/publish              # 执行完整发布流程
```