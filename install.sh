#!/usr/bin/env bash
# Linux installer. Parse the complete main function before running a piped download.
main() {
set -Eeuo pipefail
SCRIPT_SOURCE=${BASH_SOURCE[0]:-}
ROOT=''
GITHUB_REPO='oKafuChino/Monitor'
GITHUB_REF='main'
REPO_EXPLICIT=0
REF_EXPLICIT=0
UPDATE_SOURCE=0
SOURCE_TEMP=''
SOURCE_PARENT=''
BOOTSTRAP_LOCK=''
BOOTSTRAP_LOCK_OWNED=0
PORT=''
SHARE_PORT=''
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
Komari Linux 一键部署（在 Docker 中构建前后端）
默认 GitHub: https://github.com/oKafuChino/Monitor，分支 main
用法: bash install.sh [选项]
  --port PORT        服务端口，默认沿用已有配置或 25774
  --share-port PORT  临时分享端口，默认沿用已有配置或 25775（默认开启）
  --install-docker   缺少 Docker 时，通过官方 APT 仓库安装（Debian/Ubuntu）
  --skip-build       使用已有 Compose 镜像，不重新构建
  --timeout SECONDS  启动就绪超时，默认 180 秒
  --dir DIRECTORY   安装目录；本地默认源码目录，远程默认 /opt/monitor（root）或 ~/monitor
  --repo OWNER/REPO  GitHub 仓库，默认 oKafuChino/Monitor
  --ref REF          分支或标签，默认 main；后续沿用已保存值
  --update           获取并快进更新源码；拒绝覆盖未提交改动
  --check            只检查依赖与配置，不安装、不启动、不写文件
  -h, --help         显示帮助
远程安装会下载源码；--install-docker 可在 Debian/Ubuntu 补齐 Git 和 Docker。
已有安装默认重新部署当前源码，获取新版本请使用 --update；保留 data/ 与 .env。
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
    --share-port) need_value "$@"; SHARE_PORT=$2; shift 2 ;;
    --timeout) need_value "$@"; TIMEOUT=$2; shift 2 ;;
    --dir) need_value "$@"; ROOT=$2; shift 2 ;;
    --repo) need_value "$@"; GITHUB_REPO=$2; REPO_EXPLICIT=1; shift 2 ;;
    --ref) need_value "$@"; GITHUB_REF=$2; REF_EXPLICIT=1; shift 2 ;;
    --update) UPDATE_SOURCE=1; shift ;;
    --install-docker) INSTALL_DOCKER=1; shift ;;
    --skip-build) SKIP_BUILD=1; shift ;;
    --check) CHECK_ONLY=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) fail "未知选项: $1（使用 --help 查看用法）" ;;
  esac
done
[[ -z $PORT ]] || number_in_range "$PORT" 1 65535 || fail '端口必须是 1–65535 的整数'
[[ -z $SHARE_PORT ]] || number_in_range "$SHARE_PORT" 1 65535 || fail '分享端口必须是 1–65535 的整数'
number_in_range "$TIMEOUT" 1 86400 || fail '超时必须是 1–86400 秒'
TIMEOUT=$((10#$TIMEOUT))
[[ $(uname -s) == Linux ]] || fail '仅支持 Linux 部署'
(( ! CHECK_ONLY || ! UPDATE_SOURCE )) || fail '--check 不会更新源码，请去掉 --update'
SOURCE_FILES=(install.sh compose.yaml Dockerfile go.mod scripts/embed-frontend.mjs komari-web/package.json komari-web/package-lock.json)
source_complete() {
  local file
  for file in "${SOURCE_FILES[@]}"; do
    [[ -f "$1/$file" ]] || return 1
  done
}
if [[ -z $ROOT ]]; then
  if [[ -n $SCRIPT_SOURCE && -f $SCRIPT_SOURCE ]] && source_complete "$(dirname -- "$SCRIPT_SOURCE")"; then ROOT=$(dirname -- "$SCRIPT_SOURCE")
  elif source_complete "$PWD"; then ROOT=$PWD
  elif (( EUID == 0 )); then ROOT=/opt/monitor
  else ROOT="${HOME:?请设置 HOME 或使用 --dir}/monitor"
  fi
fi
[[ ! -L $ROOT ]] || fail '安装目录不能是符号链接'
[[ ! -e $ROOT || -d $ROOT ]] || fail '安装路径已被文件占用'
ROOT=$(realpath -m -- "$ROOT")
if (( CHECK_ONLY )) && ! source_complete "$ROOT"; then fail '源码目录不存在或不完整；--check 不会下载文件，请先安装'; fi
validate_source_options() {
  [[ $GITHUB_REPO =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$ && $GITHUB_REPO != *..* ]] || fail '--repo 必须是 GitHub 的 OWNER/REPO'
  [[ $GITHUB_REF =~ ^[A-Za-z0-9][A-Za-z0-9._/-]*$ && $GITHUB_REF != *..* && $GITHUB_REF != */ && $GITHUB_REF != *//* ]] || fail '--ref 必须是有效的分支或标签'
}
if source_complete "$ROOT" && [[ -d $ROOT/.git ]] && command -v git >/dev/null; then
  if (( ! REPO_EXPLICIT )); then GITHUB_REPO=$(git -C "$ROOT" config --get monitor.installRepo || printf '%s' "$GITHUB_REPO"); fi
  if (( ! REF_EXPLICIT )); then GITHUB_REF=$(git -C "$ROOT" config --get monitor.installRef || printf '%s' "$GITHUB_REF"); fi
fi
validate_source_options
if source_complete "$ROOT" && (( (REPO_EXPLICIT || REF_EXPLICIT) && ! UPDATE_SOURCE && ! CHECK_ONLY )); then
  fail "已有源码选择仓库或版本时需配合 --update，或使用新的空安装目录"
fi
ENV_FILE="$ROOT/.env"
LOCK_DIR="$ROOT/.deploy.lock"
[[ ! -L $ENV_FILE ]] || fail '.env 是符号链接，请改用源码目录内的普通配置文件'
[[ ! -e $ENV_FILE || -f $ENV_FILE ]] || fail '.env 必须是普通文件'

cleanup() {
  [[ -z $ENV_TEMP ]] || rm -f -- "$ENV_TEMP"
  [[ -z $KEY_TEMP ]] || rm -f -- "$KEY_TEMP"
  if (( LOCK_OWNED )); then rmdir -- "$LOCK_DIR" 2>/dev/null || true; fi
  if [[ -n $SOURCE_TEMP ]]; then
    case "$SOURCE_TEMP" in "$SOURCE_PARENT"/.monitor-download.*) rm -rf -- "$SOURCE_TEMP" ;; esac
  fi
  if (( BOOTSTRAP_LOCK_OWNED )); then rmdir -- "$BOOTSTRAP_LOCK" 2>/dev/null || true; fi
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

ensure_git() {
  command -v git >/dev/null && return 0
  if (( ! INSTALL_DOCKER || CHECK_ONLY )); then fail '需要 Git 下载源码；请先安装 Git，或在 Debian/Ubuntu 使用 --install-docker'; fi
  local ID=''
  [[ -r /etc/os-release ]] || fail '无法识别系统，请手动安装 Git'
  . /etc/os-release
  case "$ID" in debian|ubuntu) ;; *) fail '自动安装 Git 仅支持 Debian/Ubuntu' ;; esac
  if (( EUID != 0 )); then command -v sudo >/dev/null || fail '请使用 root 或安装 sudo'; sudo -v; fi
  info '安装 Git 和 HTTPS 证书'
  as_root apt-get update
  as_root apt-get install -y git ca-certificates
}
reject_persistent_source_paths() {
  local path tracked type
  for path in "${SOURCE_FILES[@]}"; do
    type=$(git -C "$1" cat-file -t "$2:$path" 2>/dev/null) || fail "远端版本缺少 $path，未应用更新"
    [[ $type == blob ]] || fail "远端版本的 $path 不是文件"
  done
  for path in .env data cache backup .deploy.lock; do
    tracked=$(git -C "$1" ls-tree --name-only "$2" -- "$path") || fail "无法检查远端文件，已停止"
    [[ -z $tracked ]] || fail "远端源码包含运行数据路径 $path，拒绝覆盖"
  done
}
if ! source_complete "$ROOT"; then
  (( ! SKIP_BUILD )) || fail '首次下载源码不能使用 --skip-build'
  [[ ! -d $ROOT || -z $(find "$ROOT" -mindepth 1 -maxdepth 1 -print -quit) ]] || fail '目标目录非空且不是完整源码目录；请选择空目录，下载脚本可放在 /tmp'
  ensure_git
  SOURCE_PARENT=$(dirname -- "$ROOT")
  mkdir -p -- "$SOURCE_PARENT"
  BOOTSTRAP_LOCK="$SOURCE_PARENT/.$(basename -- "$ROOT").install.lock"
  mkdir -- "$BOOTSTRAP_LOCK" 2>/dev/null || fail '已有进程正在下载到此安装目录'
  BOOTSTRAP_LOCK_OWNED=1
  SOURCE_TEMP=$(mktemp -d "$SOURCE_PARENT/.monitor-download.XXXXXX")
  info "从 GitHub 下载 $GITHUB_REPO ($GITHUB_REF) 到 $ROOT"
  GIT_TERMINAL_PROMPT=0 git clone --depth 1 --single-branch --branch "$GITHUB_REF" -- "https://github.com/$GITHUB_REPO.git" "$SOURCE_TEMP/source"
  source_complete "$SOURCE_TEMP/source" || fail '下载的源码不完整，请检查仓库与分支'
  reject_persistent_source_paths "$SOURCE_TEMP/source" HEAD
  git -C "$SOURCE_TEMP/source" config monitor.installRepo "$GITHUB_REPO"
  git -C "$SOURCE_TEMP/source" config monitor.installRef "$GITHUB_REF"
  if [[ -d $ROOT ]]; then rmdir -- "$ROOT"; fi
  mv -- "$SOURCE_TEMP/source" "$ROOT"
fi
if (( ! CHECK_ONLY )); then
  mkdir -- "$LOCK_DIR" 2>/dev/null || fail '已有部署锁 .deploy.lock；请确认没有其他部署进程运行后再处理该锁'
  LOCK_OWNED=1
fi
if (( UPDATE_SOURCE )); then
  ensure_git
  [[ -d $ROOT/.git && ! -L $ROOT/.git ]] || fail '--update 需要普通 Git 克隆目录；源码压缩包请手动更新'
  origin=$(git -C "$ROOT" remote get-url origin)
  [[ ${origin%.git} == "https://github.com/$GITHUB_REPO" ]] || fail 'Git origin 与指定仓库不一致，拒绝更新；请核对 --repo'
  [[ -z $(git -C "$ROOT" status --porcelain --untracked-files=normal) ]] || fail '源码有未提交改动或未跟踪文件，请先提交/备份；脚本不会覆盖'
  info "获取并快进更新 $GITHUB_REPO ($GITHUB_REF)"
  GIT_TERMINAL_PROMPT=0 git -C "$ROOT" fetch --no-tags origin "$GITHUB_REF"
  target_commit=$(git -C "$ROOT" rev-parse FETCH_HEAD)
  reject_persistent_source_paths "$ROOT" "$target_commit"
  git -C "$ROOT" merge --ff-only "$target_commit"
  [[ $(git -C "$ROOT" rev-parse HEAD) == "$target_commit" ]] || fail "指定版本早于当前版本，未降级；请使用独立安装目录或手动处理版本"
  source_complete "$ROOT" || fail '更新后的源码不完整，未启动服务'
  git -C "$ROOT" config monitor.installRepo "$GITHUB_REPO"
  git -C "$ROOT" config monitor.installRef "$GITHUB_REF"
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
if [[ -z $SHARE_PORT ]]; then
  SHARE_PORT=$(compose config --environment | awk -F= '$1=="MONITOR_SHARE_PORT" {sub(/^[^=]*=/, ""); value=$0} END {print value}')
  SHARE_PORT=${SHARE_PORT:-25775}
fi
number_in_range "$SHARE_PORT" 1 65535 || fail 'MONITOR_SHARE_PORT 必须是 1–65535 的整数'
SHARE_PORT=$((10#$SHARE_PORT))
(( PORT != SHARE_PORT )) || fail '主站端口和分享端口不能相同，请使用 --share-port 指定其他端口'
export MONITOR_SHARE_PORT="$SHARE_PORT"
compose config --quiet
if command -v curl >/dev/null; then
  http_ready() { curl --noproxy '*' -fsSL --max-time 5 -o /dev/null "http://127.0.0.1:$PORT/"; }
elif command -v wget >/dev/null; then
  http_ready() { http_proxy= https_proxy= HTTP_PROXY= HTTPS_PROXY= ALL_PROXY= all_proxy= wget -q -T 5 -O /dev/null "http://127.0.0.1:$PORT/"; }
else
  fail '需要 curl 或 wget 检查网页就绪状态'
fi
if (( CHECK_ONLY )); then info "检查通过；源码: $ROOT；主站端口: $PORT；分享端口: $SHARE_PORT"; exit 0; fi
[[ -w "$ROOT" && ( ! -e $ENV_FILE || -w $ENV_FILE ) ]] || fail '没有写入源码目录或 .env 的权限'

# Prepare both ports. Keep other values, comments and file mode.
ENV_TEMP=$(mktemp "$ROOT/.env.deploy.XXXXXX")
if [[ -f $ENV_FILE ]]; then
  cp -p -- "$ENV_FILE" "$ENV_TEMP"
  content=$(awk -v port="$PORT" -v share_port="$SHARE_PORT" '
    /^[[:space:]]*(export[[:space:]]+)?MONITOR_PORT[[:space:]]*=/ {if (!written++) print "MONITOR_PORT=" port; next}
    /^[[:space:]]*(export[[:space:]]+)?MONITOR_SHARE_PORT[[:space:]]*=/ {if (!share_written++) print "MONITOR_SHARE_PORT=" share_port; next}
    {print}
    END {if (!written) print "MONITOR_PORT=" port; if (!share_written) print "MONITOR_SHARE_PORT=" share_port}
  ' "$ENV_FILE")
  printf '%s\n' "$content" > "$ENV_TEMP"
else
  chmod 600 "$ENV_TEMP"
  printf 'MONITOR_PORT=%s\nMONITOR_SHARE_PORT=%s\n' "$PORT" "$SHARE_PORT" > "$ENV_TEMP"
fi
mkdir -p -- "$ROOT/data"
if (( ! SKIP_BUILD )); then
  info '构建整合镜像；已有服务会继续运行到构建完成'
  compose build monitor
fi
info "启动服务，主站端口 $PORT，分享端口 $SHARE_PORT"
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
info "部署完成：主站 http://127.0.0.1:$PORT；分享监听 http://127.0.0.1:$SHARE_PORT"
printf '分享页面已默认开启。请将独立分享域名通过 HTTPS 反代到分享端口，并在后台「设置 → 站点 → 临时分享节点」保存分享公开地址。\n默认端口仅供服务器本机访问；主站请通过隧道或受控反代访问。\n'
printf '首次访问请按页面向导创建管理员账号。\n数据目录: %s/data\n端口已保存到 .env；获取并部署新版本: bash install.sh --update。\n查看日志: docker compose logs -f monitor（在源码根目录执行）\n' "$ROOT"

}

main "$@"
