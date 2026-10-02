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
      for key in MONITOR_PORT MONITOR_SHARE_PORT; do
      if [[ -n ${!key:-} ]]; then echo "$key=${!key}"
      elif [[ -f $root/.env ]]; then
        value=$(awk -F= -v key="$key" '$1==key {sub(/^[^=]*=/, ""); gsub(/["\047\r]/, ""); value=$0} END {print value}' "$root/.env")
        echo "$key=$value"
      fi
      done
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
unset MONITOR_PORT MONITOR_SHARE_PORT
new_case() {
  case_root="$TEST_ROOT/$1 with spaces"
  mkdir -p "$case_root/komari-web" "$case_root/data" "$case_root/scripts"
  printf 'fixture' > "$case_root/data/keep.txt"
  printf '# keep comment\nOTHER_SETTING=keep=value\nMONITOR_PORT="28000"\n' > "$case_root/.env"
  for file in install.sh compose.yaml Dockerfile go.mod scripts/embed-frontend.mjs komari-web/package.json komari-web/package-lock.json; do printf '{}\n' > "$case_root/$file"; done
  export MOCK_LOG="$case_root/commands.log"
  : > "$MOCK_LOG"
  unset MOCK_FAIL
}
run_install() { bash "$REPO/install.sh" --dir "$case_root" "$@" > "$TEST_ROOT/last-install.log" 2>&1; }
assert_preserved() {
  [[ $(cat "$case_root/data/keep.txt") == fixture ]]
  [[ ! -e "$case_root/.deploy.lock" ]]
  [[ -z $(find "$case_root" -maxdepth 1 -name '.env.deploy.*' -print) ]]
}
new_case success
run_install --port 28001 --share-port 28011
grep -qx 'MONITOR_SHARE_PORT=28011' "$case_root/.env"
[[ $(grep '^MONITOR_PORT=' "$case_root/.env") == MONITOR_PORT=28001 ]]
grep -q '^# keep comment$' "$case_root/.env"
grep -q '^OTHER_SETTING=keep=value$' "$case_root/.env"
assert_preserved
echo 'PASS successful deploy preserves other settings and data'
run_install --skip-build
grep -qx 'MONITOR_SHARE_PORT=28011' "$case_root/.env"
[[ $(grep '^MONITOR_PORT=' "$case_root/.env") == MONITOR_PORT=28001 ]]
echo 'PASS repeat deployment reuses saved port'
new_case readonly
cp "$case_root/.env" "$case_root/expected.env"
run_install --check --install-docker --port 28002 --share-port 28012
cmp "$case_root/.env" "$case_root/expected.env"
! grep -Eq ' build | up ' "$MOCK_LOG"
assert_preserved
echo 'PASS check mode is read-only'
new_case invalid
for port in 0 65536 nope -1; do
  if run_install --port "$port"; then echo "invalid port accepted: $port" >&2; exit 1; fi
  if run_install --share-port "$port"; then echo "invalid share port accepted: $port" >&2; exit 1; fi
  [[ ! -s $MOCK_LOG ]]
done
assert_preserved
echo 'PASS invalid ports fail before Docker or filesystem changes'
new_case conflict
cp "$case_root/.env" "$case_root/expected.env"
if run_install --port 28000 --share-port 28000; then echo 'port conflict accepted' >&2; exit 1; fi
cmp "$case_root/.env" "$case_root/expected.env"
! grep -Eq ' build | up ' "$MOCK_LOG"
assert_preserved
echo 'PASS conflicting ports do not deploy or change saved settings'
for reason in build up readiness; do
  new_case "failed-$reason"
  cp "$case_root/.env" "$case_root/expected.env"
  export MOCK_FAIL="$reason"
  if run_install --port 28003 --share-port 28013 --timeout 1; then echo "failure ignored: $reason" >&2; exit 1; fi
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
[[ $(cat "$case_root/.env") == $'MONITOR_PORT=25774\nMONITOR_SHARE_PORT=25775' ]]
[[ $(stat -c '%a' "$case_root/.env") == 600 ]]
assert_preserved
echo 'PASS fresh install persists the default port with private file permissions'

# Exercise bootstrap/update with real Git repositories; only network and Docker
# are substituted. No production source or containers are touched.
REAL_GIT=$(command -v git)
export REAL_GIT
export MOCK_REMOTE="$TEST_ROOT/remote"
export GIT_LOG="$TEST_ROOT/git.log"
mkdir -p "$MOCK_REMOTE/komari-web" "$MOCK_REMOTE/scripts"
for file in install.sh compose.yaml Dockerfile go.mod scripts/embed-frontend.mjs komari-web/package.json komari-web/package-lock.json; do printf '{}\n' > "$MOCK_REMOTE/$file"; done
printf '.env\ndata/\n.deploy.lock\n.env.deploy.*\n' > "$MOCK_REMOTE/.gitignore"
printf 'v1\n' > "$MOCK_REMOTE/version.txt"
"$REAL_GIT" init -q -b main "$MOCK_REMOTE"
"$REAL_GIT" -C "$MOCK_REMOTE" config user.name 'Installer test'
"$REAL_GIT" -C "$MOCK_REMOTE" config user.email 'installer@example.invalid'
"$REAL_GIT" -C "$MOCK_REMOTE" add .
"$REAL_GIT" -C "$MOCK_REMOTE" -c commit.gpgsign=false commit -qm 'fixture v1'
"$REAL_GIT" -C "$MOCK_REMOTE" tag v1
cat > "$TEST_ROOT/bin/git" <<'MOCK'
#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$GIT_LOG"
if [[ $1 == clone ]]; then
  [[ ${MOCK_GIT_FAIL:-} != clone ]] || exit 28
  args=("$@")
  url=${args[${#args[@]}-2]}
  dest=${args[${#args[@]}-1]}
  [[ $url == 'https://github.com/oKafuChino/Monitor.git' ]] || exit 98
  args[${#args[@]}-2]="file://$MOCK_REMOTE"
  "$REAL_GIT" "${args[@]}"
  "$REAL_GIT" -C "$dest" remote set-url origin "$url"
elif [[ $1 == -C && $3 == fetch ]]; then
  root=$2; shift 2
  "$REAL_GIT" -C "$root" -c "url.file://$MOCK_REMOTE.insteadOf=https://github.com/oKafuChino/Monitor.git" "$@"
else
  "$REAL_GIT" "$@"
fi
MOCK
chmod +x "$TEST_ROOT/bin/git"
export MOCK_LOG="$TEST_ROOT/bootstrap-docker.log"
: > "$MOCK_LOG"
case_root="$TEST_ROOT/piped checkout"
cat "$REPO/install.sh" | bash -s -- --dir "$case_root" --port 28010 > "$TEST_ROOT/bootstrap.log" 2>&1 || { cat "$TEST_ROOT/bootstrap.log"; exit 1; }
[[ -d $case_root/.git && $(cat "$case_root/version.txt") == v1 ]]
[[ $("$REAL_GIT" -C "$case_root" config monitor.installRepo) == oKafuChino/Monitor ]]
[[ $("$REAL_GIT" -C "$case_root" config monitor.installRef) == main ]]
printf 'fixture' > "$case_root/data/keep.txt"
assert_preserved
echo 'PASS piped installer clones the default GitHub repository into a new directory'
printf 'v2\n' > "$MOCK_REMOTE/version.txt"
"$REAL_GIT" -C "$MOCK_REMOTE" -c commit.gpgsign=false commit -qam 'fixture v2'
cat "$REPO/install.sh" | bash -s -- --dir "$case_root" --update > "$TEST_ROOT/update.log" 2>&1 || { cat "$TEST_ROOT/update.log"; exit 1; }
[[ $(cat "$case_root/version.txt") == v2 ]]
[[ $(cat "$case_root/.env") == $'MONITOR_PORT=28010\nMONITOR_SHARE_PORT=25775' ]]
assert_preserved
echo 'PASS shallow Git checkout fast-forwards while preserving data and port'
printf 'local edit\n' > "$case_root/version.txt"
if run_install --update; then echo 'dirty checkout was overwritten' >&2; exit 1; fi
[[ $(cat "$case_root/version.txt") == 'local edit' ]]
"$REAL_GIT" -C "$case_root" restore -- version.txt
printf 'local untracked' > "$case_root/untracked.txt"
if run_install --update; then echo 'untracked files ignored' >&2; exit 1; fi
[[ $(cat "$case_root/untracked.txt") == 'local untracked' ]]
rm -- "$case_root/untracked.txt"
assert_preserved
echo 'PASS updates reject modified and untracked source files'
"$REAL_GIT" -C "$case_root" remote set-url origin 'https://github.com/example/another.git'
if run_install --update; then echo 'wrong origin accepted' >&2; exit 1; fi
"$REAL_GIT" -C "$case_root" remote set-url origin 'https://github.com/oKafuChino/Monitor.git'
echo 'PASS updates reject a mismatched Git origin'
for args in '--repo https://example.com/repo' '--ref ../main'; do
  # Controlled test tokens, deliberately split as command-line arguments.
  if run_install $args; then echo 'invalid GitHub source accepted' >&2; exit 1; fi
done
echo 'PASS repository and ref arguments are validated'
case_root="$TEST_ROOT/tag checkout"
cat "$REPO/install.sh" | bash -s -- --dir "$case_root" --ref v1 > "$TEST_ROOT/tag.log" 2>&1 || { cat "$TEST_ROOT/tag.log"; exit 1; }
[[ $(cat "$case_root/version.txt") == v1 ]]
[[ $("$REAL_GIT" -C "$case_root" config monitor.installRef) == v1 ]]
echo 'PASS tagged installation records the selected ref'
case_root="$TEST_ROOT/failed clone"
export MOCK_GIT_FAIL=clone
if cat "$REPO/install.sh" | bash -s -- --dir "$case_root" > "$TEST_ROOT/failed-clone.log" 2>&1; then echo 'clone failure ignored' >&2; exit 1; fi
unset MOCK_GIT_FAIL
[[ ! -e $case_root && ! -e "$TEST_ROOT/.failed clone.install.lock" ]]
[[ -z $(find "$TEST_ROOT" -maxdepth 1 -name '.monitor-download.*' -print) ]]
echo 'PASS failed downloads remove only their staging directory and lock'
case_root="$TEST_ROOT/nonempty target"
mkdir -p "$case_root"
printf 'retain' > "$case_root/user-file"
if cat "$REPO/install.sh" | bash -s -- --dir "$case_root" > "$TEST_ROOT/nonempty.log" 2>&1; then echo 'nonempty target overwritten' >&2; exit 1; fi
[[ $(cat "$case_root/user-file") == retain ]]
echo 'PASS bootstrap refuses an unrelated nonempty target'
case_root="$TEST_ROOT/check missing"
if cat "$REPO/install.sh" | bash -s -- --dir "$case_root" --check > "$TEST_ROOT/check-missing.log" 2>&1; then echo 'missing checkout accepted' >&2; exit 1; fi
[[ ! -e $case_root ]]
echo 'PASS read-only checks never download missing source'
cat > "$TEST_ROOT/bin/uname" <<'MOCK'
#!/usr/bin/env bash
printf 'MINGW64_NT\n'
MOCK
chmod +x "$TEST_ROOT/bin/uname"
if bash "$REPO/install.sh" --dir "$TEST_ROOT/piped checkout" --check > "$TEST_ROOT/platform.log" 2>&1; then echo 'non-Linux host accepted' >&2; exit 1; fi
rm -- "$TEST_ROOT/bin/uname"
echo 'PASS unsupported host platforms are rejected'

case_root="$TEST_ROOT/piped checkout"
if run_install --update --ref v1; then echo 'downgrade reported success' >&2; exit 1; fi
grep -q '未降级' "$TEST_ROOT/last-install.log"
[[ $(cat "$case_root/version.txt") == v2 ]]
[[ $("$REAL_GIT" -C "$case_root" config monitor.installRef) == main ]]
echo 'PASS an older ref cannot silently succeed as a downgrade'
printf 'MONITOR_PORT=1\n' > "$MOCK_REMOTE/.env"
"$REAL_GIT" -C "$MOCK_REMOTE" add -f .env
"$REAL_GIT" -C "$MOCK_REMOTE" -c commit.gpgsign=false commit -qm 'fixture accidentally tracks runtime config'
if run_install --update; then echo 'remote runtime config overwrote local settings' >&2; exit 1; fi
grep -q '远端源码包含运行数据路径 .env' "$TEST_ROOT/last-install.log"
[[ $(cat "$case_root/.env") == $'MONITOR_PORT=28010\nMONITOR_SHARE_PORT=25775' ]]
assert_preserved
echo 'PASS upstream runtime files cannot overwrite local persistence'
