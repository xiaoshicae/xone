
# 单测
UNIT_TEST:
	go test -gcflags="all=-N -l" ./...

# 覆盖率 且 转换为html格式
UNIT_TEST_WITH_COVER_TO_HTML:
	go test -gcflags="all=-N -l" -coverprofile=cover.out ./...
	go tool cover -html=cover.out -o cover.html
