#!/bin/bash
# 编辑模块的 config.go 后，提醒检查 config_schema.json 是否需要同步更新
# 此脚本由 Claude Code PostToolUse hook 触发

INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // .tool_input.pathInProject // ""')

# 规范化路径
REL_PATH="$FILE_PATH"
if [[ "$REL_PATH" == "$CLAUDE_PROJECT_DIR"* ]]; then
  REL_PATH="${REL_PATH#"$CLAUDE_PROJECT_DIR"/}"
fi

# 只处理 */config.go 文件
if [[ "$REL_PATH" =~ ^([^/]+)/config\.go$ ]]; then
  MODULE="${BASH_REMATCH[1]}"
  echo "提示：$MODULE 模块的 config.go 已变更，请在任务完成后检查 config_schema.json 是否需要同步更新（字段增删、默认值、类型、描述等）。"
fi