import { useQuery } from '@tanstack/react-query'
import { useParams, Link } from 'react-router-dom'
import { ArrowLeft, Server, Tag } from 'lucide-react'
import { checksApi } from '@/api/checks'
import { analyticsApi } from '@/api/analytics'
import { StatusBadge } from '@/components/StatusBadge'
import { MetricCard } from '@/components/MetricCard'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { ResponseTimeChart } from '@/components/charts/ResponseTimeChart'
import { formatDuration, formatUptime, relativeTime, checkTypeLabel } from '@/lib/utils'
import { REFETCH_INTERVAL } from '@/lib/constants'

export default function CheckDetail() {
  const { id } = useParams<{ id: string }>()

  const { data: detail, isLoading, error, refetch } = useQuery({
    queryKey: ['checks', id],
    queryFn: () => checksApi.get(id!),
    enabled: !!id,
    refetchInterval: REFETCH_INTERVAL,
  })

  const { data: rtData } = useQuery({
    queryKey: ['analytics', 'response-times', id, '24h'],
    queryFn: () => analyticsApi.responseTimes({ checkId: id, period: '24h', interval: '1h' }),
    enabled: !!id,
  })

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message={error.message} retry={() => refetch()} />
  if (!detail) return null

  const { config, latestResult, uptime24h, uptime7d, avgDurationMs, recentResults, openIncidents } = detail

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Breadcrumb */}
      <div className="flex items-center gap-3">
        <Link to="/checks" className="rounded-md p-1 text-slate-400 transition-colors hover:text-slate-600 dark:hover:text-slate-300">
          <ArrowLeft className="h-5 w-5" />
        </Link>
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">{config.name}</h1>
          <div className="mt-0.5 flex items-center gap-2 text-xs text-slate-500">
            <span className="rounded bg-slate-100 px-1.5 py-0.5 font-medium dark:bg-slate-800">{checkTypeLabel(config.type)}</span>
            {config.server && <span className="flex items-center gap-1"><Server className="h-3 w-3" /> {config.server}</span>}
            {config.target && <span className="truncate max-w-[200px]">{config.target}</span>}
          </div>
        </div>
        <div className="ml-auto">
          {latestResult && <StatusBadge status={latestResult.status} size="md" />}
        </div>
      </div>

      {/* Metric cards */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <MetricCard label="Uptime (24h)" value={formatUptime(uptime24h)} />
        <MetricCard label="Uptime (7d)" value={formatUptime(uptime7d)} />
        <MetricCard label="Avg Response" value={formatDuration(avgDurationMs)} />
        <MetricCard
          label="Last Check"
          value={latestResult ? formatDuration(latestResult.durationMs) : '—'}
          subValue={latestResult ? relativeTime(latestResult.finishedAt) : undefined}
        />
      </div>

      {/* Response time chart */}
      {rtData && rtData.length > 0 && (
        <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
          <h2 className="mb-4 text-sm font-semibold text-slate-900 dark:text-slate-100">Response Time (24h)</h2>
          <ResponseTimeChart data={rtData} showPercentiles />
        </div>
      )}

      {/* Open incidents */}
      {openIncidents && openIncidents.length > 0 && (
        <div className="rounded-xl border border-red-200 bg-red-50/50 p-5 dark:border-red-900 dark:bg-red-950/20">
          <h2 className="mb-3 text-sm font-semibold text-red-700 dark:text-red-400">
            Open Incidents ({openIncidents.length})
          </h2>
          <div className="space-y-2">
            {openIncidents.map(inc => (
              <Link
                key={inc.id}
                to={`/incidents/${inc.id}`}
                className="flex items-center gap-3 rounded-lg bg-white p-3 transition-colors hover:bg-red-50 dark:bg-slate-900 dark:hover:bg-slate-800"
              >
                <StatusBadge status={inc.severity} label={false} />
                <span className="text-sm text-slate-700 dark:text-slate-300">{inc.message}</span>
                <span className="ml-auto text-xs text-slate-400">{relativeTime(inc.startedAt)}</span>
              </Link>
            ))}
          </div>
        </div>
      )}

      {/* Recent results table */}
      <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
        <div className="border-b border-slate-100 px-5 py-3.5 dark:border-slate-800">
          <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Recent Results</h2>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-100 bg-slate-50/50 dark:border-slate-800 dark:bg-slate-800/30">
                <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">Status</th>
                <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">Response</th>
                <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">Message</th>
                <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">Time</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {recentResults.slice(0, 20).map(r => (
                <tr key={r.id}>
                  <td className="px-4 py-2.5"><StatusBadge status={r.status} /></td>
                  <td className="px-4 py-2.5 font-mono text-xs">{formatDuration(r.durationMs)}</td>
                  <td className="max-w-xs truncate px-4 py-2.5 text-slate-500 dark:text-slate-400">{r.message || '—'}</td>
                  <td className="px-4 py-2.5 text-xs text-slate-400">{relativeTime(r.finishedAt)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Config details */}
      <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
        <h2 className="mb-3 text-sm font-semibold text-slate-900 dark:text-slate-100">Configuration</h2>
        <dl className="grid gap-x-8 gap-y-2 sm:grid-cols-2 lg:grid-cols-3">
          {([
            ['ID', config.id],
            ['Type', checkTypeLabel(config.type)],
            ['Target', config.target],
            ['Server', config.server],
            ['Application', config.application],
            ['Timeout', config.timeoutSeconds ? `${config.timeoutSeconds}s` : 'Default'],
            ['Warning Threshold', config.warningThresholdMs ? `${config.warningThresholdMs}ms` : 'None'],
            ['Interval', config.intervalSeconds ? `${config.intervalSeconds}s` : 'Default'],
            ['Enabled', config.enabled !== false ? 'Yes' : 'No'],
          ] as const).filter(([, v]) => v).map(([label, value]) => (
            <div key={label} className="flex gap-2 text-sm">
              <dt className="font-medium text-slate-500 dark:text-slate-400">{label}:</dt>
              <dd className="text-slate-700 dark:text-slate-300">{value}</dd>
            </div>
          ))}
        </dl>
        {config.tags && config.tags.length > 0 && (
          <div className="mt-3 flex items-center gap-2">
            <Tag className="h-3.5 w-3.5 text-slate-400" />
            {config.tags.map(tag => (
              <span key={tag} className="rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-600 dark:bg-slate-800 dark:text-slate-400">{tag}</span>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
