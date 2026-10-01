#!/usr/bin/env bash
set -Eeuo pipefail
REPO=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)
TEST_ROOT=$(mktemp -d /tmp/komari-deploy-tests.XXXXXX)
cleanup() { case "$TEST_ROOT" in /tmp/komari-deploy-tests.*) rm -rf -- "$TEST_ROOT" ;; esac; }
trap cleanup EXIT
mkdir -p "$TEST_ROOT/bin"
cat > "$TEST_ROOT/bin/docker" <<'MOCK'
#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$MOCK_LOG"
case "$1" in
  info) [[ ${MOCK_FAIL:-} != info ]] || exit 10; [[ $# == 1 ]] || echo linux; exit 0 ;;
  inspect) echo true; exit 0 ;;
  compose) shift ;;
  *) exit 91 ;;
esac
if [[ ${1:-} == version ]]; then echo 'Docker Compose version v2.mock'; exit 0; fi
[[ $1 == --project-directory && $3 == -f && $4 == "$2/compose.yaml" ]] || exit 92
root=$2; shift 4
case "$1" in
  config)
    [[ ${MOCK_FAIL:-} != config ]] || exit 11
    if [[ ${2:-} == --environment ]]; then
      if [[ -n ${MONITOR_PORT:-} ]]; then echo "MONITOR_PORT=$MONITOR_PORT"
      elif [[ -f $root/.env ]]; then
        value=$(awk -F= '/^MONITOR_PORT=/ {sub(/^[^=]*=/, ""); gsub(/["\047\r]/, ""); value=$0} END {print value}' "$root/.env")
        echo "MONITOR_PORT=$value"
      fi
    fi ;;
  build) [[ ${MOCK_FAIL:-} != build ]] || exit 12 ;;
  up) [[ "$*" == 'up -d --no-build monitor' ]] || exit 93; [[ ${MOCK_FAIL:-} != up ]] || exit 13 ;;
  ps) echo 'isolated-container' ;;
  logs) echo 'mock diagnostics' ;;
  *) exit 94 ;;
esac
MOCK
cat > "$TEST_ROOT/bin/curl" <<'MOCK'
#!/usr/bin/env bash
[[ ${MOCK_FAIL:-} != readiness ]]
MOCK
chmod +x "$TEST_ROOT/bin/docker" "$TEST_ROOT/bin/curl"
export PATH="$TEST_ROOT/bin:$PATH"
unset MONITOR_PORT
new_case() {
  case_root="$TEST_ROOT/$1 with spaces"
  mkdir -p "$case_root/komari-web" "$case_root/data"
  printf 'fixture' > "$case_root/data/keep.txt"
  printf '# keep comment\nOTHER_SETTING=keep=value\nMONITOR_PORT="28000"\n' > "$case_root/.env"
  for file in compose.yaml Dockerfile komari-web/package.json komari-web/package-lock.json; do printf '{}\n' > "$case_root/$file"; done
  export MOCK_LOG="$case_root/commands.log"
  : > "$MOCK_LOG"
  unset MOCK_FAIL
}
run_install() { bash "$REPO/install.sh" --dir "$case_root" "$@" > "$case_root/output.log" 2>&1; }
assert_preserved() {
  [[ $(cat "$case_root/data/keep.txt") == fixture ]]
  [[ ! -e "$case_root/.deploy.lock" ]]
  [[ -z $(find "$case_root" -maxdepth 1 -name '.env.deploy.*' -print) ]]
}
new_case success
run_install --port 28001
[[ $(grep '^MONITOR_PORT=' "$case_root/.env") == MONITOR_PORT=28001 ]]
grep -q '^# keep comment$' "$case_root/.env"
grep -q '^OTHER_SETTING=keep=value$' "$case_root/.env"
assert_preserved
echo 'PASS successful deploy preserves other settings and data'
run_install --skip-build
[[ $(grep '^MONITOR_PORT=' "$case_root/.env") == MONITOR_PORT=28001 ]]
echo 'PASS repeat deployment reuses saved port'
new_case readonly
cp "$case_root/.env" "$case_root/expected.env"
run_install --check --install-docker --port 28002
cmp "$case_root/.env" "$case_root/expected.env"
! grep -Eq ' build | up ' "$MOCK_LOG"
assert_preserved
echo 'PASS check mode is read-only'
new_case invalid
for port in 0 65536 nope -1; do
  if run_install --port "$port"; then echo "invalid port accepted: $port" >&2; exit 1; fi
  [[ ! -s $MOCK_LOG ]]
done
assert_preserved
echo 'PASS invalid ports fail before Docker or filesystem changes'
for reason in build up readiness; do
  new_case "failed-$reason"
  cp "$case_root/.env" "$case_root/expected.env"
  export MOCK_FAIL="$reason"
  if run_install --port 28003 --timeout 1; then echo "failure ignored: $reason" >&2; exit 1; fi
  cmp "$case_root/.env" "$case_root/expected.env"
  if [[ $reason == build ]]; then ! grep -q ' up ' "$MOCK_LOG"; fi
  assert_preserved
  echo "PASS $reason failure keeps saved config/data and cleans installer state"
done
new_case skip
run_install --skip-build
! grep -q ' build ' "$MOCK_LOG"
grep -q ' up -d --no-build monitor$' "$MOCK_LOG"
assert_preserved
echo 'PASS skip-build uses only the existing image'
new_case locked
mkdir "$case_root/.deploy.lock"
if run_install; then echo 'existing lock ignored' >&2; exit 1; fi
[[ -d $case_root/.deploy.lock && ! -s $MOCK_LOG ]]
echo 'PASS existing deployment lock is not removed'
new_case linked
mv "$case_root/.env" "$case_root/target.env"
ln -s "$case_root/target.env" "$case_root/.env"
if run_install --port 28004; then echo 'symlink accepted' >&2; exit 1; fi
[[ -L $case_root/.env ]]
grep -q '^MONITOR_PORT="28000"$' "$case_root/target.env"
echo 'PASS symlink configuration is rejected without touching its target'
new_case fresh
rm "$case_root/.env"
run_install
[[ $(cat "$case_root/.env") == MONITOR_PORT=25774 ]]
[[ $(stat -c '%a' "$case_root/.env") == 600 ]]
assert_preserved
echo 'PASS fresh install persists the default port with private file permissions'
