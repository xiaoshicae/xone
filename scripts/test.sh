#!/bin/sh
# 跑全仓库的测试。go test ./... 不跨模块边界，所以按模块逐个跑。
# 模块列表的取法和 check.sh 一致（files 照抄自那里）：签入的，加上新建了
# 还没 git add 的，排掉删了还没暂存的。只认签入的话，新加的模块在 add 之前
# check.sh 已经在查它，这里却不跑它的测试
#
# 一个模块红了也接着跑完其余的，最后一起报：一次看全哪些模块红了，
# 不用修一个、重跑一遍、再发现下一个
set -e
cd "$(dirname "$0")/.."
files() {
  git ls-files -co --exclude-standard -- "$@" | while IFS= read -r f; do
    if [ -e "$f" ]; then echo "$f"; fi
  done
}
failed=
for m in $(files go.mod '*/go.mod' | xargs -n1 dirname | sort); do
  echo "── $m"
  (cd "$m" && GOWORK=off go test -race "$@" ./...) || failed="$failed $m"
done
if [ -n "$failed" ]; then
  echo
  echo "✗ 这些模块的测试没过：$failed"
  exit 1
fi
