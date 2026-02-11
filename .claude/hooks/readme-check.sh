#!/bin/bash
# 编辑 .go 文件后，提醒检查对应模块的 README.md
# 此脚本由 Claude Code PostToolUse hook 触发

INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // .tool_input.pathInProject // ""')

# 只处理 .go 文件
if [[ "$FILE_PATH" != *.go ]]; then
  exit 0
fi

# 规范化路径
REL_PATH="$FILE_PATH"
if [[ "$REL_PATH" == "$CLAUDE_PROJECT_DIR"* ]]; then
  REL_PATH="${REL_PATH#"$CLAUDE_PROJECT_DIR"/}"
fi

# 提取顶层模块名（xutil/net.go → xutil, xgin/middleware/log.go → xgin）
MODULE=""
if [[ "$REL_PATH" =~ ^([^/]+)/ ]]; then
  MODULE="${BASH_REMATCH[1]}"
fi

# 无法识别模块时跳过（如根目录的 .go 文件）
if [[ -z "$MODULE" ]]; then
  exit 0
fi

# 检查模块 README 是否存在
MODULE_README="$CLAUDE_PROJECT_DIR/$MODULE/README.md"

if [[ -f "$MODULE_README" ]]; then
  echo "提示：$MODULE 模块的 .go 文件已变更，请在任务完成后检查 $MODULE/README.md 是否需要同步更新（公开 API、配置项、使用示例等）。"
fi