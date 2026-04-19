import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { TrendingUp, AlertTriangle } from 'lucide-react'
import { analyticsApi } from '@/api/analytics'
import { MetricCard } from '@/components/MetricCard'
import { LoadingState } from '@/components/LoadingState'
import { ExportButton } from '@/components/ExportButton'
import { ResponseTimeChart } from '@/components/charts/ResponseTimeChart'
import { UptimeChart } from '@/components/charts/UptimeChart'
import { settingsApi } from '@/api/settings'
import { cn } from '@/lib/utils'
import { CHART_COLORS } from '@/lib/constants'
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,

} from 'recharts'

type Period = '24h' | '7d' | '30d'

export default function Analytics() {
  const [period, setPeriod] = useState<Period>('7d')

  const { data: uptime, isLoading: uptimeLoading } = useQuery({
    queryKey: ['analytics', 'uptime', period],
    queryFn: () => analyticsApi.uptime({ period }),
  })

  const { data: responseTimes, isLoading: rtLoading } = useQuery({
    queryKey: ['analytics', 'response-times', period],
    queryFn: () => analyticsApi.responseTimes({ period, interval: period === '24h' ? '1h' : '1d' }),
  })

  const { data: failureRate } = useQuery({
    queryKey: ['analytics', 'failure-rate', period],
    queryFn: () => analyticsApi.failureRate({ period, interval: period === '24h' ? '1h' : '1d' }),
  })

  const { data: statusTimeline } = useQuery({
    queryKey: ['analytics', 'status-timeline', period],
    queryFn: () => analyticsApi.statusTimeline({ period }),
  })

  const { data: incidentStats } = useQuery({
    queryKey: ['analytics', 'incidents'],
    queryFn: analyticsApi.incidents,
  })

  const isLoading = uptimeLoading || rtLoading

  if (isLoading) return <LoadingState />

  const avgUptime = uptime && uptime.length > 0
    ? uptime.reduce((sum, u) => sum + u.uptimePct, 0) / uptime.length
    : 0

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">Analytics</h1>
          <p className="text-sm text-slate-500">Performance and reliability metrics</p>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex overflow-hidden rounded-lg border border-slate-200 dark:border-slate-700">
            {(['24h', '7d', '30d'] as Period[]).map(p => (
              <button
                key={p}
                onClick={() => setPeriod(p)}
                className={cn(
                  'px-3 py-1.5 text-xs font-medium transition-colors',
                  period === p
                    ? 'bg-slate-900 text-white dark:bg-slate-100 dark:text-slate-900'
                    : 'text-slate-600 hover:bg-slate-50 dark:text-slate-400 dark:hover:bg-slate-800',
                )}
              >
                {p}
              </button>
            ))}
          </div>
          <ExportButton downloadUrl={settingsApi.exportResults('csv')} />
        </div>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <MetricCard label="Avg Uptime" value={`${avgUptime.toFixed(2)}%`} icon={<TrendingUp className="h-5 w-5" />} />
        {incidentStats && <>
          <MetricCard label="Total Incidents" value={incidentStats.total} icon={<AlertTriangle className="h-5 w-5" />} />
          <MetricCard
            label="MTTA"
            value={incidentStats.mttaMinutes > 0 ? `${incidentStats.mttaMinutes.toFixed(0)}m` : '—'}
            subValue="Mean time to acknowledge"
          />
          <MetricCard
            label="MTTR"
            value={incidentStats.mttrMinutes > 0 ? `${incidentStats.mttrMinutes.toFixed(0)}m` : '—'}
            subValue="Mean time to resolve"
          />
        </>}
      </div>

      {/* Uptime by check */}
      {uptime && uptime.length > 0 && (
        <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
          <h2 className="mb-4 text-sm font-semibold text-slate-900 dark:text-slate-100">Uptime by Check ({period})</h2>
          <UptimeChart data={uptime} />
        </div>
      )}

      {/* Response times */}
      {responseTimes && responseTimes.length > 0 && (
        <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
          <h2 className="mb-4 text-sm font-semibold text-slate-900 dark:text-slate-100">Response Times ({period})</h2>
          <ResponseTimeChart data={responseTimes} showPercentiles />
        </div>
      )}

      {/* Failure rate */}
      {failureRate && failureRate.length > 0 && (
        <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
          <h2 className="mb-4 text-sm font-semibold text-slate-900 dark:text-slate-100">Failure Rate ({period})</h2>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={failureRate}>
                <defs>
                  <linearGradient id="failGrad" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor={CHART_COLORS.critical} stopOpacity={0.2} />
                    <stop offset="95%" stopColor={CHART_COLORS.critical} stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--chart-grid, #e2e8f0)" />
                <XAxis
                  dataKey="timestamp"
                  tickFormatter={(v) => new Date(v).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}
                  tick={{ fontSize: 11, fill: '#94a3b8' }}
                />
                <YAxis tickFormatter={(v) => `${v}%`} tick={{ fontSize: 11, fill: '#94a3b8' }} />
                <Tooltip
                  formatter={(v: number) => [`${v.toFixed(2)}%`, 'Failure Rate']}
                  contentStyle={{ borderRadius: '8px', border: '1px solid #e2e8f0', fontSize: 12 }}
                />
                <Area type="monotone" dataKey="rate" stroke={CHART_COLORS.critical} fill="url(#failGrad)" strokeWidth={2} />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </div>
      )}

      {/* Status timeline */}
      {statusTimeline && statusTimeline.length > 0 && (
        <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
          <h2 className="mb-4 text-sm font-semibold text-slate-900 dark:text-slate-100">Status Timeline ({period})</h2>
          <div className="flex flex-wrap gap-1">
            {statusTimeline.map((entry, i) => (
              <div
                key={i}
                className={cn(
                  'h-6 w-6 rounded-sm',
                  entry.status === 'healthy' && 'bg-emerald-500',
                  entry.status === 'warning' && 'bg-amber-500',
                  entry.status === 'critical' && 'bg-red-500',
                  entry.status === 'unknown' && 'bg-slate-300 dark:bg-slate-600',
                )}
                title={`${new Date(entry.timestamp).toLocaleString()}: ${entry.status}`}
              />
            ))}
          </div>
          <div className="mt-3 flex gap-4 text-xs text-slate-500">
            <span className="flex items-center gap-1"><span className="inline-block h-3 w-3 rounded-sm bg-emerald-500" /> Healthy</span>
            <span className="flex items-center gap-1"><span className="inline-block h-3 w-3 rounded-sm bg-amber-500" /> Warning</span>
            <span className="flex items-center gap-1"><span className="inline-block h-3 w-3 rounded-sm bg-red-500" /> Critical</span>
          </div>
        </div>
      )}
    </div>
  )
}
