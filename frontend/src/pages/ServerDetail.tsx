import { useQuery } from '@tanstack/react-query'
import { useParams, Link } from 'react-router-dom'
import { useState } from 'react'
import {
  ArrowLeft, Cpu, MemoryStick, HardDrive, Activity, Clock,
  RefreshCw, ChevronDown, ChevronUp,
} from 'lucide-react'
import {
  AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer,
  CartesianGrid, LineChart, Line,
} from 'recharts'
import { format } from 'date-fns'
import { serversApi } from '@/api/servers'
import { MetricCard } from '@/components/MetricCard'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { LiveIndicator } from '@/components/db/LiveIndicator'
import { Sparkline } from '@/components/charts/Sparkline'
import { cn } from '@/lib/utils'
import { REFETCH_INTERVAL, CHART_COLORS } from '@/lib/constants'
import { useServerLive } from '@/hooks/useServerLive'
import type { MetricsPoint } from '@/types'

type TimeRange = '1h' | '6h' | '12h' | '24h' | '7d'

const RANGES: { label: string; value: TimeRange }[] = [
  { label: '1H', value: '1h' },
  { label: '6H', value: '6h' },
  { label: '12H', value: '12h' },
  { label: '24H', value: '24h' },
  { label: '7D', value: '7d' },
]

type SortField = 'memPercent' | 'cpuPercent' | 'memMB'
type SortDir = 'asc' | 'desc'

export default function ServerDetail() {
  const { id } = useParams<{ id: string }>()
  const [range, setRange] = useState<TimeRange>('24h')
  const [sortField, setSortField] = useState<SortField>('memPercent')
  const [sortDir, setSortDir] = useState<SortDir>('desc')

  const { data: server, isLoading: serverLoading } = useQuery({
    queryKey: ['servers', id],
    queryFn: () => serversApi.get(id!),
    enabled: !!id,
  })

  const { data: snapshot, isLoading: metricsLoading, error: metricsError, refetch } = useQuery({
    queryKey: ['server-metrics', id],
    queryFn: () => serversApi.metrics(id!),
    enabled: !!id,
    refetchInterval: REFETCH_INTERVAL,
  })

  const { data: history } = useQuery({
    queryKey: ['server-metrics-history', id, range],
    queryFn: () => serversApi.metricsHistory(id!, range),
    enabled: !!id,
  })

  const { snapshot: liveSnap, history: liveHistory, connected: liveConnected } = useServerLive(id, !metricsLoading && !metricsError)

  if (serverLoading || metricsLoading) return <LoadingState />
  if (metricsError) {
    return (
      <div className="space-y-6 animate-fade-in">
        <BackLink />
        <ErrorState
          message={metricsError instanceof Error ? metricsError.message : 'Failed to load metrics'}
          retry={() => refetch()}
        />
        <p className="text-center text-sm text-slate-500">
          Metrics will appear once the SSH health check has run at least once.
        </p>
      </div>
    )
  }

  const toggleSort = (field: SortField) => {
    if (sortField === field) setSortDir(d => d === 'desc' ? 'asc' : 'desc')
    else { setSortField(field); setSortDir('desc') }
  }

  // Prefer live SSE snapshot over polled
  const s = liveSnap ?? snapshot
  const cpuHistory = liveHistory.map(h => h.cpuPercent)
  const memHistory = liveHistory.map(h => h.memoryPercent)
  const loadHistory = liveHistory.map(h => h.loadAvg1)

  const sortedProcesses = [...(s?.topProcesses || [])].sort((a, b) => {
    const mul = sortDir === 'desc' ? -1 : 1
    return mul * (a[sortField] - b[sortField])
  })

  const chartData = (history || []).map((p: MetricsPoint) => ({
    ...p,
    time: format(new Date(p.timestamp), 'HH:mm'),
    date: format(new Date(p.timestamp), 'MMM d, HH:mm'),
  }))

  const uptimeStr = s
    ? s.uptimeHours >= 24
      ? `${Math.floor(s.uptimeHours / 24)}d ${Math.floor(s.uptimeHours % 24)}h`
      : `${Math.floor(s.uptimeHours)}h ${Math.floor((s.uptimeHours % 1) * 60)}m`
    : '-'

  const gaugeColor = (pct: number) =>
    pct >= 90 ? 'text-red-500' : pct >= 75 ? 'text-amber-500' : 'text-emerald-500'

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-3">
          <BackLink />
          <div>
            <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">
              {server?.name || id}
            </h1>
            {server && (
              <p className="text-sm text-slate-500">
                {server.user}@{server.host}:{server.port}
              </p>
            )}
          </div>
        </div>
        <div className="flex items-center gap-2">
          <LiveIndicator connected={liveConnected} />
          <span className="text-xs text-slate-400">
            Last updated: {s ? format(new Date(s.timestamp), 'MMM d, HH:mm:ss') : '-'}
          </span>
          <button
            onClick={() => refetch()}
            className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-sm text-slate-600 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300"
          >
            <RefreshCw className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      {/* System metrics cards */}
      {s && (
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
          <div className="rounded-xl border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
            <div className="flex items-center gap-2 mb-1">
              <Cpu className={cn('h-4 w-4', gaugeColor(s.cpuPercent))} />
              <span className="text-xs font-medium text-slate-500">CPU</span>
            </div>
            <p className="text-2xl font-bold tabular-nums text-slate-900 dark:text-slate-100">{s.cpuPercent.toFixed(1)}%</p>
            {cpuHistory.length > 3 && <Sparkline data={cpuHistory} color="#3b82f6" height={28} />}
          </div>
          <div className="rounded-xl border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
            <div className="flex items-center gap-2 mb-1">
              <MemoryStick className={cn('h-4 w-4', gaugeColor(s.memoryPercent))} />
              <span className="text-xs font-medium text-slate-500">Memory</span>
            </div>
            <p className="text-2xl font-bold tabular-nums text-slate-900 dark:text-slate-100">{s.memoryPercent.toFixed(1)}%</p>
            <p className="text-[10px] text-slate-400 tabular-nums">{s.memoryUsedMB.toFixed(0)} / {s.memoryTotalMB.toFixed(0)} MB</p>
            {memHistory.length > 3 && <Sparkline data={memHistory} color="#8b5cf6" height={28} />}
          </div>
          <MetricCard
            label="Disk"
            value={`${s.diskPercent.toFixed(1)}%`}
            subValue={`${s.diskUsedGB.toFixed(1)} / ${s.diskTotalGB.toFixed(1)} GB`}
            icon={<HardDrive className={cn('h-5 w-5', gaugeColor(s.diskPercent))} />}
          />
          <div className="rounded-xl border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
            <div className="flex items-center gap-2 mb-1">
              <Activity className="h-4 w-4 text-blue-500" />
              <span className="text-xs font-medium text-slate-500">Load Avg</span>
            </div>
            <p className="text-2xl font-bold tabular-nums text-slate-900 dark:text-slate-100">{s.loadAvg1.toFixed(2)}</p>
            <p className="text-[10px] text-slate-400 tabular-nums">{s.loadAvg5.toFixed(2)} / {s.loadAvg15.toFixed(2)}</p>
            {loadHistory.length > 3 && <Sparkline data={loadHistory} color="#f59e0b" height={28} />}
          </div>
          <MetricCard
            label="Uptime"
            value={uptimeStr}
            icon={<Clock className="h-5 w-5 text-slate-500" />}
          />
        </div>
      )}

      {/* Charts section */}
      <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">
            Resource Usage Over Time
          </h2>
          <div className="flex rounded-lg border border-slate-200 dark:border-slate-700">
            {RANGES.map(r => (
              <button
                key={r.value}
                onClick={() => setRange(r.value)}
                className={cn(
                  'px-3 py-1 text-xs font-medium transition-colors',
                  range === r.value
                    ? 'bg-blue-600 text-white'
                    : 'text-slate-500 hover:bg-slate-50 dark:text-slate-400 dark:hover:bg-slate-800',
                  r.value === '1h' && 'rounded-l-lg',
                  r.value === '7d' && 'rounded-r-lg',
                )}
              >
                {r.label}
              </button>
            ))}
          </div>
        </div>

        {chartData.length > 0 ? (
          <div className="space-y-6">
            {/* CPU + Memory chart */}
            <div>
              <h3 className="mb-2 text-sm font-medium text-slate-500 dark:text-slate-400">CPU & Memory %</h3>
              <ResponsiveContainer width="100%" height={220}>
                <AreaChart data={chartData} margin={{ top: 4, right: 4, bottom: 0, left: -12 }}>
                  <defs>
                    <linearGradient id="cpuFill" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor={CHART_COLORS.primary} stopOpacity={0.15} />
                      <stop offset="100%" stopColor={CHART_COLORS.primary} stopOpacity={0} />
                    </linearGradient>
                    <linearGradient id="memFill" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor={CHART_COLORS.p95} stopOpacity={0.15} />
                      <stop offset="100%" stopColor={CHART_COLORS.p95} stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--chart-grid, #e2e8f0)" vertical={false} />
                  <XAxis dataKey="time" tick={{ fontSize: 11, fill: 'var(--chart-tick, #94a3b8)' }} axisLine={false} tickLine={false} />
                  <YAxis domain={[0, 100]} tick={{ fontSize: 11, fill: 'var(--chart-tick, #94a3b8)' }} axisLine={false} tickLine={false} tickFormatter={(v: number) => `${v}%`} />
                  <Tooltip
                    contentStyle={{ backgroundColor: 'var(--tooltip-bg, #fff)', border: '1px solid var(--tooltip-border, #e2e8f0)', borderRadius: '8px', fontSize: '12px', boxShadow: '0 4px 12px rgba(0,0,0,0.08)' }}
                    labelFormatter={(_, payload) => payload[0]?.payload?.date ?? ''}
                    formatter={(value: number, name: string) => [`${value.toFixed(1)}%`, name]}
                  />
                  <Area type="monotone" dataKey="cpuPercent" name="CPU" stroke={CHART_COLORS.primary} fill="url(#cpuFill)" strokeWidth={2} dot={false} />
                  <Area type="monotone" dataKey="memoryPercent" name="Memory" stroke={CHART_COLORS.p95} fill="url(#memFill)" strokeWidth={2} dot={false} />
                </AreaChart>
              </ResponsiveContainer>
            </div>

            {/* Disk + Load chart */}
            <div>
              <h3 className="mb-2 text-sm font-medium text-slate-500 dark:text-slate-400">Disk Usage & Load Average</h3>
              <ResponsiveContainer width="100%" height={180}>
                <LineChart data={chartData} margin={{ top: 4, right: 4, bottom: 0, left: -12 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--chart-grid, #e2e8f0)" vertical={false} />
                  <XAxis dataKey="time" tick={{ fontSize: 11, fill: 'var(--chart-tick, #94a3b8)' }} axisLine={false} tickLine={false} />
                  <YAxis yAxisId="disk" domain={[0, 100]} tick={{ fontSize: 11, fill: 'var(--chart-tick, #94a3b8)' }} axisLine={false} tickLine={false} tickFormatter={(v: number) => `${v}%`} />
                  <YAxis yAxisId="load" orientation="right" tick={{ fontSize: 11, fill: 'var(--chart-tick, #94a3b8)' }} axisLine={false} tickLine={false} />
                  <Tooltip
                    contentStyle={{ backgroundColor: 'var(--tooltip-bg, #fff)', border: '1px solid var(--tooltip-border, #e2e8f0)', borderRadius: '8px', fontSize: '12px', boxShadow: '0 4px 12px rgba(0,0,0,0.08)' }}
                    labelFormatter={(_, payload) => payload[0]?.payload?.date ?? ''}
                  />
                  <Line yAxisId="disk" type="monotone" dataKey="diskPercent" name="Disk %" stroke={CHART_COLORS.critical} strokeWidth={1.5} dot={false} />
                  <Line yAxisId="load" type="monotone" dataKey="loadAvg1" name="Load 1m" stroke={CHART_COLORS.healthy} strokeWidth={1.5} dot={false} />
                </LineChart>
              </ResponsiveContainer>
            </div>
          </div>
        ) : (
          <div className="flex h-48 items-center justify-center text-sm text-slate-400">
            No historical data available yet. Metrics are collected on each SSH check run.
          </div>
        )}
      </div>

      {/* Top processes table */}
      {s?.topProcesses && s.topProcesses.length > 0 && (
        <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
          <div className="flex items-center justify-between border-b border-slate-200 px-5 py-4 dark:border-slate-800">
            <div>
              <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">
                Top Processes
              </h2>
              <p className="mt-0.5 text-xs text-slate-500">Sorted by memory usage</p>
            </div>
            <LiveIndicator connected={liveConnected} />
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-100 bg-slate-50/50 dark:border-slate-800 dark:bg-slate-800/30">
                  <th className="px-4 py-2.5 text-left text-xs font-medium text-slate-500">PID</th>
                  <th className="px-4 py-2.5 text-left text-xs font-medium text-slate-500">User</th>
                  <SortableHeader label="CPU %" field="cpuPercent" current={sortField} dir={sortDir} onSort={toggleSort} />
                  <SortableHeader label="MEM %" field="memPercent" current={sortField} dir={sortDir} onSort={toggleSort} />
                  <SortableHeader label="RSS (MB)" field="memMB" current={sortField} dir={sortDir} onSort={toggleSort} />
                  <th className="px-4 py-2.5 text-left text-xs font-medium text-slate-500">Command</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {sortedProcesses.map((proc, i) => (
                  <tr key={`${proc.pid}-${i}`} className="transition-colors hover:bg-slate-50/50 dark:hover:bg-slate-800/30">
                    <td className="px-4 py-2 font-mono text-xs text-slate-600 dark:text-slate-400">{proc.pid}</td>
                    <td className="px-4 py-2 text-xs text-slate-600 dark:text-slate-400">{proc.user}</td>
                    <td className="px-4 py-2 text-right">
                      <span className={cn('text-xs font-medium', proc.cpuPercent > 50 ? 'text-red-500' : proc.cpuPercent > 20 ? 'text-amber-500' : 'text-slate-600 dark:text-slate-400')}>
                        {proc.cpuPercent.toFixed(1)}
                      </span>
                    </td>
                    <td className="px-4 py-2 text-right">
                      <div className="flex items-center justify-end gap-2">
                        <div className="h-1.5 w-16 overflow-hidden rounded-full bg-slate-200 dark:bg-slate-700">
                          <div
                            className={cn('h-full rounded-full transition-all',
                              proc.memPercent > 20 ? 'bg-red-500' : proc.memPercent > 10 ? 'bg-amber-500' : 'bg-emerald-500'
                            )}
                            style={{ width: `${Math.min(proc.memPercent, 100)}%` }}
                          />
                        </div>
                        <span className={cn('text-xs font-medium', proc.memPercent > 20 ? 'text-red-500' : proc.memPercent > 10 ? 'text-amber-500' : 'text-slate-600 dark:text-slate-400')}>
                          {proc.memPercent.toFixed(1)}
                        </span>
                      </div>
                    </td>
                    <td className="px-4 py-2 text-right text-xs text-slate-600 dark:text-slate-400">
                      {proc.memMB.toFixed(1)}
                    </td>
                    <td className="max-w-xs truncate px-4 py-2 font-mono text-xs text-slate-500">
                      {proc.command}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}

function BackLink() {
  return (
    <Link
      to="/servers"
      className="inline-flex items-center gap-1.5 text-sm text-slate-500 transition-colors hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200"
    >
      <ArrowLeft className="h-4 w-4" />
      Servers
    </Link>
  )
}

function SortableHeader({
  label, field, current, dir, onSort,
}: {
  label: string
  field: SortField
  current: SortField
  dir: SortDir
  onSort: (f: SortField) => void
}) {
  return (
    <th
      className="cursor-pointer select-none px-4 py-2.5 text-right text-xs font-medium text-slate-500 transition-colors hover:text-slate-700 dark:hover:text-slate-300"
      onClick={() => onSort(field)}
    >
      <span className="inline-flex items-center gap-1">
        {label}
        {current === field && (dir === 'desc' ? <ChevronDown className="h-3 w-3" /> : <ChevronUp className="h-3 w-3" />)}
      </span>
    </th>
  )
}
