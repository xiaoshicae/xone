#!/bin/bash
# 编辑 .go 文件后自动运行 go vet，快速反馈编译错误
# 此脚本由 Claude Code PostToolUse hook 触发

INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // .tool_input.pathInProject // ""')

if [[ "$FILE_PATH" != *.go ]]; then
  exit 0
fi

# 提取模块目录（xutil/net.go → xutil, xgin/middleware/log.go → xgin/middleware）
REL_PATH="$FILE_PATH"
if [[ "$REL_PATH" == "$CLAUDE_PROJECT_DIR"* ]]; then
  REL_PATH="${REL_PATH#"$CLAUDE_PROJECT_DIR"/}"
fi
MODULE_DIR=$(dirname "$REL_PATH")

cd "$CLAUDE_PROJECT_DIR" || exit 0
OUTPUT=$(go vet "./$MODULE_DIR/..." 2>&1)
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  echo "$OUTPUT"
  cat <<EOF
{"decision": "warn", "message": "go vet 发现编译错误，请修复"}
EOF
fi