#!/usr/bin/env bash
# Optional live network check: GitHub download is real; deployment is stubbed.
set -Eeuo pipefail
REPO=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)
work=$(mktemp -d /tmp/monitor-live-download.XXXXXX)
cleanup() { case "$work" in /tmp/monitor-live-download.*) rm -rf -- "$work" ;; esac; }
trap cleanup EXIT
mkdir "$work/bin"
cat > "$work/bin/docker" <<'MOCK'
#!/usr/bin/env bash
set -eu
case "$1" in
 info) [[ $# == 1 ]] || printf 'linux\n'; exit 0 ;;
 inspect) printf 'true\n'; exit 0 ;;
 compose) shift ;;
 *) exit 1 ;;
esac
[[ $1 != version ]] || exit 0
[[ $1 == --project-directory && $3 == -f ]] || exit 1
shift 4
case "$1" in config|build|up|logs) exit 0 ;; ps) printf 'stub-container\n' ;; *) exit 1 ;; esac
MOCK
printf '#!/usr/bin/env bash\nexit 0\n' > "$work/bin/curl"
chmod +x "$work/bin/docker" "$work/bin/curl"
export PATH="$work/bin:$PATH"
unset MONITOR_PORT
cat "$REPO/install.sh" | bash -s -- --dir "$work/source" --repo oKafuChino/Monitor --ref main
[[ -f $work/source/compose.yaml && -f $work/source/komari-web/package-lock.json ]]
[[ $(git -C "$work/source" remote get-url origin) == https://github.com/oKafuChino/Monitor.git ]]
printf 'PASS real GitHub checkout: '
git -C "$work/source" rev-parse --short HEAD
printf 'Source downloaded and validated; no real service was deployed.\n'
