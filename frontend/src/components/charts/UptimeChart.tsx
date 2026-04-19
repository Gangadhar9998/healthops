import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid, Cell } from 'recharts'
import { CHART_COLORS } from '@/lib/constants'
import type { UptimeStats } from '@/types'

interface Props {
  data: UptimeStats[]
  height?: number
}

function barColor(pct: number): string {
  if (pct >= 99.9) return CHART_COLORS.healthy
  if (pct >= 99) return CHART_COLORS.warning
  return CHART_COLORS.critical
}

export function UptimeChart({ data, height = 260 }: Props) {
  const sorted = [...data].sort((a, b) => a.uptimePct - b.uptimePct)

  return (
    <ResponsiveContainer width="100%" height={height}>
      <BarChart data={sorted} layout="vertical" margin={{ top: 4, right: 16, bottom: 0, left: 0 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="var(--chart-grid, #e2e8f0)" horizontal={false} />
        <XAxis
          type="number"
          domain={[Math.min(95, Math.floor(Math.min(...data.map(d => d.uptimePct)))), 100]}
          tick={{ fontSize: 11, fill: 'var(--chart-tick, #94a3b8)' }}
          axisLine={false}
          tickLine={false}
          tickFormatter={(v: number) => `${v}%`}
        />
        <YAxis
          dataKey="checkName"
          type="category"
          tick={{ fontSize: 11, fill: 'var(--chart-tick, #94a3b8)' }}
          axisLine={false}
          tickLine={false}
          width={120}
        />
        <Tooltip
          contentStyle={{
            backgroundColor: 'var(--tooltip-bg, #fff)',
            border: '1px solid var(--tooltip-border, #e2e8f0)',
            borderRadius: '8px',
            fontSize: '12px',
          }}
          formatter={(value: number) => [`${value.toFixed(2)}%`, 'Uptime']}
        />
        <Bar dataKey="uptimePct" radius={[0, 4, 4, 0]} barSize={20}>
          {sorted.map((entry, i) => (
            <Cell key={i} fill={barColor(entry.uptimePct)} />
          ))}
        </Bar>
      </BarChart>
    </ResponsiveContainer>
  )
}
