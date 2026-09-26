#!/usr/bin/env bash
# 跑 e2e/ 下的真实 Web 服务测试：真的 PostgreSQL、MySQL、Redis、ClickHouse，真的进程和信号。
#
#   scripts/e2e.sh                 # 全部（压测除外）
#   scripts/e2e.sh -run Smoke      # 后面的参数原样交给 go test
#   scripts/e2e.sh --load -run Load  # 连压测一起跑（XONE_E2E_LOAD=1），压测一轮十来分钟
#
# 压测要独占机器：压测器、被测服务、PG 本来就挤在同一台机器上，再有别的负载数字就没法看了。
#
# 服务从哪来，三种都行：
#   - 本机装的：已经在跑就直接用，没在跑就按本机的装法拉起来（pg_ctlcluster / service mysql / redis-server）；
#   - docker compose：docker compose -f e2e/compose.yml up -d --wait，账号密码和这里的默认值一致；
#   - 别处管着的（CI 的服务容器、另一台机器）：XONE_E2E_EXTERNAL=1，或者地址不在本机——
#     这时只等它就绪（最多 60 秒），不去启动。
# 就绪只看 TCP 连不连得上（bash 的 /dev/tcp），不要求装 psql / mysqladmin / redis-cli；
# 装了客户端的话再用测试的账号连一次，账号或库不对在这里说一遍，而不是每个用例各报一遍。
#
# ClickHouse 是可选的：在跑就用；本机的话跑在 Docker 容器 xone-ch 里（XONE_E2E_CH_CONTAINER），
# 停着就 docker start。没有 Docker、没有这个容器或起不来时不算失败——CH 的用例各自跳过
# 并说明原因，其余照跑。事先设了 XONE_E2E_CH=0 就不碰 CH。
# 连接参数都能用环境变量盖掉，默认值和 e2e/harness 里的一致。
#
# 不在 scripts/test.sh 里：那里没有数据库，e2e 的每个测试在 XONE_E2E 不为 1 时都会跳过。
# CI 里是单独的工作流（.github/workflows/e2e.yml），只在 go.mod / go.sum 变了的 PR、每晚和手动时跑。
set -e
cd "$(dirname "$0")/.."

# --load 可以写在任意位置，其余参数原样留给 go test
timeout=30m
for a; do
  shift
  if [ "$a" = --load ]; then
    export XONE_E2E_LOAD=1
    timeout=60m
    continue
  fi
  set -- "$@" "$a"
done

: "${XONE_E2E_PG_ADDR:=127.0.0.1:5432}"
: "${XONE_E2E_PG_USER:=xone}"
: "${XONE_E2E_PG_PASSWORD:=e2e-secret-pw}"
: "${XONE_E2E_PG_DB:=xone_e2e}"
: "${XONE_E2E_REDIS_ADDR:=127.0.0.1:6379}"
: "${XONE_E2E_MYSQL_ADDR:=127.0.0.1:3306}"
: "${XONE_E2E_MYSQL_USER:=xone}"
: "${XONE_E2E_MYSQL_PASSWORD:=e2e-secret-pw}"
: "${XONE_E2E_MYSQL_DB:=xone_e2e}"
# pg_ctlcluster 的版本和集群名，故意不加引号：它是两个参数
: "${XONE_E2E_PG_CLUSTER:=16 main}"
export XONE_E2E_PG_ADDR XONE_E2E_PG_USER XONE_E2E_PG_PASSWORD XONE_E2E_PG_DB XONE_E2E_REDIS_ADDR
export XONE_E2E_MYSQL_ADDR XONE_E2E_MYSQL_USER XONE_E2E_MYSQL_PASSWORD XONE_E2E_MYSQL_DB
: "${XONE_E2E_CH_ADDR:=127.0.0.1:9000}"
: "${XONE_E2E_CH_HTTP_ADDR:=127.0.0.1:8123}"
: "${XONE_E2E_CH_USER:=xone}"
: "${XONE_E2E_CH_PASSWORD:=e2e-secret-pw}"
: "${XONE_E2E_CH_DB:=xone_e2e}"
: "${XONE_E2E_CH_CONTAINER:=xone-ch}"
export XONE_E2E_CH_ADDR XONE_E2E_CH_HTTP_ADDR XONE_E2E_CH_USER XONE_E2E_CH_PASSWORD XONE_E2E_CH_DB

# 地址是 host:port；取 host 时去掉 IPv6 的方括号
host_of() { h=${1%:*}; h=${h#[}; echo "${h%]}"; }
is_local() { case $(host_of "$1") in 127.*|localhost|::1) ;; *) return 1;; esac; }
# tcp_up 地址：连得上就算起来了。只用 bash 自带的 /dev/tcp；timeout 防的是连一个黑洞地址挂住
tcp_up() { timeout 2 bash -c ': <>"/dev/tcp/$0/$1"' "$(host_of "$1")" "${1##*:}" 2>/dev/null; }
has() { command -v "$1" >/dev/null 2>&1; }

# wait_for 次数 命令...：命令成功为止，每 0.2 秒试一次
wait_for() {
  tries=$1
  shift
  i=0
  until "$@" >/dev/null 2>&1; do
    i=$((i + 1))
    [ "$i" -lt "$tries" ] || return 1
    sleep 0.2
  done
}

# ensure 名字 地址 启动命令...：在跑就用；别处管着的、或者本机没有这个启动命令，就只等它；否则拉起来
ensure() {
  what=$1 addr=$2
  shift 2
  if tcp_up "$addr"; then
    echo "✓ $what 已在 $addr 运行"
    return
  fi
  if [ "${XONE_E2E_EXTERNAL:-}" = 1 ] || ! is_local "$addr" || ! has "$1"; then
    echo "== 等 $what 在 $addr 就绪"
    wait_for 300 tcp_up "$addr" || {
      echo "✗ $what 在 $addr 上 60 秒内没连上。本机没装的话：docker compose -f e2e/compose.yml up -d --wait"
      exit 1
    }
  else
    echo "== 启动 $what（$*）"
    "$@" >/dev/null
    wait_for 50 tcp_up "$addr" || { echo "✗ $what 没起来"; exit 1; }
  fi
  echo "✓ $what 已就绪"
}

# ---- PostgreSQL ----
# shellcheck disable=SC2086  # $XONE_E2E_PG_CLUSTER 是两个参数
ensure PostgreSQL "$XONE_E2E_PG_ADDR" pg_ctlcluster $XONE_E2E_PG_CLUSTER start
if has psql && ! PGPASSWORD=$XONE_E2E_PG_PASSWORD psql -h "$(host_of "$XONE_E2E_PG_ADDR")" -p "${XONE_E2E_PG_ADDR##*:}" \
  -U "$XONE_E2E_PG_USER" -d "$XONE_E2E_PG_DB" -tAc 'SELECT 1' >/dev/null 2>&1; then
  cat <<TIP
✗ 用 $XONE_E2E_PG_USER 连不上 $XONE_E2E_PG_ADDR 上的库 $XONE_E2E_PG_DB。第一次跑的话先建账号和库：

  su postgres -c "psql -c \"CREATE ROLE $XONE_E2E_PG_USER LOGIN PASSWORD '<密码>'\""
  su postgres -c "createdb -O $XONE_E2E_PG_USER $XONE_E2E_PG_DB"

密码用 XONE_E2E_PG_PASSWORD 告诉本脚本。
TIP
  exit 1
fi

# ---- MySQL ----
ensure MySQL "$XONE_E2E_MYSQL_ADDR" service mysql start
# 密码经 MYSQL_PWD 交给客户端，不出现在命令行上（ps 看得见命令行）
if has mysql && ! MYSQL_PWD=$XONE_E2E_MYSQL_PASSWORD mysql -h "$(host_of "$XONE_E2E_MYSQL_ADDR")" -P "${XONE_E2E_MYSQL_ADDR##*:}" \
  -u "$XONE_E2E_MYSQL_USER" -D "$XONE_E2E_MYSQL_DB" -Nse 'SELECT 1' >/dev/null 2>&1; then
  cat <<TIP
✗ 用 $XONE_E2E_MYSQL_USER 连不上 $XONE_E2E_MYSQL_ADDR 上的库 $XONE_E2E_MYSQL_DB。第一次跑的话先建账号和库：

  mysql -uroot -e "CREATE DATABASE $XONE_E2E_MYSQL_DB; CREATE USER '$XONE_E2E_MYSQL_USER'@'127.0.0.1' IDENTIFIED BY '<密码>';
    GRANT ALL ON $XONE_E2E_MYSQL_DB.* TO '$XONE_E2E_MYSQL_USER'@'127.0.0.1'"

密码用 XONE_E2E_MYSQL_PASSWORD 告诉本脚本。
TIP
  exit 1
fi

# ---- Redis ----
# 不落盘：脚本此刻的工作目录是仓库根，Redis 的 dir 默认就是它。带着默认的
# save 规则（300 秒内 100 次改动就 BGSAVE），跑一轮 e2e 就会在仓库根留下 dump.rdb
ensure Redis "$XONE_E2E_REDIS_ADDR" redis-server --port "${XONE_E2E_REDIS_ADDR##*:}" --daemonize yes --save '' --appendonly no

# ---- ClickHouse ----
# 起不来就 XONE_E2E_CH=0：harness.RequireCH 据此跳过 CH 的用例，并在跳过原因里说清楚。
# 没有 curl 时退回只看 native 端口连不连得上
ch_ping() {
  if has curl; then curl -sf "http://$XONE_E2E_CH_HTTP_ADDR/ping" >/dev/null 2>&1; else tcp_up "$XONE_E2E_CH_ADDR"; fi
}
# 凭证经 curl -K - 从标准输入给，不出现在命令行上；没有 curl 就不查，交给用例去报
ch_auth() {
  has curl || return 0
  printf 'user = "%s:%s"\n' "$XONE_E2E_CH_USER" "$XONE_E2E_CH_PASSWORD" |
    curl -sf -K - "http://$XONE_E2E_CH_HTTP_ADDR/?database=$XONE_E2E_CH_DB" --data-binary 'SELECT 1' >/dev/null 2>&1
}
ch_skip() {
  echo "⚠ ClickHouse 不可用（$1），CH 的用例会跳过；要跑它们：docker start $XONE_E2E_CH_CONTAINER，或者 docker compose -f e2e/compose.yml up -d --wait"
  export XONE_E2E_CH=0
}
if [ "${XONE_E2E_CH:-}" = 0 ]; then
  echo "⚠ XONE_E2E_CH=0，CH 的用例会跳过"
elif ch_ping; then
  echo "✓ ClickHouse 已在 $XONE_E2E_CH_ADDR 运行"
elif [ "${XONE_E2E_EXTERNAL:-}" = 1 ] || ! is_local "$XONE_E2E_CH_ADDR"; then
  echo "== 等 ClickHouse 在 $XONE_E2E_CH_ADDR 就绪"
  if wait_for 300 ch_ping; then echo "✓ ClickHouse 已就绪"; else ch_skip "$XONE_E2E_CH_ADDR 上 60 秒内没连上"; fi
elif ! has docker; then
  ch_skip "没有 docker 命令"
elif ! docker inspect "$XONE_E2E_CH_CONTAINER" >/dev/null 2>&1; then
  ch_skip "没有名为 $XONE_E2E_CH_CONTAINER 的容器"
else
  echo "== 启动 ClickHouse（docker start $XONE_E2E_CH_CONTAINER）"
  if docker start "$XONE_E2E_CH_CONTAINER" >/dev/null 2>&1; then
    # ClickHouse 冷启动比 PG 慢，给到 30 秒
    wait_for 150 ch_ping || true
  fi
  if ch_ping; then echo "✓ ClickHouse 已启动"; else ch_skip "docker start 之后 30 秒仍没有就绪，看 docker logs $XONE_E2E_CH_CONTAINER"; fi
fi
if [ "${XONE_E2E_CH:-}" != 0 ] && ! ch_auth; then
  ch_skip "用 $XONE_E2E_CH_USER 连不上 $XONE_E2E_CH_HTTP_ADDR 上的库 $XONE_E2E_CH_DB，密码用 XONE_E2E_CH_PASSWORD 告诉本脚本"
fi

# 和 scripts/test.sh 一样 GOWORK=off：测的是 e2e/go.mod 自己解出来的依赖
cd e2e
export XONE_E2E=1 GOWORK=off
exec go test -count=1 -v -timeout "$timeout" ./... "$@"
