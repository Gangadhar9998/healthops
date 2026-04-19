import { PieChart, Pie, Cell, ResponsiveContainer, Tooltip } from 'recharts'
import { CHART_COLORS } from '@/lib/constants'

interface Props {
  healthy: number
  warning: number
  critical: number
  unknown: number
  size?: number
}

export function StatusDistribution({ healthy, warning, critical, unknown, size = 160 }: Props) {
  const data = [
    { name: 'Healthy', value: healthy, color: CHART_COLORS.healthy },
    { name: 'Warning', value: warning, color: CHART_COLORS.warning },
    { name: 'Critical', value: critical, color: CHART_COLORS.critical },
    { name: 'Unknown', value: unknown, color: CHART_COLORS.unknown },
  ].filter(d => d.value > 0)

  const total = healthy + warning + critical + unknown

  if (total === 0) {
    return (
      <div className="flex items-center justify-center" style={{ width: size, height: size }}>
        <p className="text-sm text-slate-400">No data</p>
      </div>
    )
  }

  return (
    <div className="relative" style={{ width: size, height: size }}>
      <ResponsiveContainer>
        <PieChart>
          <Pie
            data={data}
            cx="50%"
            cy="50%"
            innerRadius={size * 0.32}
            outerRadius={size * 0.45}
            paddingAngle={2}
            dataKey="value"
            stroke="none"
          >
            {data.map((entry, i) => (
              <Cell key={i} fill={entry.color} />
            ))}
          </Pie>
          <Tooltip
            contentStyle={{
              backgroundColor: 'var(--tooltip-bg, #fff)',
              border: '1px solid var(--tooltip-border, #e2e8f0)',
              borderRadius: '8px',
              fontSize: '12px',
            }}
          />
        </PieChart>
      </ResponsiveContainer>
      <div className="absolute inset-0 flex flex-col items-center justify-center">
        <span className="text-2xl font-bold text-slate-900 dark:text-slate-100">{total}</span>
        <span className="text-[10px] font-medium text-slate-400">CHECKS</span>
      </div>
    </div>
  )
}
