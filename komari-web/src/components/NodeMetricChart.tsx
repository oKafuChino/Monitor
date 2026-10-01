import { ResponsiveContainer, LineChart, Line, CartesianGrid, XAxis, YAxis, Tooltip } from "recharts";

export interface MetricChartSeries { id: string; name: string; points: { time: string; value: number | null }[] }
// Pure detail chart: adapters provide all data, without site/account contexts.
export function NodeMetricChart({ series, unit, empty }: { series: MetricChartSeries[]; unit: string; empty: string }) {
  if (!series.some(s => s.points.some(p => p.value !== null))) return <div className="chart-empty">{empty}</div>;
  const rows = new Map<number, { time: number; [key: string]: number | null }>();
  series.forEach((s, index) => s.points.forEach(p => {
    const time = new Date(p.time).getTime();
    const row = rows.get(time) ?? { time };
    row[`s${index}`] = p.value; rows.set(time, row);
  }));
  const data = [...rows.values()].sort((a, b) => a.time - b.time);
  return <ResponsiveContainer width="100%" height={200}><LineChart data={data} margin={{ top: 12, right: 12, left: 0, bottom: 4 }}>
    <CartesianGrid strokeDasharray="3 3" opacity={0.16} />
    <XAxis dataKey="time" type="number" domain={["dataMin", "dataMax"]} tickFormatter={v => new Date(v).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })} minTickGap={40} />
    <YAxis width={60} tickFormatter={v => Intl.NumberFormat(undefined, { notation: "compact" }).format(v)} />
    <Tooltip labelFormatter={v => new Date(Number(v)).toLocaleString()} formatter={v => [typeof v === "number" ? `${Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(v)} ${unit}` : "—"]} />
    {series.map((s, i) => <Line key={s.id} name={s.name} dataKey={`s${i}`} stroke={["#8b80f9", "#2bbda8", "#f2aa54", "#e579a0"][i % 4]} dot={false} isAnimationActive={false} connectNulls={false} />)}
  </LineChart></ResponsiveContainer>;
}
