# Go Module 版本号规范

## 语义化版本 (SemVer)

版本号格式：`vMAJOR.MINOR.PATCH`

| 字段    | 含义        | 何时递增                  |
|-------|-----------|-----------------------|
| MAJOR | 主版本（不兼容）  | API 破坏性变更（删除/重命名/改签名） |
| MINOR | 次版本（向后兼容） | 新增功能、新增模块             |
| PATCH | 补丁版本      | Bug 修复、性能优化、文档更新      |

## Go Module v2+ 规则

Go 对 v2 及以上版本有**强制要求**：

1. **`go.mod` 的 module 路径必须带 `/v2` 后缀**
   ```
   module github.com/xiaoshicae/xone/v2
   ```

2. **所有内部 import 必须带 `/v2`**
   ```go
   import "github.com/xiaoshicae/xone/v2/xlog"
   import "github.com/xiaoshicae/xone/v2/xconfig"
   ```

3. **下游引用也必须带 `/v2`**
   ```
   require github.com/xiaoshicae/xone/v2 v2.x.x
   ```

### 为什么？

Go module 通过路径区分大版本。不加 `/v2`，Go 工具链会认为只有 v0/v1 版本可用，导致：

- `go get` 无法拉取 v2+ 的 tag
- `+incompatible` 标记仅适用于无 go.mod 的旧仓库，有 go.mod 时会直接报错

## 发版流程

### 发布补丁/次版本

```bash
git tag v2.0.2
git push origin v2.0.2
```

### 发布新主版本（如 v3）

1. 修改 `go.mod`：`module github.com/xiaoshicae/xone/v3`
2. 全局替换 import：`/v2/` → `/v3/`
3. 验证编译：`go build ./...`
4. 打 tag：`git tag v3.0.0 && git push origin v3.0.0`

## 禁止事项

- **禁止** v2+ 的 module 路径不带 `/vN` 后缀
- **禁止** tag 与 go.mod 中的主版本号不一致（如 go.mod 写 `/v2` 但打 `v3.0.0` tag）
- **禁止** 使用 `+incompatible`（本仓库有 go.mod，该标记无效）
- **禁止** 在 v0/v1 中使用 `/v1` 后缀（v0 和 v1 不需要）