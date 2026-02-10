# 运行测试

运行指定模块的测试，如果不指定模块则运行所有测试。

使用方法: /test [模块名]

示例:
- /test        # 运行所有测试
- /test xhttp  # 只运行 xhttp 模块测试

$ARGUMENTS

---

请运行测试命令：

如果提供了模块名参数，运行该模块的测试：
```bash
go test -gcflags="all=-N -l" ./$ARGUMENTS/... -v
```

如果没有提供参数，运行所有测试：
```bash
go test -gcflags="all=-N -l" ./... -v
```

注意：必须使用 `-gcflags="all=-N -l"` 参数，否则 Mockey 框架无法正常工作。