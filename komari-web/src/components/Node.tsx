import React from "react";
import { Card, IconButton } from "@radix-ui/themes";
import { Cpu, MemoryStick, HardDrive, Gauge, ArrowUp, ArrowDown, Globe, Clock, CalendarDays, TrendingUp } from "lucide-react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import type { LiveData, Record } from "../types/LiveData";
import type { NodeBasicInfo } from "@/contexts/NodeListContext";
import { usePublicInfo } from "@/contexts/PublicInfoContext";
import { diskIOStatusKey, formatDiskIORate } from "@/utils/diskIO";
import { formatBytes } from "@/utils/unitHelper";
import { getOSImage, getOSName } from "@/utils";
import Flag from "./Flag";
import PriceTags from "./PriceTags";
import MiniPingChartFloat from "./MiniPingChartFloat";
import Tips from "./ui/tips";

export function formatUptime(seconds: number, t: TFunction): string {
  if (!seconds || seconds < 0) return `0 ${t("nodeCard.time_second", { val: 0 })}`;
  const parts: string[] = [];
  const units = [[86400, 'day'], [3600, 'hour'], [60, 'minute'], [1, 'second']] as const;
  let remaining = seconds;
  for (const [size, name] of units) {
    const val = Math.floor(remaining / size);
    if (val) parts.push(`${val} ${t(`nodeCard.time_${name}`, { val })}`);
    remaining %= size;
  }
  return parts.join(' ');
}
function Segments({ value, tone }: { value?: number; tone: string }) {
  const clamped = value === undefined ? 0 : Math.min(100, Math.max(0, value));
  return <div className="liquid-segments" data-tone={tone} aria-hidden="true">{Array.from({ length: 20 }, (_, i) => <i key={i} data-filled={value !== undefined && i < Math.ceil(clamped / 5)} />)}</div>;
}
function Metric({ icon, label, value, detail, percent, tone }: { icon: React.ReactNode; label: string; value: string; detail: string; percent?: number; tone: string }) {
  return <div className="liquid-metric"><div className="liquid-metric-heading"><span>{icon}{label}</span><strong>{value}</strong></div><small>{detail}</small><Segments value={percent} tone={tone} /></div>;
}
const Node = React.memo(({ basic, live, online, showIpTagsInCard }: { basic: NodeBasicInfo; live?: Record; online: boolean; showIpTagsInCard: boolean }) => {
  const { t, i18n } = useTranslation();
  const available = online && !!live;
  const memory = available && basic.mem_total > 0 ? live.ram.used / basic.mem_total * 100 : undefined;
  const disk = available && basic.disk_total > 0 ? live.disk.used / basic.disk_total * 100 : undefined;
  const cpu = available ? live.cpu.usage : undefined;
  const load = available ? live.load.load1 : undefined;
  const percent = (v?: number) => v === undefined ? '—' : `${v.toFixed(1)}%`;
  const up = live?.network.totalUp;
  const down = live?.network.totalDown;
  const mode = basic.traffic_limit_type ?? 'sum';
  const used = up === undefined || down === undefined ? undefined : ({ sum: up + down, max: Math.max(up, down), min: Math.min(up, down), up, down })[mode];
  const limit = basic.traffic_limit;
  const expires = basic.expired_at ? new Date(basic.expired_at) : null;
  const io = live?.disk_io;
  return <Card className="km-node-card liquid-node" data-online={online} id={basic.uuid}>
    <div className="liquid-node-heading">
      <Flag flag={basic.region} /><Link className="km-node-name" to={`/instance/${basic.uuid}`}>{basic.name}</Link>
      {live?.message && <Tips color="#CE282E">{live.message}</Tips>}
      <MiniPingChartFloat uuid={basic.uuid} hours={24} trigger={<IconButton variant="ghost" size="1" aria-label={t('nodeCard.chart')}><TrendingUp size={16}/></IconButton>}/>
      <img src={getOSImage(basic.os)} alt={getOSName(basic.os)} title={`${basic.os} / ${basic.arch}`} width={18} height={18}/>
    </div>
    <div className="liquid-node-badges"><span className="km-node-status">{t(online ? 'nodeCard.online' : 'nodeCard.offline')}</span>{showIpTagsInCard && basic.ipv4 && <span>V4</span>}{showIpTagsInCard && basic.ipv6 && <span>V6</span>}<span className="liquid-node-os">{getOSName(basic.os)} · {basic.arch}</span></div>
    <div className="liquid-node-metrics">
      <Metric icon={<Cpu size={13}/>} label="CPU" value={percent(cpu)} detail={t('liquid.cores', { count: basic.cpu_cores })} percent={cpu} tone="cpu"/>
      <Metric icon={<MemoryStick size={13}/>} label={t('nodeCard.ram')} value={percent(memory)} detail={`${available ? formatBytes(live.ram.used) : '—'} / ${formatBytes(basic.mem_total)}`} percent={memory} tone="memory"/>
      <Metric icon={<HardDrive size={13}/>} label={t('nodeCard.disk')} value={percent(disk)} detail={`${available ? formatBytes(live.disk.used) : '—'} / ${formatBytes(basic.disk_total)}`} percent={disk} tone="disk"/>
      <Metric icon={<Gauge size={13}/>} label={t('liquid.load')} value={load === undefined ? '—' : load.toFixed(2)} detail={t('liquid.loadNormalized')} percent={load !== undefined && basic.cpu_cores > 0 ? load / basic.cpu_cores * 100 : undefined} tone="load"/>
    </div>
    <div className="liquid-node-network">
      {(['up', 'down'] as const).map(direction => <div key={direction} className="liquid-network-cell"><div><span>{direction === 'up' ? <ArrowUp size={14}/> : <ArrowDown size={14}/>} {t(`liquid.${direction}`)}</span><strong>{available ? formatBytes(live.network[direction]) : '—'}<small>{available ? '/s' : ''}</small></strong></div><div className="liquid-network-total"><span><Globe size={13}/>{t(`liquid.${direction}Total`)}</span><span>{live ? formatBytes(direction === 'up' ? live.network.totalUp : live.network.totalDown) : '—'}</span></div></div>)}
    </div>
    <div className="liquid-node-quota"><div><span><HardDrive size={13}/>{t('liquid.remaining')}</span><strong>{used === undefined ? '—' : limit > 0 ? formatBytes(Math.max(0, limit - used)) : '∞'}</strong><small>{used === undefined ? '—' : formatBytes(used)} / {limit > 0 ? formatBytes(limit) : '∞'} · {mode}</small></div><Segments value={limit > 0 && used !== undefined ? used / limit * 100 : undefined} tone="quota"/></div>
    <div className="liquid-node-io"><span>{t('diskIO.title')}</span><strong>{available && io?.status === 'ok' ? `${t('diskIO.read')} ${formatDiskIORate(io.read_bytes_per_sec ?? 0)} · ${t('diskIO.write')} ${formatDiskIORate(io.write_bytes_per_sec ?? 0)}` : t(diskIOStatusKey(io, online))}</strong></div>
    <div className="liquid-node-footer"><div><span><Clock size={13}/>{t('nodeCard.uptime')}</span><strong>{available ? formatUptime(live.uptime, t) : '—'}</strong></div><div><span><CalendarDays size={13}/>{t('liquid.expires')}</span><strong>{expires && Number.isFinite(expires.getTime()) ? expires.toLocaleDateString(i18n.language) : '—'}</strong></div></div>
    <PriceTags price={basic.price} billing_cycle={basic.billing_cycle} expired_at={basic.expired_at} currency={basic.currency} tags={basic.tags || ''}/>
  </Card>;
});
export default Node;
export const NodeGrid = ({ nodes, liveData, onlineSet }: { nodes: NodeBasicInfo[]; liveData: LiveData; onlineSet: ReadonlySet<string> }) => {
  const { publicInfo } = usePublicInfo();
  const position = publicInfo?.theme_settings?.offlineServerPosition;
  const sorted = React.useMemo(() => [...nodes].sort((a,b) => {
    if (position !== 'Keep' && onlineSet.has(a.uuid) !== onlineSet.has(b.uuid)) return (onlineSet.has(a.uuid) ? 1 : -1) * (position === 'First' ? 1 : -1);
    return a.weight - b.weight;
  }), [nodes, onlineSet, position]);
  return <div className="km-node-list liquid-node-grid">{sorted.map(node => <Node key={node.uuid} basic={node} live={liveData.data[node.uuid]} online={onlineSet.has(node.uuid)} showIpTagsInCard={Boolean(publicInfo?.theme_settings?.showIpTagsInCard)} />)}</div>;
};
