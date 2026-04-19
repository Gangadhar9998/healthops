import { AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid } from 'recharts'
import { format } from 'date-fns'
import { CHART_COLORS } from '@/lib/constants'
import type { ResponseTimeBucket } from '@/types'

interface Props {
  data: ResponseTimeBucket[]
  height?: number
  showPercentiles?: boolean
}

export function ResponseTimeChart({ data, height = 260, showPercentiles = false }: Props) {
  const formatted = data.map(d => ({
    ...d,
    time: format(new Date(d.timestamp), 'HH:mm'),
    date: format(new Date(d.timestamp), 'MMM d, HH:mm'),
  }))

  return (
    <ResponsiveContainer width="100%" height={height}>
      <AreaChart data={formatted} margin={{ top: 4, right: 4, bottom: 0, left: -12 }}>
        <defs>
          <linearGradient id="avgFill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={CHART_COLORS.primary} stopOpacity={0.15} />
            <stop offset="100%" stopColor={CHART_COLORS.primary} stopOpacity={0} />
          </linearGradient>
          <linearGradient id="p95Fill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={CHART_COLORS.p95} stopOpacity={0.1} />
            <stop offset="100%" stopColor={CHART_COLORS.p95} stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke="var(--chart-grid, #e2e8f0)" vertical={false} />
        <XAxis
          dataKey="time"
          tick={{ fontSize: 11, fill: 'var(--chart-tick, #94a3b8)' }}
          axisLine={false}
          tickLine={false}
        />
        <YAxis
          tick={{ fontSize: 11, fill: 'var(--chart-tick, #94a3b8)' }}
          axisLine={false}
          tickLine={false}
          tickFormatter={(v: number) => `${v}ms`}
        />
        <Tooltip
          contentStyle={{
            backgroundColor: 'var(--tooltip-bg, #fff)',
            border: '1px solid var(--tooltip-border, #e2e8f0)',
            borderRadius: '8px',
            fontSize: '12px',
            boxShadow: '0 4px 12px rgba(0,0,0,0.08)',
          }}
          labelFormatter={(_, payload) => payload[0]?.payload?.date ?? ''}
          formatter={(value: number, name: string) => [`${value.toFixed(1)}ms`, name]}
        />
        {showPercentiles && (
          <Area
            type="monotone"
            dataKey="p95DurationMs"
            name="p95"
            stroke={CHART_COLORS.p95}
            fill="url(#p95Fill)"
            strokeWidth={1.5}
            dot={false}
          />
        )}
        <Area
          type="monotone"
          dataKey="avgDurationMs"
          name="Avg"
          stroke={CHART_COLORS.primary}
          fill="url(#avgFill)"
          strokeWidth={2}
          dot={false}
          activeDot={{ r: 4, strokeWidth: 0 }}
        />
      </AreaChart>
    </ResponsiveContainer>
  )
}
