import type { DiskIO } from "../types/LiveData";

export interface NodeInfo {
  name: string; region: string; cpu_name: string; cpu_cores: number;
  arch: string; virtualization: string; os: string; kernel_version: string;
  gpu_name: string; mem_total: number; swap_total: number; disk_total: number;
}
export interface Report {
  cpu: { usage: number }; ram: { used: number; total: number };
  swap: { used: number; total: number }; disk: { used: number; total: number };
  disk_io?: DiskIO;
  network: { up: number; down: number; totalUp: number; totalDown: number };
  connections: { tcp: number; udp: number }; process: number; uptime: number;
  load: { load1: number; load5: number; load15: number };
  gpu?: { average_usage: number; detailed_info: { name: string; utilization: number; memory_used: number; memory_total: number; temperature: number }[] };
  updated_at: string;
}
export interface Bootstrap {
  node: NodeInfo; share_expires_at: string | null; session_expires_at: string;
  definitions: { name: string; unit: string }[]; ping_tasks: { id: string; name: string }[];
}
export interface Status { online: boolean; latest: Report | null; recent: Report[]; share_expires_at: string | null; session_expires_at: string }
export interface Series { metric_key: string; id: string; name: string; unit: string; points: { time: string; value: number | null }[] }

export class ShareError extends Error {
  expired: boolean;
  constructor(expired: boolean) { super("Share unavailable"); this.expired = expired; }
}
// No main-site client, storage, query UUID, analytics or WebSocket connection.
export async function shareRPC<T>(method: string, params: object = {}, signal?: AbortSignal): Promise<T> {
  const response = await fetch("/api/share/rpc", {
    method: "POST", credentials: "same-origin", cache: "no-store",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ jsonrpc: "2.0", id: 1, method, params }),
    signal: signal ?? AbortSignal.timeout(12000),
  });
  if (!response.ok) throw new ShareError(response.status === 404 || response.status === 410);
  const data = await response.json();
  if (data.error) throw new ShareError(false);
  return data.result as T;
}
