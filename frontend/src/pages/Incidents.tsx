import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { useState } from 'react'
import { AlertTriangle, CheckCircle, Clock, Filter } from 'lucide-react'
import { incidentsApi } from '@/api/incidents'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { EmptyState } from '@/components/EmptyState'
import { ExportButton } from '@/components/ExportButton'
import { MetricCard } from '@/components/MetricCard'
import { cn, relativeTime, incidentStatusLabel, severityColor } from '@/lib/utils'
import { settingsApi } from '@/api/settings'
import { analyticsApi } from '@/api/analytics'
import { REFETCH_INTERVAL } from '@/lib/constants'

export default function Incidents() {
  const [statusFilter, setStatusFilter] = useState<string>('')
  const [severityFilter, setSeverityFilter] = useState<string>('')

  const { data: incidents, isLoading, error, refetch } = useQuery({
    queryKey: ['incidents', statusFilter, severityFilter],
    queryFn: () => incidentsApi.list({ status: statusFilter || undefined, severity: severityFilter || undefined, limit: 100 }),
    refetchInterval: REFETCH_INTERVAL,
  })

  const { data: stats } = useQuery({
    queryKey: ['analytics', 'incidents'],
    queryFn: analyticsApi.incidents,
  })

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message={error.message} retry={() => refetch()} />

  return (
    <div className="space-y-5 animate-fade-in">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">Incidents</h1>
          <p className="text-sm text-slate-500">{incidents?.length ?? 0} incidents</p>
        </div>
        <ExportButton downloadUrl={settingsApi.exportIncidents('csv')} />
      </div>

      {/* Summary cards */}
      {stats && (
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-5">
          <MetricCard label="Total" value={stats.total} />
          <MetricCard label="Open" value={stats.open} className={stats.open > 0 ? 'ring-1 ring-red-200 dark:ring-red-900' : ''} />
          <MetricCard label="Acknowledged" value={stats.acknowledged} />
          <MetricCard label="MTTA" value={stats.mttaMinutes > 0 ? `${stats.mttaMinutes.toFixed(0)}m` : '—'} subValue="Mean time to acknowledge" />
          <MetricCard label="MTTR" value={stats.mttrMinutes > 0 ? `${stats.mttrMinutes.toFixed(0)}m` : '—'} subValue="Mean time to resolve" />
        </div>
      )}

      {/* Filters */}
      <div className="flex items-center gap-3">
        <Filter className="h-4 w-4 text-slate-400" />
        <select
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          className="rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-600 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300"
        >
          <option value="">All statuses</option>
          <option value="open">Open</option>
          <option value="acknowledged">Acknowledged</option>
          <option value="resolved">Resolved</option>
        </select>
        <select
          value={severityFilter}
          onChange={(e) => setSeverityFilter(e.target.value)}
          className="rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-600 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300"
        >
          <option value="">All severities</option>
          <option value="critical">Critical</option>
          <option value="warning">Warning</option>
        </select>
      </div>

      {/* Incident list */}
      {incidents && incidents.length > 0 ? (
        <div className="overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
          <div className="divide-y divide-slate-100 dark:divide-slate-800">
            {incidents.map((inc) => (
              <Link
                key={inc.id}
                to={`/incidents/${inc.id}`}
                className="flex items-center gap-4 px-5 py-4 transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50"
              >
                <div className={cn(
                  'flex h-9 w-9 shrink-0 items-center justify-center rounded-full',
                  inc.severity === 'critical' ? 'bg-red-100 text-red-600 dark:bg-red-950/40 dark:text-red-400' : 'bg-amber-100 text-amber-600 dark:bg-amber-950/40 dark:text-amber-400',
                )}>
                  {inc.status === 'resolved' ? <CheckCircle className="h-4 w-4" /> : <AlertTriangle className="h-4 w-4" />}
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <p className="truncate text-sm font-medium text-slate-900 dark:text-slate-100">
                      {inc.checkName}
                    </p>
                    <span className={cn(
                      'rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase',
                      inc.status === 'open' ? 'bg-red-100 text-red-700 dark:bg-red-950/40 dark:text-red-400' :
                      inc.status === 'acknowledged' ? 'bg-amber-100 text-amber-700 dark:bg-amber-950/40 dark:text-amber-400' :
                      'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-400'
                    )}>
                      {incidentStatusLabel(inc.status)}
                    </span>
                    <span className={cn('text-xs font-medium capitalize', severityColor(inc.severity))}>
                      {inc.severity}
                    </span>
                  </div>
                  <p className="mt-0.5 truncate text-xs text-slate-500">{inc.message}</p>
                </div>
                <div className="shrink-0 text-right text-xs text-slate-400">
                  <div className="flex items-center gap-1">
                    <Clock className="h-3 w-3" />
                    {relativeTime(inc.startedAt)}
                  </div>
                </div>
              </Link>
            ))}
          </div>
        </div>
      ) : (
        <EmptyState
          title="No incidents found"
          description={statusFilter || severityFilter ? 'Try adjusting your filters.' : 'All systems are operating normally.'}
          icon={<CheckCircle className="h-6 w-6" />}
        />
      )}
    </div>
  )
}
