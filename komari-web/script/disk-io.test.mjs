import test from "node:test";
import assert from "node:assert/strict";
import { parseDiskIO, diskIOStatusKey, formatDiskIORate, breakDiskIOGaps } from "../src/utils/diskIO.ts";
import fs from "node:fs";
import ts from "typescript";
const recordHelper = ts.transpileModule(fs.readFileSync(new URL("../src/utils/RecordHelper.tsx", import.meta.url), "utf8"), {compilerOptions: {module: ts.ModuleKind.ESNext}}).outputText;
const { liveDataToRecords } = await import(`data:text/javascript;base64,${Buffer.from(recordHelper).toString("base64")}`);

test("disk IO retains true zero, missing values and safe states", () => {
  assert.equal(formatDiskIORate(0), "0 B/s");
  assert.equal(formatDiskIORate(1048576), "1 MiB/s");
  assert.equal(formatDiskIORate(524288), "512 KiB/s");
  const idle = { status: "ok", read_bytes_per_sec: 0, write_bytes_per_sec: 0, sample_interval_ms: 2000 };
  assert.deepEqual(parseDiskIO(idle), idle);
  for (const invalid of [undefined, {}, {...idle, read_bytes_per_sec: -1}, {...idle, write_bytes_per_sec: NaN}, {...idle, sample_interval_ms: 0}, {...idle, read_bytes_per_sec: null}]) assert.equal(parseDiskIO(invalid), undefined);
  for (const status of ["warming_up", "unsupported", "unavailable", "disabled"]) {
    assert.deepEqual(parseDiskIO({status, sample_interval_ms: 0}), {status, sample_interval_ms: 0});
    assert.equal(parseDiskIO({status, sample_interval_ms: 0, read_bytes_per_sec: 0}), undefined);
  }
  assert.equal(diskIOStatusKey(idle, false), "diskIO.offline");
  assert.equal(diskIOStatusKey(undefined, true), "diskIO.unsupported");
});

test("live chart adaptation never fills unavailable disk IO with zero", () => {
  const base = { cpu: {}, ram: {}, swap: {}, load: {}, disk: {}, connections: {} };
  const rows = liveDataToRecords("node", [base, {...base, disk_io: {status: "unavailable"}}, {...base, disk_io: {status: "ok", read_bytes_per_sec: 0, write_bytes_per_sec: 512}}]);
  assert.equal(rows[0].disk_read_rate, null);
  assert.equal(rows[1].disk_write_rate, null);
  assert.equal(rows[2].disk_read_rate, 0);
  assert.equal(rows[2].disk_write_rate, 512);
});

test("reconnected probe leaves an IO chart gap without fabricated zero", () => {
  const rows = [{time:"2026-10-01T00:00:00Z", read:0}, {time:"2026-10-01T00:01:00Z", read:100}];
  const result = breakDiskIOGaps(rows, ["read"], 2000);
  assert.equal(result.length, 3);
  assert.equal(result[1].read, null);
  assert.equal(result[1].time, "2026-10-01T00:00:02.000Z");
  assert.equal(result[0].read, 0);
});
