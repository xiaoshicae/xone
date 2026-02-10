# 完整质量检查

运行完整质量检查流水线。

---

请按顺序执行以下检查：

## 1. 格式检查

```bash
gofmt -l .
```

如果有未格式化的文件，自动修复：
```bash
gofmt -w .
```

## 2. 静态分析

```bash
go vet ./...
```

如果有问题，报告并尝试修复。

## 3. 全量测试

```bash
go test -gcflags="all=-N -l" ./... -v
```

## 4. 构建检查

```bash
go build ./...
```

## 5. 输出总结

以表格形式输出检查结果：

| 检查项 | 状态 | 说明 |
|--------|------|------|
| 格式检查 | PASS/FAIL | ... |
| 静态分析 | PASS/FAIL | ... |
| 全量测试 | PASS/FAIL | X passed, Y failed |
| 构建检查 | PASS/FAIL | ... |