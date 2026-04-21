import { useQuery } from '@tanstack/react-query'
import { useState, useMemo } from 'react'
import { TrendingUp, AlertTriangle } from 'lucide-react'
import { format } from 'date-fns'
import { analyticsApi } from '@/api/analytics'
import { checksApi } from '@/api/checks'
import { MetricCard } from '@/components/MetricCard'
import { LoadingState } from '@/components/LoadingState'
import { ExportButton } from '@/components/ExportButton'
import { ResponseTimeChart } from '@/components/charts/ResponseTimeChart'
import { UptimeChart } from '@/components/charts/UptimeChart'
import { settingsApi } from '@/api/settings'
import { cn } from '@/lib/utils'
import { CHART_COLORS } from '@/lib/constants'
import {
  BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from 'recharts'

type Period = '24h' | '7d' | '30d'

export default function Analytics() {
  const [period, setPeriod] = useState<Period>('24h')

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

  const { data: checks } = useQuery({
    queryKey: ['checks'],
    queryFn: checksApi.list,
  })

  const activeCheckIds = useMemo(() => {
    if (!checks) return null
    return new Set(checks.map((c: { id: string }) => c.id))
  }, [checks])

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
              <BarChart data={failureRate}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--chart-grid, #e2e8f0)" />
                <XAxis
                  dataKey="group"
                  tick={{ fontSize: 11, fill: '#94a3b8' }}
                />
                <YAxis tickFormatter={(v) => `${v}%`} tick={{ fontSize: 11, fill: '#94a3b8' }} domain={[0, 100]} />
                <Tooltip
                  formatter={(v: number) => [`${v.toFixed(2)}%`, 'Failure Rate']}
                  contentStyle={{ borderRadius: '8px', border: '1px solid #e2e8f0', fontSize: 12 }}
                />
                <Bar dataKey="failureRate" fill={CHART_COLORS.critical} radius={[4, 4, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>
      )}

      {/* Status timeline — grouped by check, bucketed by time */}
      {statusTimeline && statusTimeline.length > 0 && (
        <StatusTimelineGrid entries={statusTimeline} period={period} activeCheckIds={activeCheckIds} />
      )}
    </div>
  )
}

/* ——— Status Timeline Grid: one row per check, time-bucketed cells ——— */

import type { StatusTimelineEntry } from '@/types'

function StatusTimelineGrid({ entries, period, activeCheckIds }: { entries: StatusTimelineEntry[]; period: Period; activeCheckIds: Set<string> | null }) {
  const grid = useMemo(() => {
    // Determine bucket size based on period
    const bucketMs = period === '24h' ? 60 * 60 * 1000 : // 1 hour buckets
                     period === '7d'  ? 6 * 60 * 60 * 1000 : // 6 hour buckets
                                        24 * 60 * 60 * 1000   // 1 day buckets

    // Calculate time range
    const now = Date.now()
    const periodMs = period === '24h' ? 24 * 60 * 60 * 1000 :
                     period === '7d'  ? 7 * 24 * 60 * 60 * 1000 :
                                        30 * 24 * 60 * 60 * 1000
    const start = now - periodMs
    const bucketCount = Math.ceil(periodMs / bucketMs)

    // Filter to only active checks
    const filtered = activeCheckIds
      ? entries.filter(e => activeCheckIds.has(e.checkId || ''))
      : entries

    // Group entries by check
    const byCheck = new Map<string, { name: string; entries: StatusTimelineEntry[] }>()
    for (const e of filtered) {
      const key = e.checkId || 'unknown'
      if (!byCheck.has(key)) byCheck.set(key, { name: e.checkName || key, entries: [] })
      byCheck.get(key)!.entries.push(e)
    }

    // For each check, assign entries into time buckets
    const rows: { checkId: string; name: string; buckets: (string | null)[] }[] = []
    for (const [checkId, { name, entries: checkEntries }] of byCheck) {
      const buckets: (string | null)[] = new Array(bucketCount).fill(null)
      for (const e of checkEntries) {
        const ts = new Date(e.timestamp).getTime()
        if (ts < start) continue
        const idx = Math.min(Math.floor((ts - start) / bucketMs), bucketCount - 1)
        // Worst status wins in each bucket
        const current = buckets[idx]
        if (!current || statusPriority(e.status) > statusPriority(current)) {
          buckets[idx] = e.status
        }
      }
      rows.push({ checkId, name, buckets })
    }

    // Sort rows by name
    rows.sort((a, b) => a.name.localeCompare(b.name))

    // Build time labels
    const labels: string[] = []
    const labelInterval = period === '24h' ? 4 : period === '7d' ? 4 : 5
    for (let i = 0; i < bucketCount; i++) {
      if (i % labelInterval === 0) {
        const t = new Date(start + i * bucketMs)
        labels.push(
          period === '24h' ? format(t, 'HH:mm') :
          period === '7d'  ? format(t, 'EEE HH:mm') :
                             format(t, 'MMM d')
        )
      } else {
        labels.push('')
      }
    }

    return { rows, labels, bucketCount }
  }, [entries, period, activeCheckIds])

  if (grid.rows.length === 0) return null

  return (
    <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
      <h2 className="mb-4 text-sm font-semibold text-slate-900 dark:text-slate-100">
        Status Timeline ({period === '24h' ? 'Today' : period})
      </h2>
      <div className="overflow-x-auto">
        <div className="min-w-[600px]">
          {/* Time labels row */}
          <div className="flex items-end mb-1 ml-[140px]">
            {grid.labels.map((label, i) => (
              <div key={i} className="text-[9px] text-slate-400 tabular-nums" style={{ width: `${100 / grid.bucketCount}%` }}>
                {label}
              </div>
            ))}
          </div>

          {/* Check rows */}
          <div className="space-y-0.5">
            {grid.rows.map(row => (
              <div key={row.checkId} className="flex items-center gap-2">
                <span className="w-[132px] shrink-0 truncate text-right text-[11px] text-slate-600 dark:text-slate-400" title={row.name}>
                  {row.name}
                </span>
                <div className="flex flex-1 gap-px">
                  {row.buckets.map((status, i) => (
                    <div
                      key={i}
                      className={cn(
                        'h-5 flex-1 rounded-[2px] transition-colors',
                        status === 'healthy' && 'bg-emerald-500',
                        status === 'warning' && 'bg-amber-400',
                        status === 'critical' && 'bg-red-500',
                        status === 'unknown' && 'bg-slate-300 dark:bg-slate-600',
                        status === null && 'bg-slate-100 dark:bg-slate-800',
                      )}
                      title={status ? `${row.name}: ${status}` : `${row.name}: no data`}
                    />
                  ))}
                </div>
              </div>
            ))}
          </div>

          {/* Legend */}
          <div className="mt-3 flex gap-4 text-xs text-slate-500 ml-[140px]">
            <span className="flex items-center gap-1"><span className="inline-block h-2.5 w-2.5 rounded-sm bg-emerald-500" /> Healthy</span>
            <span className="flex items-center gap-1"><span className="inline-block h-2.5 w-2.5 rounded-sm bg-amber-400" /> Warning</span>
            <span className="flex items-center gap-1"><span className="inline-block h-2.5 w-2.5 rounded-sm bg-red-500" /> Critical</span>
            <span className="flex items-center gap-1"><span className="inline-block h-2.5 w-2.5 rounded-sm bg-slate-100 dark:bg-slate-800 border border-slate-200 dark:border-slate-700" /> No data</span>
          </div>
        </div>
      </div>
    </div>
  )
}

function statusPriority(status: string): number {
  switch (status) {
    case 'critical': return 3
    case 'warning': return 2
    case 'unknown': return 1
    case 'healthy': return 0
    default: return -1
  }
}
