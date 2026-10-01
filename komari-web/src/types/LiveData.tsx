export type DiskIO = {
  status: "ok" | "warming_up" | "unsupported" | "unavailable" | "disabled";
  read_bytes_per_sec?: number | null;
  write_bytes_per_sec?: number | null;
  sample_interval_ms: number;
};

export type LiveData = {
    online: string[];
    data: { [key: string]: Record };
};

export type Record = {
  cpu: {
    usage: number;
  };
  ram: {
    used: number;
  };
  swap: {
    used: number;
  };
  load: {
    load1: number;
    load5: number;
    load15: number;
  };
  disk: {
    used: number;
  };
  disk_io?: DiskIO;
  network: {
    up: number;
    down: number;
    totalUp: number;
    totalDown: number;
  };
  connections: {
    tcp: number;
    udp: number;
  };
  gpu?: {
    count: number;
    average_usage: number;
    detailed_info: {
      name: string;
      memory_total: number;
      memory_used: number;
      utilization: number;
      temperature: number;
    }[];
  };
  uptime: number;
  process: number;
  message: string;
  updated_at: string;
};

export type LiveDataResponse = {
  data: LiveData;
  status: string;
};
