import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import {
  Activity, AlertTriangle, Clock, Server,
  ArrowRight, Play, TrendingUp, Shield,
} from 'lucide-react'
import { dashboardApi } from '@/api/dashboard'
import { analyticsApi } from '@/api/analytics'
import { incidentsApi } from '@/api/incidents'
import { MetricCard } from '@/components/MetricCard'
import { StatusBadge } from '@/components/StatusBadge'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { ResponseTimeChart } from '@/components/charts/ResponseTimeChart'
import { StatusDistribution } from '@/components/charts/StatusDistribution'
import { ExportButton } from '@/components/ExportButton'
import { cn, relativeTime, formatDuration, formatUptime, checkTypeLabel } from '@/lib/utils'
import { REFETCH_INTERVAL } from '@/lib/constants'
import { settingsApi } from '@/api/settings'

export default function Dashboard() {
  const { data: dashboard, isLoading, error, refetch } = useQuery({
    queryKey: ['dashboard'],
    queryFn: dashboardApi.snapshot,
    refetchInterval: REFETCH_INTERVAL,
  })

  const { data: incidents } = useQuery({
    queryKey: ['incidents', 'active'],
    queryFn: () => incidentsApi.list({ status: 'open', limit: 5 }),
    refetchInterval: REFETCH_INTERVAL,
  })

  const { data: responseTimes } = useQuery({
    queryKey: ['analytics', 'response-times', '24h'],
    queryFn: () => analyticsApi.responseTimes({ period: '24h', interval: '1h' }),
    refetchInterval: REFETCH_INTERVAL,
  })

  const { data: uptime } = useQuery({
    queryKey: ['analytics', 'uptime', '7d'],
    queryFn: () => analyticsApi.uptime({ period: '7d' }),
    refetchInterval: REFETCH_INTERVAL,
  })

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message={error.message} retry={() => refetch()} />
  if (!dashboard) return null

  const { summary, state } = dashboard
  const healthPct = summary.enabledChecks > 0
    ? Math.round((summary.healthy / summary.enabledChecks) * 100)
    : 0

  const avgUptime = uptime && uptime.length > 0
    ? uptime.reduce((sum, u) => sum + u.uptimePct, 0) / uptime.length
    : null

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">Dashboard</h1>
          <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">
            {summary.lastRunAt ? `Last check ${relativeTime(summary.lastRunAt)}` : 'No checks run yet'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <ExportButton downloadUrl={settingsApi.exportResults('csv')} />
          <button
            onClick={() => dashboardApi.runNow()}
            className="inline-flex items-center gap-1.5 rounded-lg bg-blue-600 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700"
          >
            <Play className="h-3.5 w-3.5" />
            Run checks
          </button>
        </div>
      </div>

      {/* Hero metrics row */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <MetricCard
          label="System Health"
          value={`${healthPct}%`}
          subValue={`${summary.healthy} of ${summary.enabledChecks} healthy`}
          icon={<Shield className="h-5 w-5" />}
          className={cn(
            healthPct === 100 && 'ring-1 ring-emerald-200 dark:ring-emerald-900',
            healthPct < 100 && healthPct >= 80 && 'ring-1 ring-amber-200 dark:ring-amber-900',
            healthPct < 80 && 'ring-1 ring-red-200 dark:ring-red-900',
          )}
        />
        <MetricCard
          label="Active Incidents"
          value={incidents?.length ?? 0}
          subValue={incidents && incidents.length > 0 ? `${incidents.filter(i => i.severity === 'critical').length} critical` : 'All clear'}
          icon={<AlertTriangle className="h-5 w-5" />}
        />
        <MetricCard
          label="Avg Uptime (7d)"
          value={avgUptime != null ? formatUptime(avgUptime) : '—'}
          subValue={uptime ? `${uptime.length} checks tracked` : undefined}
          icon={<TrendingUp className="h-5 w-5" />}
        />
        <MetricCard
          label="Total Checks"
          value={summary.totalChecks}
          subValue={`${summary.enabledChecks} enabled`}
          icon={<Activity className="h-5 w-5" />}
        />
      </div>

      {/* Charts row */}
      <div className="grid gap-4 lg:grid-cols-3">
        {/* Response Time chart - takes 2 cols */}
        <div className="rounded-xl border border-slate-200 bg-white p-5 lg:col-span-2 dark:border-slate-800 dark:bg-slate-900">
          <div className="mb-4 flex items-center justify-between">
            <div>
              <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Response Times</h2>
              <p className="text-xs text-slate-400">Last 24 hours, hourly average</p>
            </div>
            <Link
              to="/analytics"
              className="inline-flex items-center gap-1 text-xs font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400"
            >
              Details <ArrowRight className="h-3 w-3" />
            </Link>
          </div>
          {responseTimes && responseTimes.length > 0 ? (
            <ResponseTimeChart data={responseTimes} />
          ) : (
            <div className="flex h-[260px] items-center justify-center text-sm text-slate-400">
              No response time data yet
            </div>
          )}
        </div>

        {/* Status distribution */}
        <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
          <h2 className="mb-4 text-sm font-semibold text-slate-900 dark:text-slate-100">Check Status</h2>
          <div className="flex flex-col items-center">
            <StatusDistribution
              healthy={summary.healthy}
              warning={summary.warning}
              critical={summary.critical}
              unknown={summary.unknown}
            />
            <div className="mt-4 grid w-full grid-cols-2 gap-2">
              {([
                ['Healthy', summary.healthy, 'bg-emerald-500'],
                ['Warning', summary.warning, 'bg-amber-500'],
                ['Critical', summary.critical, 'bg-red-500'],
                ['Unknown', summary.unknown, 'bg-slate-400'],
              ] as const).map(([label, count, dot]) => (
                <div key={label} className="flex items-center gap-2 text-xs text-slate-600 dark:text-slate-400">
                  <span className={cn('h-2 w-2 rounded-full', dot)} />
                  {label}: {count}
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>

      {/* Bottom section: incidents + latest checks */}
      <div className="grid gap-4 lg:grid-cols-2">
        {/* Active Incidents */}
        <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
          <div className="flex items-center justify-between border-b border-slate-100 px-5 py-3.5 dark:border-slate-800">
            <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Active Incidents</h2>
            <Link
              to="/incidents"
              className="inline-flex items-center gap-1 text-xs font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400"
            >
              View all <ArrowRight className="h-3 w-3" />
            </Link>
          </div>
          <div className="divide-y divide-slate-100 dark:divide-slate-800">
            {incidents && incidents.length > 0 ? (
              incidents.slice(0, 5).map((inc) => (
                <Link
                  key={inc.id}
                  to={`/incidents/${inc.id}`}
                  className="flex items-center gap-3 px-5 py-3 transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50"
                >
                  <StatusBadge status={inc.severity} label={false} size="md" />
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-slate-900 dark:text-slate-100">
                      {inc.checkName}
                    </p>
                    <p className="truncate text-xs text-slate-500">{inc.message}</p>
                  </div>
                  <span className="shrink-0 text-xs text-slate-400">{relativeTime(inc.startedAt)}</span>
                </Link>
              ))
            ) : (
              <div className="px-5 py-8 text-center text-sm text-slate-400">
                No active incidents
              </div>
            )}
          </div>
        </div>

        {/* Latest Check Results */}
        <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
          <div className="flex items-center justify-between border-b border-slate-100 px-5 py-3.5 dark:border-slate-800">
            <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Latest Results</h2>
            <Link
              to="/checks"
              className="inline-flex items-center gap-1 text-xs font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400"
            >
              All checks <ArrowRight className="h-3 w-3" />
            </Link>
          </div>
          <div className="divide-y divide-slate-100 dark:divide-slate-800">
            {summary.latest && summary.latest.length > 0 ? (
              summary.latest.slice(0, 8).map((result) => (
                <Link
                  key={result.id}
                  to={`/checks/${result.checkId}`}
                  className="flex items-center gap-3 px-5 py-2.5 transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50"
                >
                  <StatusBadge status={result.status} label={false} />
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm text-slate-700 dark:text-slate-300">{result.name}</p>
                  </div>
                  <div className="flex items-center gap-3 text-xs text-slate-400">
                    <span className="rounded bg-slate-100 px-1.5 py-0.5 font-mono text-[10px] dark:bg-slate-800">
                      {checkTypeLabel(result.type)}
                    </span>
                    <span className="flex items-center gap-1">
                      <Clock className="h-3 w-3" />
                      {formatDuration(result.durationMs)}
                    </span>
                  </div>
                </Link>
              ))
            ) : (
              <div className="px-5 py-8 text-center text-sm text-slate-400">
                No results yet
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Server breakdown */}
      {Object.keys(summary.byServer).length > 0 && (
        <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
          <h2 className="mb-4 text-sm font-semibold text-slate-900 dark:text-slate-100">
            <Server className="mr-1.5 inline-block h-4 w-4 text-slate-400" />
            By Server
          </h2>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {Object.entries(summary.byServer).map(([server, counts]) => {
              const total = counts.total
              const healthyPct = total > 0 ? Math.round((counts.healthy / total) * 100) : 0
              return (
                <div key={server} className="rounded-lg bg-slate-50 p-3 dark:bg-slate-800/50">
                  <div className="flex items-center justify-between">
                    <span className="text-sm font-medium text-slate-700 dark:text-slate-300">{server}</span>
                    <span className={cn(
                      'text-xs font-semibold',
                      healthyPct === 100 ? 'text-emerald-600' : healthyPct >= 80 ? 'text-amber-600' : 'text-red-600',
                    )}>
                      {healthyPct}%
                    </span>
                  </div>
                  <div className="mt-2 flex h-1.5 overflow-hidden rounded-full bg-slate-200 dark:bg-slate-700">
                    {counts.healthy > 0 && <div className="bg-emerald-500" style={{ width: `${(counts.healthy / total) * 100}%` }} />}
                    {counts.warning > 0 && <div className="bg-amber-500" style={{ width: `${(counts.warning / total) * 100}%` }} />}
                    {counts.critical > 0 && <div className="bg-red-500" style={{ width: `${(counts.critical / total) * 100}%` }} />}
                    {counts.unknown > 0 && <div className="bg-slate-400" style={{ width: `${(counts.unknown / total) * 100}%` }} />}
                  </div>
                  <div className="mt-1.5 flex gap-3 text-[10px] text-slate-400">
                    <span>{counts.healthy} healthy</span>
                    {counts.warning > 0 && <span>{counts.warning} warn</span>}
                    {counts.critical > 0 && <span>{counts.critical} crit</span>}
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      )}

      {/* Checks by type */}
      {state.checks.length > 0 && (
        <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
          <h2 className="mb-3 text-sm font-semibold text-slate-900 dark:text-slate-100">Checks by Type</h2>
          <div className="flex flex-wrap gap-2">
            {Object.entries(
              state.checks.reduce<Record<string, number>>((acc, c) => {
                acc[c.type] = (acc[c.type] || 0) + 1
                return acc
              }, {})
            ).map(([type, count]) => (
              <span
                key={type}
                className="inline-flex items-center gap-1.5 rounded-full bg-slate-100 px-3 py-1 text-xs font-medium text-slate-600 dark:bg-slate-800 dark:text-slate-400"
              >
                {checkTypeLabel(type)}
                <span className="font-bold">{count}</span>
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
