# 集成测试

运行指定模块的集成测试，自动检测并启动所需的 Docker 依赖服务。

使用方法: /integration-test <模块名>

示例:
- /integration-test xredis  # 运行 Redis 集成测试
- /integration-test xgorm   # 运行 PostgreSQL 集成测试

$ARGUMENTS

---

请严格按照以下步骤执行集成测试：

## 1. 参数校验

- 验证 `$ARGUMENTS` 非空，为空则提示用法并终止
- 验证 `test/$ARGUMENTS/` 目录存在，不存在则列出 `test/` 下可用模块并终止

## 2. 依赖检测与 Docker 启动

根据模块名匹配依赖服务：

| 模块 | 依赖 | Docker 镜像 | 端口 | 启动参数 |
|------|------|------------|------|---------|
| xredis | Redis | `redis:7-alpine` | 6379 | - |
| xgorm | PostgreSQL | `postgres:16-alpine` | 5432 | `-e POSTGRES_USER=root -e POSTGRES_PASSWORD=root -e POSTGRES_DB=testdb` |

执行流程：

1. 检查 Docker 是否可用：`docker info > /dev/null 2>&1`，不可用则报错终止
2. 检查目标端口是否已被占用：`lsof -i :PORT`
   - **已占用** → 打印提示"端口 PORT 已被占用，复用现有服务"，跳过启动
   - **未占用** → 启动容器：
     ```bash
     docker run -d --rm --name xone-test-{服务名} -p PORT:PORT {镜像} {参数}
     ```
3. 等待服务就绪（最多重试 15 次，每次间隔 1 秒）：
   - Redis: `docker exec xone-test-redis redis-cli ping` 返回 PONG
   - PostgreSQL: `docker exec xone-test-postgres pg_isready -U root` 返回 accepting connections
4. 如果是 PostgreSQL（xgorm），还需要检查并初始化表结构：
   - 检查 `test/xgorm/single/table_schema.sql` 是否存在
   - 如存在，执行：`docker exec -i xone-test-postgres psql -U root -d testdb < test/xgorm/single/table_schema.sql`
   - 忽略 "already exists" 类错误（幂等）

用变量 `DOCKER_STARTED=true/false` 记录是否是本次启动的容器（用于清理阶段判断）。

如果模块不在上表中，跳过 Docker 步骤，直接进入测试阶段。

## 3. 运行集成测试

### 3.1 临时注释 t.Skip()

查找 `test/$ARGUMENTS/` 下所有 `*_test.go` 文件中的 `t.Skip(` 行，在行首添加 `//` 注释掉：

```bash
# 使用 sed 注释掉 t.Skip 行（macOS 语法）
find test/$ARGUMENTS -name "*_test.go" -exec sed -i '' 's/^[[:space:]]*t\.Skip(/\/\/ &/' {} \;
```

**注意**：记录修改了哪些文件，后续需要恢复。

### 3.2 检测目录结构并运行测试

检查 `test/$ARGUMENTS/` 下是否有子目录（如 single/multi）：

- **有子目录** → 逐个运行每个子目录的测试，分别报告结果：
  ```bash
  go test -gcflags="all=-N -l" ./test/$ARGUMENTS/single/... -v -count=1
  go test -gcflags="all=-N -l" ./test/$ARGUMENTS/multi/... -v -count=1
  ```
- **无子目录** → 直接运行：
  ```bash
  go test -gcflags="all=-N -l" ./test/$ARGUMENTS/... -v -count=1
  ```

## 4. 恢复与清理

无论测试成功还是失败，都必须执行以下清理步骤：

### 4.1 恢复 t.Skip()

还原之前注释掉的 `t.Skip()` 行：

```bash
# 恢复（macOS 语法）
find test/$ARGUMENTS -name "*_test.go" -exec sed -i '' 's/^\/\/ \([[:space:]]*t\.Skip(\)/\1/' {} \;
```

运行 `git diff test/$ARGUMENTS/` 确认文件已完全恢复，如果还有残留差异，使用 `git checkout test/$ARGUMENTS/` 强制恢复。

### 4.2 停止 Docker 容器

仅当 `DOCKER_STARTED=true`（即容器是本次启动的）时执行：

```bash
docker stop xone-test-{服务名}
```

容器使用了 `--rm` 参数，停止后会自动删除。

### 4.3 测试结果摘要

输出格式化的测试结果：

```
=== 集成测试结果 ===
模块: $ARGUMENTS
子测试:
  - single: PASS / FAIL
  - multi:  PASS / FAIL
Docker 服务: 已启动并清理 / 复用已有服务
```