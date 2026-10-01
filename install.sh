#!/usr/bin/env bash
# Deploy the source checkout without installing Node.js or Go on the host.
set -Eeuo pipefail

ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
PORT=''
TIMEOUT=180
INSTALL_DOCKER=0
SKIP_BUILD=0
CHECK_ONLY=0
LOCK_OWNED=0
ENV_TEMP=''
KEY_TEMP=''
DOCKER=(docker)

usage() {
  cat <<'HELP'
Komari 一键部署（在 Docker 中构建前后端）
用法: bash install.sh [选项]
  --port PORT        服务端口，默认沿用已有配置或 25774
  --install-docker   缺少 Docker 时，通过官方 APT 仓库安装（Debian/Ubuntu）
  --skip-build       使用已有 Compose 镜像，不重新构建
  --timeout SECONDS  启动就绪超时，默认 180 秒
  --dir DIRECTORY   指定本地源码目录，默认脚本所在目录
  --check            只检查依赖与配置，不安装、不启动、不写文件
  -h, --help         显示帮助
更新: 获取新版本源码后重复运行脚本；保留 data/ 目录。
HELP
}
fail() { printf '\n错误: %s\n' "$*" >&2; exit 1; }
info() { printf '\n[Komari] %s\n' "$*"; }
need_value() { [[ $# -ge 2 && -n $2 ]] || fail "选项 $1 缺少参数"; }
number_in_range() {
  [[ $1 =~ ^[0-9]{1,5}$ ]] && (( 10#$1 >= $2 && 10#$1 <= $3 ))
}
while (( $# )); do
  case "$1" in
    --port) need_value "$@"; PORT=$2; shift 2 ;;
    --timeout) need_value "$@"; TIMEOUT=$2; shift 2 ;;
    --dir) need_value "$@"; ROOT=$2; shift 2 ;;
    --install-docker) INSTALL_DOCKER=1; shift ;;
    --skip-build) SKIP_BUILD=1; shift ;;
    --check) CHECK_ONLY=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) fail "未知选项: $1（使用 --help 查看用法）" ;;
  esac
done
[[ -z $PORT ]] || number_in_range "$PORT" 1 65535 || fail '端口必须是 1–65535 的整数'
number_in_range "$TIMEOUT" 1 86400 || fail '超时必须是 1–86400 秒'
TIMEOUT=$((10#$TIMEOUT))
ROOT=$(cd -- "$ROOT" && pwd -P) || fail '源码目录不存在'
for file in compose.yaml Dockerfile komari-web/package.json komari-web/package-lock.json; do
  [[ -f "$ROOT/$file" ]] || fail "缺少 $file，请获取完整仓库源码后运行"
done
ENV_FILE="$ROOT/.env"
LOCK_DIR="$ROOT/.deploy.lock"
[[ ! -L $ENV_FILE ]] || fail '.env 是符号链接，请改用源码目录内的普通配置文件'
[[ ! -e $ENV_FILE || -f $ENV_FILE ]] || fail '.env 必须是普通文件'

cleanup() {
  [[ -z $ENV_TEMP ]] || rm -f -- "$ENV_TEMP"
  [[ -z $KEY_TEMP ]] || rm -f -- "$KEY_TEMP"
  if (( LOCK_OWNED )); then rmdir -- "$LOCK_DIR" 2>/dev/null || true; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'printf "\n部署未完成，请查看上方错误。已有 data/ 不会被脚本删除。\n" >&2' ERR

as_root() {
  if (( EUID == 0 )); then "$@"; else sudo -- "$@"; fi
}
install_docker() {
  [[ $(uname -s) == Linux && -r /etc/os-release ]] || fail '自动安装仅支持 Debian/Ubuntu Linux'
  # System-owned OS metadata; never source the application's .env file.
  . /etc/os-release
  case "$ID" in debian|ubuntu) ;; *) fail "不支持自动安装 Docker 的系统: $ID，请先手动安装" ;; esac
  local suite="${UBUNTU_CODENAME:-${VERSION_CODENAME:-}}" arch package
  [[ $suite =~ ^[a-z0-9-]+$ ]] || fail '无法确定系统版本代号'
  command -v apt-get >/dev/null && command -v dpkg >/dev/null || fail '自动安装需要 APT 和 dpkg'
  if (( EUID != 0 )); then
    command -v sudo >/dev/null || fail '请使用 root 或安装 sudo 后重试'
    sudo -v
  fi
  if ! command -v docker >/dev/null; then
    for package in docker.io docker-compose docker-doc docker-buildx podman-docker containerd runc; do
      if dpkg-query -W -f='${Status}' "$package" 2>/dev/null | grep -qx 'install ok installed'; then
        fail "检测到可能冲突的 $package；请自行处理现有容器环境后重试，脚本不会卸载它"
      fi
    done
  fi
  info '通过 Docker 官方 APT 仓库安装依赖'
  # Official repository steps: docs.docker.com/engine/install/{debian,ubuntu}/
  as_root apt-get update
  as_root apt-get install -y ca-certificates curl
  arch=$(dpkg --print-architecture)
  [[ $arch =~ ^[a-z0-9]+$ ]] || fail '无法确定系统架构'
  KEY_TEMP=$(mktemp)
  curl -fsSL --retry 3 "https://download.docker.com/linux/$ID/gpg" -o "$KEY_TEMP"
  as_root install -m 0755 -d /etc/apt/keyrings
  as_root install -m 0644 "$KEY_TEMP" /etc/apt/keyrings/docker.asc
  printf 'Types: deb\nURIs: https://download.docker.com/linux/%s\nSuites: %s\nComponents: stable\nArchitectures: %s\nSigned-By: /etc/apt/keyrings/docker.asc\n' "$ID" "$suite" "$arch" |
    as_root tee /etc/apt/sources.list.d/docker.sources >/dev/null
  as_root apt-get update
  if command -v docker >/dev/null; then
    # Do not upgrade/replace an existing engine just to add Compose.
    as_root apt-get install -y docker-compose-plugin
  else
    as_root apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
    if command -v systemctl >/dev/null; then as_root systemctl enable --now docker; fi
  fi
}

if (( ! CHECK_ONLY )); then
  mkdir -- "$LOCK_DIR" 2>/dev/null || fail '已有部署锁 .deploy.lock；请确认没有其他部署进程运行后再处理该锁'
  LOCK_OWNED=1
fi
if ! command -v docker >/dev/null || ! docker compose version >/dev/null 2>&1; then
  if (( INSTALL_DOCKER && ! CHECK_ONLY )); then install_docker; else
    fail '需要 Docker 和 Docker Compose；Debian/Ubuntu 可使用 --install-docker 安装，其他系统请先安装并启动 Docker'
  fi
fi
if ! docker info >/dev/null 2>&1; then
  if (( EUID != 0 )) && command -v sudo >/dev/null && sudo docker info >/dev/null 2>&1; then
    DOCKER=(sudo docker)
  else
    fail '无法连接 Docker，请确认 Docker 已启动且当前用户有权限'
  fi
fi
"${DOCKER[@]}" compose version >/dev/null
[[ $("${DOCKER[@]}" info --format '{{.OSType}}') == linux ]] || fail '需要 Linux 容器模式'
compose() { "${DOCKER[@]}" compose --project-directory "$ROOT" -f "$ROOT/compose.yaml" "$@"; }

if [[ -z $PORT ]]; then
  # Compose handles quoting/interpolation; never execute .env as shell code.
  PORT=$(compose config --environment | awk -F= '$1=="MONITOR_PORT" {sub(/^[^=]*=/, ""); value=$0} END {print value}')
  PORT=${PORT:-25774}
fi
number_in_range "$PORT" 1 65535 || fail 'MONITOR_PORT 必须是 1–65535 的整数'
PORT=$((10#$PORT))
export MONITOR_PORT="$PORT"
compose config --quiet
if command -v curl >/dev/null; then
  http_ready() { curl --noproxy '*' -fsSL --max-time 5 -o /dev/null "http://127.0.0.1:$PORT/"; }
elif command -v wget >/dev/null; then
  http_ready() { wget -q --no-proxy -T 5 -O /dev/null "http://127.0.0.1:$PORT/"; }
else
  fail '需要 curl 或 wget 检查网页就绪状态'
fi
if (( CHECK_ONLY )); then info "检查通过；源码: $ROOT；端口: $PORT"; exit 0; fi
[[ -w "$ROOT" && ( ! -e $ENV_FILE || -w $ENV_FILE ) ]] || fail '没有写入源码目录或 .env 的权限'

# Prepare only the changed setting. Keep other values, comments and file mode.
ENV_TEMP=$(mktemp "$ROOT/.env.deploy.XXXXXX")
if [[ -f $ENV_FILE ]]; then
  cp -p -- "$ENV_FILE" "$ENV_TEMP"
  content=$(awk -v port="$PORT" '
    /^[[:space:]]*(export[[:space:]]+)?MONITOR_PORT[[:space:]]*=/ {if (!written++) print "MONITOR_PORT=" port; next}
    {print}
    END {if (!written) print "MONITOR_PORT=" port}
  ' "$ENV_FILE")
  printf '%s\n' "$content" > "$ENV_TEMP"
else
  chmod 600 "$ENV_TEMP"
  printf 'MONITOR_PORT=%s\n' "$PORT" > "$ENV_TEMP"
fi
mkdir -p -- "$ROOT/data"
if (( ! SKIP_BUILD )); then
  info '构建整合镜像；已有服务会继续运行到构建完成'
  compose build monitor
fi
info "启动服务，端口 $PORT"
compose up -d --no-build monitor
container_id=$(compose ps -q monitor)
[[ -n $container_id && $container_id != *$'\n'* ]] || fail '未找到唯一的 monitor 容器'
deadline=$((SECONDS + TIMEOUT))
ready=0
while (( SECONDS < deadline )); do
  running=$("${DOCKER[@]}" inspect --format '{{.State.Running}}' "$container_id" 2>/dev/null || true)
  if [[ $running == true ]] && http_ready 2>/dev/null; then ready=1; break; fi
  sleep 2
done
if (( ! ready )); then
  compose logs --tail 60 monitor >&2 || true
  fail "服务在 $TIMEOUT 秒内未就绪，未保存新的端口配置。可修复问题后使用相同参数重试"
fi
mv -f -- "$ENV_TEMP" "$ENV_FILE"
ENV_TEMP=''
info "部署完成：http://127.0.0.1:$PORT（远程访问请替换为服务器 IP）"
printf '首次访问请按页面向导创建管理员账号。\n数据目录: %s/data\n端口已保存到 .env；更新源码后重复运行本脚本即可。\n查看日志: docker compose logs -f monitor（在源码根目录执行）\n' "$ROOT"
