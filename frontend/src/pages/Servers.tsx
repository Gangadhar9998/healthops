import { useQuery, useMutation } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import {
  Server, Monitor, Wifi, ExternalLink, CheckCircle2,
  AlertTriangle, XCircle, HelpCircle, ArrowRight, Play, RefreshCw,
  Activity,
} from 'lucide-react'
import { dashboardApi } from '@/api/dashboard'
import { checksApi } from '@/api/checks'
import { serversApi } from '@/api/servers'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { useToast } from '@/components/Toast'
import { cn, checkTypeLabel, formatDuration } from '@/lib/utils'
import { REFETCH_INTERVAL } from '@/lib/constants'
import { useLiveSummary } from '@/hooks/useLiveSummary'
import { LiveIndicator } from '@/components/db/LiveIndicator'
import type { RemoteServer, CheckConfig, CheckResult, StatusCount } from '@/types'

export default function Servers() {
  const toast = useToast()

  const { data: dashboard, isLoading: dashLoading, error: dashError, refetch: dashRefetch } = useQuery({
    queryKey: ['dashboard'],
    queryFn: dashboardApi.snapshot,
    refetchInterval: REFETCH_INTERVAL,
  })

  const { data: checks } = useQuery({
    queryKey: ['checks'],
    queryFn: checksApi.list,
    refetchInterval: REFETCH_INTERVAL,
  })

  const { data: results } = useQuery({
    queryKey: ['results'],
    queryFn: () => checksApi.results(),
    refetchInterval: REFETCH_INTERVAL,
  })

  const { data: remoteServers } = useQuery({
    queryKey: ['servers'],
    queryFn: serversApi.list,
  })

  const testMutation = useMutation({
    mutationFn: (id: string) => serversApi.test(id),
    onSuccess: (result) => {
      if (result.success) toast.success(`SSH OK: ${result.output}`)
      else toast.error(`SSH failed: ${result.error}`)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const live = useLiveSummary(!dashLoading && !dashError)

  if (dashLoading) return <LoadingState />
  if (dashError) return <ErrorState message={dashError.message} retry={() => dashRefetch()} />
  if (!dashboard) return null

  // Prefer live SSE summary over polled dashboard
  const summary = live.summary ?? dashboard.summary
  const byServer = summary.byServer

  // Build latest result map — prefer live SSE data
  const latestByCheck = live.latestByCheck.size > 0 ? live.latestByCheck : (() => {
    const map = new Map<string, CheckResult>()
    if (results) {
      for (const r of results) {
        const existing = map.get(r.checkId)
        if (!existing || new Date(r.finishedAt) > new Date(existing.finishedAt)) {
          map.set(r.checkId, r)
        }
      }
    }
    return map
  })()

  // Map remote servers by id for lookup
  const remoteById = new Map<string, RemoteServer>()
  if (remoteServers) {
    for (const rs of remoteServers) remoteById.set(rs.id, rs)
  }

  // Group checks by server label
  const checksByServer = new Map<string, CheckConfig[]>()
  if (checks) {
    for (const c of checks) {
      const srv = c.server || 'default'
      if (!checksByServer.has(srv)) checksByServer.set(srv, [])
      checksByServer.get(srv)!.push(c)
    }
  }

  const statusIcon = (status: string) => {
    switch (status) {
      case 'healthy': return <CheckCircle2 className="h-4 w-4 text-emerald-500" />
      case 'warning': return <AlertTriangle className="h-4 w-4 text-amber-500" />
      case 'critical': return <XCircle className="h-4 w-4 text-red-500" />
      default: return <HelpCircle className="h-4 w-4 text-slate-400" />
    }
  }

  const overallStatus = (counts: StatusCount) => {
    if (counts.critical > 0) return 'critical'
    if (counts.warning > 0) return 'warning'
    if (counts.unknown > 0 && counts.healthy === 0) return 'unknown'
    return 'healthy'
  }

  const statusColor = (status: string) => {
    switch (status) {
      case 'healthy': return 'border-emerald-200 bg-emerald-50/50 dark:border-emerald-900 dark:bg-emerald-950/20'
      case 'warning': return 'border-amber-200 bg-amber-50/50 dark:border-amber-900 dark:bg-amber-950/20'
      case 'critical': return 'border-red-200 bg-red-50/50 dark:border-red-900 dark:bg-red-950/20'
      default: return 'border-slate-200 bg-slate-50/50 dark:border-slate-800 dark:bg-slate-800/50'
    }
  }

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Header */}
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">Servers</h1>
          <p className="flex items-center gap-2 text-sm text-slate-500">
            <span>
              {Object.keys(byServer).length} server group{Object.keys(byServer).length !== 1 ? 's' : ''}
              {remoteServers && remoteServers.length > 0 && ` · ${remoteServers.length} remote`}
            </span>
            <LiveIndicator connected={live.connected} />
          </p>
        </div>
        <Link
          to="/settings?tab=servers"
          className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm font-medium text-slate-600 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
        >
          <Server className="h-4 w-4" />
          Manage Servers
        </Link>
      </div>

      {/* Server cards */}
      <div className="grid gap-4 lg:grid-cols-2">
        {Object.entries(byServer).map(([serverName, counts]) => {
          const status = overallStatus(counts)
          const healthyPct = counts.total > 0 ? Math.round((counts.healthy / counts.total) * 100) : 0
          const serverChecks = checksByServer.get(serverName) || []
          // Find if there's a remote server linked to this group
          const linkedRemote = remoteServers?.find(rs =>
            serverChecks.some(c => c.serverId === rs.id)
          )

          return (
            <div
              key={serverName}
              className={cn(
                'rounded-xl border p-5 transition-all',
                statusColor(status),
              )}
            >
              {/* Server header */}
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className={cn(
                    'flex h-10 w-10 items-center justify-center rounded-lg',
                    status === 'healthy' ? 'bg-emerald-100 dark:bg-emerald-900/40' :
                    status === 'warning' ? 'bg-amber-100 dark:bg-amber-900/40' :
                    status === 'critical' ? 'bg-red-100 dark:bg-red-900/40' :
                    'bg-slate-100 dark:bg-slate-800',
                  )}>
                    {linkedRemote ? (
                      <Wifi className={cn('h-5 w-5',
                        status === 'healthy' ? 'text-emerald-600 dark:text-emerald-400' :
                        status === 'warning' ? 'text-amber-600 dark:text-amber-400' :
                        status === 'critical' ? 'text-red-600 dark:text-red-400' :
                        'text-slate-500'
                      )} />
                    ) : (
                      <Monitor className={cn('h-5 w-5',
                        status === 'healthy' ? 'text-emerald-600 dark:text-emerald-400' :
                        status === 'warning' ? 'text-amber-600 dark:text-amber-400' :
                        status === 'critical' ? 'text-red-600 dark:text-red-400' :
                        'text-slate-500'
                      )} />
                    )}
                  </div>
                  <div>
                    <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100">{serverName}</h3>
                    {linkedRemote ? (
                      <p className="text-xs text-slate-500">
                        {linkedRemote.user}@{linkedRemote.host}:{linkedRemote.port}
                        {!linkedRemote.enabled && ' · disabled'}
                      </p>
                    ) : (
                      <p className="text-xs text-slate-500">Local server</p>
                    )}
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <span className={cn(
                    'rounded-full px-2.5 py-1 text-xs font-bold',
                    healthyPct === 100 ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-400' :
                    healthyPct >= 50 ? 'bg-amber-100 text-amber-700 dark:bg-amber-950/50 dark:text-amber-400' :
                    'bg-red-100 text-red-700 dark:bg-red-950/50 dark:text-red-400',
                  )}>
                    {healthyPct}%
                  </span>
                  {statusIcon(status)}
                </div>
              </div>

              {/* Health bar */}
              <div className="mt-3 flex h-2 overflow-hidden rounded-full bg-slate-200 dark:bg-slate-700">
                {counts.healthy > 0 && <div className="bg-emerald-500 transition-all" style={{ width: `${(counts.healthy / counts.total) * 100}%` }} />}
                {counts.warning > 0 && <div className="bg-amber-500 transition-all" style={{ width: `${(counts.warning / counts.total) * 100}%` }} />}
                {counts.critical > 0 && <div className="bg-red-500 transition-all" style={{ width: `${(counts.critical / counts.total) * 100}%` }} />}
                {counts.unknown > 0 && <div className="bg-slate-400 transition-all" style={{ width: `${(counts.unknown / counts.total) * 100}%` }} />}
              </div>

              {/* Summary counts */}
              <div className="mt-2 flex gap-4 text-xs text-slate-500">
                <span>{counts.total} check{counts.total !== 1 ? 's' : ''}</span>
                <span className="text-emerald-600">{counts.healthy} healthy</span>
                {counts.warning > 0 && <span className="text-amber-600">{counts.warning} warning</span>}
                {counts.critical > 0 && <span className="text-red-600">{counts.critical} critical</span>}
                {counts.unknown > 0 && <span className="text-slate-400">{counts.unknown} unknown</span>}
              </div>

              {/* Check list */}
              <div className="mt-4 divide-y divide-slate-200/60 rounded-lg border border-slate-200/60 bg-white dark:divide-slate-700/50 dark:border-slate-700/50 dark:bg-slate-900/50">
                {serverChecks.map(check => {
                  const lr = latestByCheck.get(check.id)
                  return (
                    <Link
                      key={check.id}
                      to={`/checks/${check.id}`}
                      className="flex items-center gap-3 px-3.5 py-2 text-sm transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/70"
                    >
                      {statusIcon(lr?.status || 'unknown')}
                      <span className="flex-1 truncate text-slate-700 dark:text-slate-300">{check.name}</span>
                      <span className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] font-mono text-slate-500 dark:bg-slate-800">
                        {checkTypeLabel(check.type)}
                      </span>
                      {lr && (
                        <span className="text-xs text-slate-400">{formatDuration(lr.durationMs)}</span>
                      )}
                    </Link>
                  )
                })}
                {serverChecks.length === 0 && (
                  <div className="px-3.5 py-3 text-center text-xs text-slate-400">
                    No checks assigned
                  </div>
                )}
              </div>

              {/* Actions */}
              <div className="mt-3 flex items-center justify-between">
                <Link
                  to={`/checks?server=${encodeURIComponent(serverName)}`}
                  className="inline-flex items-center gap-1 text-xs font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300"
                >
                  View all checks <ArrowRight className="h-3 w-3" />
                </Link>
                {linkedRemote && (
                  <div className="flex items-center gap-2">
                    <Link
                      to={`/servers/${linkedRemote.id}`}
                      className="inline-flex items-center gap-1.5 rounded-md border border-blue-200 bg-blue-50 px-2.5 py-1 text-xs font-medium text-blue-600 transition-colors hover:bg-blue-100 dark:border-blue-800 dark:bg-blue-950/30 dark:text-blue-400 dark:hover:bg-blue-900/40"
                    >
                      <Activity className="h-3 w-3" />
                      Live Stats
                    </Link>
                    <button
                      onClick={() => testMutation.mutate(linkedRemote.id)}
                      disabled={testMutation.isPending}
                      className="inline-flex items-center gap-1.5 rounded-md border border-slate-200 bg-white px-2.5 py-1 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
                    >
                      {testMutation.isPending ? (
                        <RefreshCw className="h-3 w-3 animate-spin" />
                      ) : (
                        <Play className="h-3 w-3" />
                      )}
                      Test SSH
                    </button>
                    <Link
                      to="/settings?tab=servers"
                      className="inline-flex items-center gap-1.5 rounded-md border border-slate-200 bg-white px-2.5 py-1 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
                    >
                      <ExternalLink className="h-3 w-3" />
                      Edit
                    </Link>
                  </div>
                )}
              </div>
            </div>
          )
        })}
      </div>

    </div>
  )
}
