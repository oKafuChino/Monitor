import { useTranslation } from "react-i18next";
import { NodeMetricChart } from "../components/NodeMetricChart";
import { useShareData } from "./ShareDataProvider";
import { formatBytes } from "../utils/unitHelper";
import { diskIOStatusKey, formatDiskIORate } from "../utils/diskIO";

const groups = [
  ["cpu", ["cpu.usage"]], ["memory", ["memory.used", "swap.used"]],
  ["disk", ["disk.used"]], ["io", ["disk.io.read.rate", "disk.io.write.rate"]],
  ["network", ["net.in.rate", "net.out.rate"]], ["load", ["load.average"]],
  ["gpu", ["gpu.usage", "gpu.device.usage"]], ["gpu_memory", ["gpu.memory.used"]],
  ["gpu_temperature", ["gpu.temperature"]], ["ping", ["ping.latency_ms"]],
  ["loss", ["ping.loss"]], ["connections", ["connections.tcp", "connections.udp", "process.count"]],
] as const;
export function SharePage() {
  const { t, i18n } = useTranslation();
  const { bootstrap, status, series, error, hours, setHours } = useShareData();
  const node = bootstrap?.node, latest = status?.latest;
  const online = status?.online === true;
  const io = latest?.disk_io;
  const ioKey = diskIOStatusKey(io, online);
  const ioText = online && io?.status === "ok"
    ? `${t("share.read")} ${formatDiskIORate(io.read_bytes_per_sec ?? 0)} · ${t("share.write")} ${formatDiskIORate(io.write_bytes_per_sec ?? 0)}`
    : t(ioKey === "diskIO.offline" ? "share.offline" : io?.status === "unavailable" ? "share.io_unavailable" : `share.${io?.status ?? "unsupported"}`);
  const percent = (used?: number, total?: number) => used !== undefined && total ? `${(used / total * 100).toFixed(1)}%` : "—";
  const cells = latest ? [
    ["CPU", online ? `${(latest.cpu.usage ?? 0).toFixed(1)}%` : "—"],
    [t("share.memory"), online ? `${percent(latest.ram.used, latest.ram.total)} · ${formatBytes(latest.ram.used)} / ${formatBytes(latest.ram.total)}` : "—"],
    [t("share.disk"), online ? `${percent(latest.disk.used, latest.disk.total)} · ${formatBytes(latest.disk.used)} / ${formatBytes(latest.disk.total)}` : "—"],
    [t("share.io"), ioText],
    [t("share.network"), online ? `↑ ${formatBytes(latest.network.up)}/s · ↓ ${formatBytes(latest.network.down)}/s` : "—"],
    [t("share.traffic"), `↑ ${formatBytes(latest.network.totalUp)} · ↓ ${formatBytes(latest.network.totalDown)}`],
    [t("share.connections"), online ? `TCP ${latest.connections.tcp} · UDP ${latest.connections.udp}` : "—"],
    [t("share.uptime"), online ? `${Math.floor(latest.uptime / 86400)}d ${Math.floor(latest.uptime % 86400 / 3600)}h` : "—"],
  ] : [];
  return <main>
    <header><span className="eyebrow">{t("share.title")}</span><select aria-label={t("share.language")} value={i18n.resolvedLanguage} onChange={e => void i18n.changeLanguage(e.target.value)}>
      <option value="en">English</option><option value="zh-CN">简体中文</option><option value="zh-TW">繁體中文</option><option value="ja">日本語</option><option value="id">Bahasa Indonesia</option>
    </select></header>
    {error ? <section className="message" role="alert"><h1>{t(`share.${error}`)}</h1><p>{t("share.reopen")}</p></section> : !node ? <section className="message">{t("share.loading")}</section> : <>
      <div className="hero"><div><h1>{node.name}</h1><p>{node.region} · {node.os} · {node.arch}</p></div><span className={`presence ${online ? "online" : ""}`}>{t(online ? "share.online" : "share.offline")}</span></div>
      <section className="hardware"><p><strong>CPU</strong> {node.cpu_name} × {node.cpu_cores}</p><p><strong>GPU</strong> {node.gpu_name || "—"}</p><p><strong>{t("share.kernel")}</strong> {node.kernel_version || "—"}</p><p><strong>{t("share.virtualization")}</strong> {node.virtualization || "—"}</p><p><strong>{t("share.memory")}</strong> {formatBytes(node.mem_total)} · Swap {formatBytes(node.swap_total)}</p><p><strong>{t("share.disk")}</strong> {formatBytes(node.disk_total)}</p></section>
      <section className="stats">{cells.map(([label, value]) => <article key={label}><span>{label}</span><strong>{value}</strong></article>)}</section>
      {latest && <p className="muted">{t("share.updated")} {new Date(latest.updated_at).toLocaleString()}</p>}
      <div className="history-header"><h2>{t("share.history")}</h2><select aria-label={t("share.history")} value={hours} onChange={e => setHours(Number(e.target.value))}>{[1,6,24,168,720].map(h => <option key={h} value={h}>{h <= 24 ? `${h}h` : `${h / 24}d`}</option>)}</select></div>
      <section className="charts">{groups.map(([label, keys]) => {
        const selected = series.filter(s => (keys as readonly string[]).includes(s.metric_key));
        return <article key={label}><h3>{t(`share.${label}`)}</h3><NodeMetricChart series={selected.map(s => ({ ...s, id: `${s.metric_key}:${s.id}`, name: s.id === "node" ? t(`share.metric.${s.metric_key.replace(/\./g, "_")}`, s.metric_key) : s.name }))} unit={selected[0]?.unit ?? ""} empty={t("share.empty")} /></article>;
      })}</section>
      <p className="muted">{t("share.expires")} {bootstrap.share_expires_at ? new Date(bootstrap.share_expires_at).toLocaleString() : t("share.forever")} · {t("share.retention")}</p>
    </>}
  </main>;
}
