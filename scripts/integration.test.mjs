import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { zstdDecompressSync } from "node:zlib";

const root = fileURLToPath(new URL("../", import.meta.url));
function archiveEntries() {
  const raw = zstdDecompressSync(fs.readFileSync(path.join(root, "web/public/defaultTheme/dist.tar.zst")));
  const entries = new Map();
  for (let offset = 0; offset + 512 <= raw.length;) {
    const header = raw.subarray(offset, offset + 512);
    if (header.every((byte) => byte === 0)) break;
    const name = header.subarray(0, 100).toString().replace(/\0.*$/, "").replace(/^\.\//, "");
    const size = parseInt(header.subarray(124, 136).toString().replace(/\0/g, "").trim(), 8) || 0;
    if (header[156] === 0 || header[156] === 48) entries.set(name, raw.subarray(offset + 512, offset + 512 + size));
    offset += 512 + Math.ceil(size / 512) * 512;
  }
  return entries;
}

test("the backend archive contains this exact frontend and all entry assets", () => {
  const entries = archiveEntries();
  const index = fs.readFileSync(path.join(root, "komari-web/dist/index.html"));
  assert.deepEqual(entries.get("index.html"), index);
  const assets = [...index.toString().matchAll(/(?:src|href)="(\/assets\/[^"?#]+)/g)].map((match) => match[1].slice(1));
  assert.ok(assets.length > 0);
  for (const asset of assets) {
    assert.deepEqual(entries.get(asset), fs.readFileSync(path.join(root, "komari-web/dist", asset)), asset);
  }
  assert.deepEqual(fs.readFileSync(path.join(root,"web/public/defaultTheme/komari-theme.json")), fs.readFileSync(path.join(root,"komari-web/komari-theme.json")));
});

test("retired remote command clients cannot enter the shipped application", () => {
  const entries = archiveEntries();
  for (const [name, data] of entries) {
    if (!name.endsWith(".js")) continue;
    assert.doesNotMatch(data.toString(), /admin:exec|agent\.terminal\.request|\/api\/admin\/task\/exec|@xterm\//, name);
  }
  assert.ok(entries.has("sw.js"));
});

test("the shipped public layout includes Liquid Kawaii styles and welcome layout", () => {
  const entries = archiveEntries();
  const styles = [...entries].filter(([name]) => name.endsWith(".css") &&
    entries.get(name).toString().includes("--liquid-surface"));
  assert.ok(styles.length > 0, "Liquid theme CSS is missing from the Go archive");
  const scripts = [...entries].filter(([name]) => name.endsWith(".js"));
  assert.ok(scripts.some(([, data]) => data.toString().includes("liquid-hero-wave")),
    "The public layout is still the pre-Liquid version");
  for (const [name, data] of styles) {
    assert.deepEqual(data, fs.readFileSync(path.join(root, "komari-web/dist", name)));
    assert.ok(scripts.some(([, script]) => script.toString().includes(path.basename(name))),
      `Theme stylesheet is not referenced by the shipped application: ${name}`);
  }
});
