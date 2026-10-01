import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const source = fs.readFileSync(new URL("../komari-agent/install.sh", import.meta.url), "utf8");
const bash = process.env.AGENT_TEST_BASH || (process.platform === "win32" ? "D:/Git/bin/bash.exe" : "bash");
const hasBash = spawnSync(bash, ["--version"]).status === 0;
const hasJq = spawnSync("jq", ["--version"]).status === 0;
if (process.env.CI) {
  assert.ok(hasBash && hasJq, "CI must provide Bash and jq; installer regressions must not be skipped");
}
// Run only the acquisition phase, never install a service or use the network.
const acquisition = source.split("# Detect init system and configure service")[0]
  .replace(/uninstall_previous\(\) \{[\s\S]*?\n\}\n\n# Keep the existing service/, 'uninstall_previous() { printf "replaced\\n" >> "$MOCK_SERVICE_LOG"; }\n\n# Keep the existing service');
assert.doesNotMatch(acquisition, /rm -f "\/etc\//);

function runFixture(t, options = {}) {
  const dir = fs.mkdtempSync(fileURLToPath(new URL("../.installer-test-", import.meta.url)));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  fs.writeFileSync(path.join(dir, "agent"), "existing-agent");
  const shellPath = (value) => value.replaceAll("\\", "/").replace(/^([A-Za-z]):/, (_, drive) => `/${drive.toLowerCase()}`);
  const env = { ...process.env, SUDO_USER: "root", MOCK_TARGET: shellPath(dir),
    MOCK_SERVICE_LOG: shellPath(path.join(dir, "service.log")),
    MOCK_SELECTION: options.selection ?? "v2.0\tfalse", MOCK_DOWNLOAD_FAIL: options.failDownload ? "yes" : "no" };
  const stubs = `
id() { case "$1" in -u) printf '0\\n';; *) printf 'root\\n';; esac; }
uname() { case "$1" in -s) printf 'Linux\\n';; *) printf 'x86_64\\n';; esac; }
jq() { input=$(command cat); printf '%s\\n' "$MOCK_SELECTION"; }
curl() {
  out=''
  while [ "$#" -gt 0 ]; do
    case "$1" in -o) out="$2"; shift 2;; *) shift;; esac
  done
  if [ -z "$out" ]; then printf '[]\\n'; return 0; fi
  if [ "$MOCK_DOWNLOAD_FAIL" = yes ]; then return 22; fi
  printf 'replacement-agent' > "$out"
}
set -- --install-dir "$MOCK_TARGET" --install-no-mirror -e http://example.invalid -t secret-test-token ${options.version ? `--install-version ${options.version}` : ""}
`;
  const result = spawnSync(bash, ["-s"], { input: stubs + acquisition, env, encoding: "utf8", timeout: 10_000 });
  assert.ifError(result.error);
  return { result, dir, output: result.stdout + result.stderr };
}

test("installer syntax and readonly Bash EUID", { skip: !hasBash }, (t) => {
  assert.equal(spawnSync(bash, ["-n"], { input: source }).status, 0);
  const { result, output, dir } = runFixture(t);
  assert.equal(result.status, 0, output);
  assert.doesNotMatch(output, /readonly variable|secret-test-token/);
  assert.match(output, /releases\/download\/v2.0\/komari-agent-linux-amd64/);
  assert.equal(fs.readFileSync(path.join(dir, "agent"), "utf8"), "replacement-agent");
  assert.equal(fs.readFileSync(path.join(dir, "service.log"), "utf8"), "replaced\n");
});

test("default installation warns when only Snapshot assets are available", { skip: !hasBash }, (t) => {
  const { result, output } = runFixture(t, { selection: "Snapshot-2610020100\ttrue" });
  assert.equal(result.status, 0, output);
  assert.match(output, /No stable release ships/);
  assert.match(output, /releases\/download\/Snapshot-2610020100\/komari-agent-linux-amd64/);
});

for (const scenario of [{ selection: "" }, { failDownload: true }, { version: "../bad" }]) {
  test(`failed acquisition preserves the existing installation: ${JSON.stringify(scenario)}`, { skip: !hasBash }, (t) => {
    const { result, dir, output } = runFixture(t, scenario);
    assert.equal(result.status, 1, output);
    assert.equal(fs.readFileSync(path.join(dir, "agent"), "utf8"), "existing-agent");
    assert.equal(fs.existsSync(path.join(dir, "service.log")), false);
    assert.deepEqual(fs.readdirSync(dir), ["agent"]);
  });
}

const filter = source.match(/--arg asset "\$file_name" --arg channel "\$release_channel" '([\s\S]*?)' 2>\/dev\/null/)[1];
const asset = { name: "komari-agent-linux-amd64", state: "uploaded" };
const release = (tag_name, prerelease, assets = [asset], extra = {}) =>
  ({ tag_name, prerelease, assets, draft: false, published_at: tag_name, ...extra });
test("actual jq selection excludes server-only/draft/incomplete assets and respects channels", { skip: !hasJq }, () => {
  const fixtures = [release("v1", false), release("v2", false, []), release("v3", false, [asset], { draft: true }),
    release("Snapshot-2", true), release("Snapshot-3", true, [{ ...asset, state: "new" }]), release("beta4", true)];
  const select = (channel, values) => {
    const result = spawnSync("jq", ["-r", "--arg", "asset", asset.name, "--arg", "channel", channel, filter],
      { input: JSON.stringify(values), encoding: "utf8" });
    assert.equal(result.status, 0, result.stderr);
    return result.stdout.trim();
  };
  assert.equal(select("auto", fixtures), "v1\tfalse");
  assert.equal(select("stable", fixtures), "v1\tfalse");
  assert.equal(select("snapshot", fixtures), "Snapshot-2\ttrue");
  assert.equal(select("auto", fixtures.filter((r) => r.prerelease)), "Snapshot-2\ttrue");
  assert.equal(select("stable", fixtures.filter((r) => r.prerelease)), "");
  assert.equal(select("tag", release("v1", false)), "v1\tfalse");
  assert.equal(select("auto", []), "");
});
