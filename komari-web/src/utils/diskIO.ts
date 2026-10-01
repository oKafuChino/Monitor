import type { DiskIO } from "../types/LiveData";

export function parseDiskIO(value: unknown): DiskIO | undefined {
  if (!value || typeof value !== "object") return undefined;
  const io = value as DiskIO;
  if (io.status === "ok") {
    const valid = (rate: unknown) => typeof rate === "number" && Number.isFinite(rate) && rate >= 0;
    if (!valid(io.read_bytes_per_sec) || !valid(io.write_bytes_per_sec) ||
        !Number.isSafeInteger(io.sample_interval_ms) || io.sample_interval_ms <= 0) return undefined;
    return { status: "ok", read_bytes_per_sec: io.read_bytes_per_sec,
      write_bytes_per_sec: io.write_bytes_per_sec, sample_interval_ms: io.sample_interval_ms };
  }
  if (!["warming_up", "unsupported", "unavailable", "disabled"].includes(io.status) ||
      io.read_bytes_per_sec != null || io.write_bytes_per_sec != null || io.sample_interval_ms !== 0) return undefined;
  return { status: io.status, sample_interval_ms: 0 };
}

export const diskIOStatusKey = (io: DiskIO | undefined, online: boolean) =>
  !online ? "diskIO.offline" : `diskIO.${io?.status ?? "unsupported"}`;

export function formatDiskIORate(rate: number): string {
  if (!Number.isFinite(rate) || rate < 0) return "-";
  const units = ["B/s", "KiB/s", "MiB/s", "GiB/s", "TiB/s"];
  let value = rate;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) { value /= 1024; index++; }
  return `${index === 0 ? Math.round(value) : Number(value.toFixed(2))} ${units[index]}`;
}

// The probe keeps sampling through network outages. Insert a null boundary when
// reports resume so a chart never draws a continuous IO line across that outage.
export function breakDiskIOGaps(
  rows: Array<Record<string, string | number | null>>,
  keys: string[],
  intervalMs: number,
) {
  if (keys.length === 0 || intervalMs <= 0) return rows;
  const output: typeof rows = [];
  for (const row of rows) {
    const previous = output.at(-1);
    const before = previous ? new Date(String(previous.time)).getTime() : NaN;
    const now = new Date(String(row.time)).getTime();
    if (Number.isFinite(before) && now - before > intervalMs * 5) {
      output.push({time: new Date(before + intervalMs).toISOString(), ...Object.fromEntries(keys.map((key) => [key, null]))});
    }
    output.push(row);
  }
  return output;
}
