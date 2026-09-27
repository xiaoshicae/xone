# check.sh / test.sh 共用的函数。调用方先 cd 到仓库根目录再 . scripts/lib.sh
#
# 查的是仓库里的文件：签入的，加上新建了还没 git add 的。不用 find / grep -r：
# .gitignore 挡住的副本（比如 .claude/worktrees 下的工作树）不算，也不该被检查。
# 删了还没暂存的要排掉——index 里还有它，磁盘上已经没了，交给 gofmt 就是一个
# 藏在 $(...) 里的失败，set -e 让脚本一声不吭地退出
files() {
  git ls-files -co --exclude-standard -- "$@" | while IFS= read -r f; do
    if [ -e "$f" ]; then echo "$f"; fi
  done
}

# 仓库里的全部 Go 模块目录（根模块是 .），取法同 files：新加的模块在 git add 之前
# check.sh 已经在查它，test.sh 也得跑它的测试
modules() {
  files go.mod '*/go.mod' | xargs -n1 dirname | sort
}
