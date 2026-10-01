import { createContext, useContext, useEffect, useState } from "react";
import type { ReactNode } from "react";
import { shareRPC, ShareError } from "./api";
import type { Bootstrap, Status, Series } from "./api";

type Data = { bootstrap: Bootstrap | null; status: Status | null; series: Series[]; error: string; hours: number; setHours: (hours: number) => void };
const Context = createContext<Data | null>(null);
const keys = ["cpu.usage", "memory.used", "swap.used", "disk.used", "load.average", "disk.io.read.rate", "disk.io.write.rate", "net.in.rate", "net.out.rate", "gpu.usage", "gpu.device.usage", "gpu.memory.used", "gpu.temperature", "ping.latency_ms", "ping.loss", "process.count", "connections.tcp", "connections.udp"];

export function ShareDataProvider({ children }: { children: ReactNode }) {
  const [bootstrap, setBootstrap] = useState<Bootstrap | null>(null);
  const [status, setStatus] = useState<Status | null>(null);
  const [series, setSeries] = useState<Series[]>([]);
  const [error, setError] = useState("");
  const [hours, setHours] = useState(1);
  useEffect(() => {
    let stopped = false, running = false, lastHistory = 0, generation = 0;
    let expires = 0;
    const controller = new AbortController();
    const clear = (message: string) => { setBootstrap(null); setStatus(null); setSeries([]); setError(message); };
    const refresh = async () => {
      if (stopped || running || document.hidden) return;
      running = true;
      const currentGeneration = generation;
      try {
        if (!navigator.onLine) throw new ShareError(false);
        const timeout = AbortSignal.any([controller.signal, AbortSignal.timeout(12000)]);
        const node = await shareRPC<Bootstrap>("share:getNode", {}, timeout);
        const latest = await shareRPC<Status>("share:getLatestStatus", {}, timeout);
        const deadline = Math.min(new Date(node.session_expires_at).getTime(), node.share_expires_at ? new Date(node.share_expires_at).getTime() : Infinity);
        if (Date.now() >= deadline) throw new ShareError(true);
        let history: Series[] | undefined;
        if (Date.now() - lastHistory >= 30000) {
          history = (await shareRPC<{ series: Series[] }>("share:queryMetrics", { metric_keys: keys, hours, max_points: 500 }, timeout)).series;
          lastHistory = Date.now();
        }
        if (stopped || document.hidden || !navigator.onLine || currentGeneration !== generation) { lastHistory = 0; return; }
        expires = deadline;
        setBootstrap(node); setStatus(latest); if (history) setSeries(history); setError("");
      } catch (e) {
        if (!stopped && currentGeneration === generation) {
          const expired = e instanceof ShareError && e.expired;
          clear(expired ? "unavailable" : "network_error");
          lastHistory = 0;
          if (expired) stopped = true;
        }
      } finally { running = false; }
    };
    const tick = () => {
      if (expires && Date.now() >= expires) { stopped = true; clear("unavailable"); }
    };
    const visibility = () => {
      generation++; clear(""); lastHistory = 0; tick(); if (!document.hidden) void refresh();
    };
    const offline = () => { generation++; lastHistory = 0; clear("network_error"); };
    void refresh();
    const poll = window.setInterval(() => void refresh(), 5000);
    const expiryTimer = window.setInterval(tick, 250);
    document.addEventListener("visibilitychange", visibility);
    window.addEventListener("pageshow", visibility);
    window.addEventListener("offline", offline);
    window.addEventListener("online", visibility);
    return () => {
      stopped = true; controller.abort(); window.clearInterval(poll); window.clearInterval(expiryTimer);
      document.removeEventListener("visibilitychange", visibility); window.removeEventListener("pageshow", visibility);
      window.removeEventListener("offline", offline); window.removeEventListener("online", visibility);
    };
  }, [hours]);
  return <Context.Provider value={{ bootstrap, status, series, error, hours, setHours }}>{children}</Context.Provider>;
}

// eslint-disable-next-line react-refresh/only-export-components
export function useShareData() { const data = useContext(Context); if (!data) throw new Error("Missing share data provider"); return data; }
