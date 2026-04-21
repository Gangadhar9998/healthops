import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import {
  Activity, AlertTriangle, Clock, Server, Monitor, Wifi,
  ArrowRight, Play, TrendingUp, Shield, CheckCircle2,
  XCircle, HelpCircle, Calendar, ChevronDown, ChevronRight,
  Cpu, HardDrive, MemoryStick, Gauge, RefreshCw,
} from 'lucide-react'
import { format } from 'date-fns'
import { dashboardApi } from '@/api/dashboard'
import { analyticsApi } from '@/api/analytics'
import { incidentsApi } from '@/api/incidents'
import { checksApi } from '@/api/checks'
import { serversApi } from '@/api/servers'
import { MetricCard } from '@/components/MetricCard'
import { StatusBadge } from '@/components/StatusBadge'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { ResponseTimeChart } from '@/components/charts/ResponseTimeChart'
import { StatusDistribution } from '@/components/charts/StatusDistribution'
import { cn, relativeTime, formatDuration, formatUptime, checkTypeLabel } from '@/lib/utils'
import { useToast } from '@/components/Toast'
import { REFETCH_INTERVAL } from '@/lib/constants'
import type { CheckConfig, CheckResult, RemoteServer, StatusCount, ServerSnapshot, Incident } from '@/types'

const PERIOD_OPTIONS = [
  { label: 'Today', value: '24h' },
  { label: '7 Days', value: '7d' },
  { label: '30 Days', value: '30d' },
] as const

type Period = typeof PERIOD_OPTIONS[number]['value']

export default function Dashboard() {
  const [period, setPeriod] = useState<Period>('24h')
  const [expandedServers, setExpandedServers] = useState<Set<string>>(new Set())
  const [runningChecks, setRunningChecks] = useState(false)
  const queryClient = useQueryClient()
  const toast = useToast()

  const { data: dashboard, isLoading, error, refetch } = useQuery({
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

  const { data: incidents } = useQuery({
    queryKey: ['incidents', 'active'],
    queryFn: () => incidentsApi.list({ status: 'open', limit: 50 }),
    refetchInterval: REFETCH_INTERVAL,
  })

  const { data: allIncidents } = useQuery({
    queryKey: ['incidents', 'all', period],
    queryFn: () => incidentsApi.list({ limit: 200 }),
    refetchInterval: REFETCH_INTERVAL,
  })

  const { data: remoteServers } = useQuery({
    queryKey: ['servers'],
    queryFn: serversApi.list,
  })

  const { data: responseTimes } = useQuery({
    queryKey: ['analytics', 'response-times', period],
    queryFn: () => analyticsApi.responseTimes({ period, interval: period === '24h' ? '1h' : period === '7d' ? '6h' : '1d' }),
    refetchInterval: REFETCH_INTERVAL,
  })

  const { data: uptime } = useQuery({
    queryKey: ['analytics', 'uptime', period],
    queryFn: () => analyticsApi.uptime({ period }),
    refetchInterval: REFETCH_INTERVAL,
  })

  const { data: incidentStats } = useQuery({
    queryKey: ['analytics', 'incidents'],
    queryFn: () => analyticsApi.incidents(),
    refetchInterval: REFETCH_INTERVAL,
  })

  // Fetch live metrics for each remote server
  const remoteServerIds = remoteServers?.map(s => s.id) ?? []
  const metricsQueries = useQuery({
    queryKey: ['server-metrics-batch', remoteServerIds.join(',')],
    queryFn: async () => {
      if (!remoteServers || remoteServers.length === 0) return {}
      const entries = await Promise.allSettled(
        remoteServers.map(async s => {
          const snap = await serversApi.metrics(s.id)
          return [s.id, snap] as const
        })
      )
      const map: Record<string, ServerSnapshot> = {}
      for (const entry of entries) {
        if (entry.status === 'fulfilled') {
          map[entry.value[0]] = entry.value[1]
        }
      }
      return map
    },
    refetchInterval: REFETCH_INTERVAL,
    enabled: remoteServerIds.length > 0,
  })

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message={error.message} retry={() => refetch()} />
  if (!dashboard) return null

  const { summary } = dashboard
  const serverMetrics = metricsQueries.data ?? {}

  const healthPct = summary.enabledChecks > 0
    ? Math.round((summary.healthy / summary.enabledChecks) * 100)
    : 0

  const avgUptime = uptime && uptime.length > 0
    ? uptime.reduce((sum, u) => sum + u.uptimePct, 0) / uptime.length
    : null

  // Period-filtered incident stats
  const periodStart = new Date()
  if (period === '24h') periodStart.setHours(periodStart.getHours() - 24)
  else if (period === '7d') periodStart.setDate(periodStart.getDate() - 7)
  else periodStart.setDate(periodStart.getDate() - 30)

  const periodIncidents = allIncidents?.items.filter(
    inc => new Date(inc.startedAt) >= periodStart
  ) ?? []
  const resolvedInPeriod = periodIncidents.filter(i => i.status === 'resolved').length
  const openCount = incidents?.items.length ?? 0

  // Build latest result map
  const latestByCheck = new Map<string, CheckResult>()
  if (results) {
    for (const r of results) {
      const existing = latestByCheck.get(r.checkId)
      if (!existing || new Date(r.finishedAt) > new Date(existing.finishedAt)) {
        latestByCheck.set(r.checkId, r)
      }
    }
  }

  // Build uptime map by check ID
  const uptimeByCheck = new Map<string, number>()
  if (uptime) {
    for (const u of uptime) uptimeByCheck.set(u.checkId, u.uptimePct)
  }

  // Map remote servers by id
  const remoteById = new Map<string, RemoteServer>()
  if (remoteServers) {
    for (const rs of remoteServers) remoteById.set(rs.id, rs)
  }

  // Group checks by server
  const checksByServer = new Map<string, CheckConfig[]>()
  if (checks) {
    for (const c of checks) {
      const srv = c.server || 'Local'
      if (!checksByServer.has(srv)) checksByServer.set(srv, [])
      checksByServer.get(srv)!.push(c)
    }
  }

  // Build incidents by check
  const incidentsByCheck = new Map<string, Incident[]>()
  if (incidents) {
    for (const inc of incidents.items) {
      if (!incidentsByCheck.has(inc.checkId)) incidentsByCheck.set(inc.checkId, [])
      incidentsByCheck.get(inc.checkId)!.push(inc)
    }
  }

  const toggleServer = (name: string) => {
    setExpandedServers(prev => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  const statusIcon = (status: string) => {
    switch (status) {
      case 'healthy': return <CheckCircle2 className="h-3.5 w-3.5 text-emerald-500" />
      case 'warning': return <AlertTriangle className="h-3.5 w-3.5 text-amber-500" />
      case 'critical': return <XCircle className="h-3.5 w-3.5 text-red-500" />
      default: return <HelpCircle className="h-3.5 w-3.5 text-slate-400" />
    }
  }

  const overallStatus = (counts: StatusCount) => {
    if (counts.critical > 0) return 'critical'
    if (counts.warning > 0) return 'warning'
    if (counts.unknown > 0 && counts.healthy === 0) return 'unknown'
    return 'healthy'
  }

  const todayStr = format(new Date(), 'EEEE, MMMM d, yyyy')

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Header with date + period selector */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">Dashboard</h1>
          <div className="mt-1 flex items-center gap-2 text-sm text-slate-500 dark:text-slate-400">
            <Calendar className="h-3.5 w-3.5" />
            <span>{todayStr}</span>
            {summary.lastRunAt && (
              <>
                <span className="text-slate-300 dark:text-slate-600">|</span>
                <span>Last check {relativeTime(summary.lastRunAt)}</span>
              </>
            )}
          </div>
        </div>
        <div className="flex items-center gap-2">
          {/* Period selector */}
          <div className="inline-flex rounded-lg border border-slate-200 bg-white p-0.5 dark:border-slate-700 dark:bg-slate-800">
            {PERIOD_OPTIONS.map(opt => (
              <button
                key={opt.value}
                onClick={() => setPeriod(opt.value)}
                className={cn(
                  'rounded-md px-3 py-1.5 text-xs font-medium transition-colors',
                  period === opt.value
                    ? 'bg-blue-600 text-white shadow-sm'
                    : 'text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-200',
                )}
              >
                {opt.label}
              </button>
            ))}
          </div>
          <button
            onClick={async () => {
              if (runningChecks) return
              setRunningChecks(true)
              try {
                const result = await dashboardApi.runNow()
                toast.success(`Checks completed: ${result.summary.healthy} healthy, ${result.summary.warning} warning, ${result.summary.critical} critical`)
                queryClient.invalidateQueries({ queryKey: ['dashboard'] })
                queryClient.invalidateQueries({ queryKey: ['checks'] })
                queryClient.invalidateQueries({ queryKey: ['results'] })
                queryClient.invalidateQueries({ queryKey: ['incidents'] })
                queryClient.invalidateQueries({ queryKey: ['analytics'] })
              } catch (e: any) {
                toast.error(e.message || 'Failed to run checks')
              } finally {
                setRunningChecks(false)
              }
            }}
            disabled={runningChecks}
            className={cn(
              'inline-flex items-center gap-1.5 rounded-lg px-3.5 py-2 text-sm font-medium text-white transition-colors',
              runningChecks ? 'bg-blue-400 cursor-not-allowed' : 'bg-blue-600 hover:bg-blue-700',
            )}
          >
            {runningChecks ? (
              <RefreshCw className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Play className="h-3.5 w-3.5" />
            )}
            {runningChecks ? 'Running...' : 'Run Now'}
          </button>
        </div>
      </div>

      {/* Top metrics */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
        <MetricCard
          label="System Health"
          value={`${healthPct}%`}
          subValue={`${summary.healthy}/${summary.enabledChecks} healthy`}
          icon={<Shield className="h-5 w-5" />}
          className={cn(
            healthPct === 100 && 'ring-1 ring-emerald-200 dark:ring-emerald-900',
            healthPct < 100 && healthPct >= 80 && 'ring-1 ring-amber-200 dark:ring-amber-900',
            healthPct < 80 && 'ring-1 ring-red-200 dark:ring-red-900',
          )}
        />
        <MetricCard
          label="Open Incidents"
          value={openCount}
          subValue={openCount > 0 ? `${incidents?.items.filter(i => i.severity === 'critical').length ?? 0} critical` : 'All clear'}
          icon={<AlertTriangle className="h-5 w-5" />}
          className={cn(openCount > 0 && 'ring-1 ring-red-200 dark:ring-red-900')}
        />
        <MetricCard
          label={`Uptime (${period === '24h' ? 'Today' : period})`}
          value={avgUptime != null ? formatUptime(avgUptime) : '--'}
          subValue={uptime ? `${uptime.length} checks tracked` : undefined}
          icon={<TrendingUp className="h-5 w-5" />}
        />
        <MetricCard
          label={`Incidents (${period === '24h' ? 'Today' : period})`}
          value={periodIncidents.length}
          subValue={`${resolvedInPeriod} resolved`}
          icon={<Activity className="h-5 w-5" />}
        />
        <MetricCard
          label="MTTR"
          value={incidentStats?.mttrMinutes != null && incidentStats.mttrMinutes > 0 ? `${Math.round(incidentStats.mttrMinutes)}m` : '--'}
          subValue={incidentStats?.mttaMinutes != null && incidentStats.mttaMinutes > 0 ? `MTTA: ${Math.round(incidentStats.mttaMinutes)}m` : 'No resolved incidents'}
          icon={<Clock className="h-5 w-5" />}
        />
      </div>

      {/* Charts row */}
      <div className="grid gap-4 lg:grid-cols-3">
        <div className="rounded-xl border border-slate-200 bg-white p-5 lg:col-span-2 dark:border-slate-800 dark:bg-slate-900">
          <div className="mb-4 flex items-center justify-between">
            <div>
              <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Response Times</h2>
              <p className="text-xs text-slate-400">
                {period === '24h' ? 'Last 24 hours, hourly avg' : period === '7d' ? 'Last 7 days, 6h avg' : 'Last 30 days, daily avg'}
              </p>
            </div>
            <Link to="/analytics" className="inline-flex items-center gap-1 text-xs font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400">
              Details <ArrowRight className="h-3 w-3" />
            </Link>
          </div>
          {responseTimes && responseTimes.length > 0 ? (
            <ResponseTimeChart data={responseTimes} />
          ) : (
            <div className="flex h-[260px] items-center justify-center text-sm text-slate-400">No response time data yet</div>
          )}
        </div>

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

      {/* Server-based health view */}
      <div>
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">
            <Server className="mr-1.5 inline-block h-4 w-4 text-slate-400" />
            Servers &amp; Health Checks
          </h2>
          <span className="text-xs text-slate-400">
            {Object.keys(summary.byServer).length} server{Object.keys(summary.byServer).length !== 1 ? 's' : ''}
            {' '}· {summary.enabledChecks} checks
          </span>
        </div>

        <div className="space-y-3">
          {Object.entries(summary.byServer).map(([serverName, counts]) => {
            const status = overallStatus(counts)
            const healthyPct = counts.total > 0 ? Math.round((counts.healthy / counts.total) * 100) : 0
            const serverChecks = checksByServer.get(serverName) || []
            const isExpanded = expandedServers.has(serverName)
            const linkedRemote = remoteServers?.find(rs => serverChecks.some(c => c.serverId === rs.id))
            const metrics = linkedRemote ? serverMetrics[linkedRemote.id] : undefined
            const serverIncidentCount = serverChecks.reduce((n, c) => n + (incidentsByCheck.get(c.id)?.length ?? 0), 0)

            return (
              <div
                key={serverName}
                className={cn(
                  'rounded-xl border transition-all',
                  status === 'healthy' ? 'border-emerald-200 dark:border-emerald-900/60' :
                  status === 'warning' ? 'border-amber-200 dark:border-amber-900/60' :
                  status === 'critical' ? 'border-red-200 dark:border-red-900/60' :
                  'border-slate-200 dark:border-slate-800',
                  'bg-white dark:bg-slate-900',
                )}
              >
                {/* Server header - clickable to expand */}
                <button
                  onClick={() => toggleServer(serverName)}
                  className="flex w-full items-center gap-3 px-5 py-4 text-left transition-colors hover:bg-slate-50/50 dark:hover:bg-slate-800/30"
                >
                  {isExpanded ? (
                    <ChevronDown className="h-4 w-4 shrink-0 text-slate-400" />
                  ) : (
                    <ChevronRight className="h-4 w-4 shrink-0 text-slate-400" />
                  )}

                  {/* Server icon */}
                  <div className={cn(
                    'flex h-9 w-9 shrink-0 items-center justify-center rounded-lg',
                    status === 'healthy' ? 'bg-emerald-100 dark:bg-emerald-900/40' :
                    status === 'warning' ? 'bg-amber-100 dark:bg-amber-900/40' :
                    status === 'critical' ? 'bg-red-100 dark:bg-red-900/40' :
                    'bg-slate-100 dark:bg-slate-800',
                  )}>
                    {linkedRemote ? (
                      <Wifi className={cn('h-4 w-4',
                        status === 'healthy' ? 'text-emerald-600' : status === 'warning' ? 'text-amber-600' : status === 'critical' ? 'text-red-600' : 'text-slate-500'
                      )} />
                    ) : (
                      <Monitor className={cn('h-4 w-4',
                        status === 'healthy' ? 'text-emerald-600' : status === 'warning' ? 'text-amber-600' : status === 'critical' ? 'text-red-600' : 'text-slate-500'
                      )} />
                    )}
                  </div>

                  {/* Server name + meta */}
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">{serverName}</span>
                      {linkedRemote && (
                        <span className="text-[10px] text-slate-400">
                          {linkedRemote.user}@{linkedRemote.host}:{linkedRemote.port}
                        </span>
                      )}
                    </div>
                    <div className="mt-0.5 flex items-center gap-3 text-xs text-slate-500">
                      <span>{counts.total} check{counts.total !== 1 ? 's' : ''}</span>
                      <span className="text-emerald-600">{counts.healthy} ok</span>
                      {counts.warning > 0 && <span className="text-amber-600">{counts.warning} warn</span>}
                      {counts.critical > 0 && <span className="text-red-600">{counts.critical} crit</span>}
                      {serverIncidentCount > 0 && (
                        <span className="text-red-600 font-medium">{serverIncidentCount} incident{serverIncidentCount !== 1 ? 's' : ''}</span>
                      )}
                    </div>
                  </div>

                  {/* Right side: metrics preview + health % */}
                  <div className="hidden items-center gap-4 sm:flex">
                    {metrics && (
                      <div className="flex items-center gap-3 text-xs text-slate-500">
                        <span className="flex items-center gap-1" title="CPU">
                          <Cpu className="h-3 w-3" />
                          <span className={cn(metrics.cpuPercent > 90 ? 'text-red-600 font-medium' : metrics.cpuPercent > 70 ? 'text-amber-600' : '')}>
                            {metrics.cpuPercent.toFixed(0)}%
                          </span>
                        </span>
                        <span className="flex items-center gap-1" title="Memory">
                          <MemoryStick className="h-3 w-3" />
                          <span className={cn(metrics.memoryPercent > 90 ? 'text-red-600 font-medium' : metrics.memoryPercent > 80 ? 'text-amber-600' : '')}>
                            {metrics.memoryPercent.toFixed(0)}%
                          </span>
                        </span>
                        <span className="flex items-center gap-1" title="Disk">
                          <HardDrive className="h-3 w-3" />
                          {metrics.diskPercent}%
                        </span>
                        <span className="flex items-center gap-1" title="Load">
                          <Gauge className="h-3 w-3" />
                          {metrics.loadAvg1.toFixed(1)}
                        </span>
                      </div>
                    )}

                    {/* Health bar */}
                    <div className="flex items-center gap-2">
                      <div className="flex h-2 w-24 overflow-hidden rounded-full bg-slate-200 dark:bg-slate-700">
                        {counts.healthy > 0 && <div className="bg-emerald-500 transition-all" style={{ width: `${(counts.healthy / counts.total) * 100}%` }} />}
                        {counts.warning > 0 && <div className="bg-amber-500 transition-all" style={{ width: `${(counts.warning / counts.total) * 100}%` }} />}
                        {counts.critical > 0 && <div className="bg-red-500 transition-all" style={{ width: `${(counts.critical / counts.total) * 100}%` }} />}
                        {counts.unknown > 0 && <div className="bg-slate-400 transition-all" style={{ width: `${(counts.unknown / counts.total) * 100}%` }} />}
                      </div>
                      <span className={cn(
                        'min-w-[2.5rem] text-right text-xs font-bold',
                        healthyPct === 100 ? 'text-emerald-600' : healthyPct >= 50 ? 'text-amber-600' : 'text-red-600',
                      )}>
                        {healthyPct}%
                      </span>
                    </div>
                  </div>
                </button>

                {/* Expanded: individual checks */}
                {isExpanded && (
                  <div className="border-t border-slate-100 dark:border-slate-800">
                    {/* Remote server metrics row */}
                    {metrics && linkedRemote && (
                      <div className="border-b border-slate-100 bg-slate-50/50 px-5 py-3 dark:border-slate-800 dark:bg-slate-800/20">
                        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-6">
                          <MiniMetric label="CPU" value={`${metrics.cpuPercent.toFixed(1)}%`} warn={metrics.cpuPercent > 80} crit={metrics.cpuPercent > 95} />
                          <MiniMetric label="Memory" value={`${metrics.memoryPercent.toFixed(1)}%`} sub={`${metrics.memoryUsedMB.toFixed(0)}/${metrics.memoryTotalMB.toFixed(0)} MB`} warn={metrics.memoryPercent > 85} crit={metrics.memoryPercent > 95} />
                          <MiniMetric label="Disk" value={`${metrics.diskPercent}%`} sub={`${metrics.diskUsedGB.toFixed(1)}/${metrics.diskTotalGB.toFixed(1)} GB`} warn={metrics.diskPercent > 85} crit={metrics.diskPercent > 95} />
                          <MiniMetric label="Load (1m)" value={metrics.loadAvg1.toFixed(2)} sub={`5m: ${metrics.loadAvg5.toFixed(2)} · 15m: ${metrics.loadAvg15.toFixed(2)}`} />
                          <MiniMetric label="Uptime" value={metrics.uptimeHours >= 24 ? `${Math.floor(metrics.uptimeHours / 24)}d ${Math.floor(metrics.uptimeHours % 24)}h` : `${metrics.uptimeHours.toFixed(1)}h`} />
                          <div className="flex items-end">
                            <Link
                              to={`/servers/${linkedRemote.id}`}
                              className="inline-flex items-center gap-1 rounded-md border border-blue-200 bg-blue-50 px-2.5 py-1 text-xs font-medium text-blue-600 transition-colors hover:bg-blue-100 dark:border-blue-800 dark:bg-blue-950/30 dark:text-blue-400"
                            >
                              <Activity className="h-3 w-3" />
                              Live Stats
                            </Link>
                          </div>
                        </div>
                      </div>
                    )}

                    {/* Individual checks list */}
                    <div className="divide-y divide-slate-100 dark:divide-slate-800">
                      {serverChecks.map(check => {
                        const lr = latestByCheck.get(check.id)
                        const checkUptime = uptimeByCheck.get(check.id)
                        const checkIncidents = incidentsByCheck.get(check.id) ?? []

                        return (
                          <Link
                            key={check.id}
                            to={`/checks/${check.id}`}
                            className="flex items-center gap-3 px-5 py-2.5 text-sm transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50"
                          >
                            {statusIcon(lr?.status || 'unknown')}
                            <span className="min-w-0 flex-1 truncate text-slate-700 dark:text-slate-300">
                              {check.name}
                            </span>
                            <span className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] font-mono text-slate-500 dark:bg-slate-800">
                              {checkTypeLabel(check.type)}
                            </span>
                            {checkUptime != null && (
                              <span className={cn(
                                'text-xs tabular-nums',
                                checkUptime >= 99.9 ? 'text-emerald-600' : checkUptime >= 95 ? 'text-amber-600' : 'text-red-600',
                              )}>
                                {formatUptime(checkUptime)}
                              </span>
                            )}
                            {lr && (
                              <span className="flex items-center gap-1 text-xs text-slate-400 tabular-nums">
                                <Clock className="h-3 w-3" />
                                {formatDuration(lr.durationMs)}
                              </span>
                            )}
                            {checkIncidents.length > 0 && (
                              <span className="flex items-center gap-1 rounded-full bg-red-100 px-2 py-0.5 text-[10px] font-medium text-red-700 dark:bg-red-950/50 dark:text-red-400">
                                <AlertTriangle className="h-2.5 w-2.5" />
                                {checkIncidents.length}
                              </span>
                            )}
                            {lr && (
                              <span className="text-[10px] text-slate-400">{relativeTime(lr.finishedAt)}</span>
                            )}
                          </Link>
                        )
                      })}
                      {serverChecks.length === 0 && (
                        <div className="px-5 py-4 text-center text-xs text-slate-400">No checks assigned to this server</div>
                      )}
                    </div>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      </div>

      {/* Bottom: Active Incidents + Recent Results */}
      <div className="grid gap-4 lg:grid-cols-2">
        {/* Active Incidents */}
        <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
          <div className="flex items-center justify-between border-b border-slate-100 px-5 py-3.5 dark:border-slate-800">
            <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">
              Active Incidents
              {openCount > 0 && (
                <span className="ml-2 inline-flex h-5 min-w-[1.25rem] items-center justify-center rounded-full bg-red-100 px-1.5 text-[10px] font-bold text-red-700 dark:bg-red-950/50 dark:text-red-400">
                  {openCount}
                </span>
              )}
            </h2>
            <Link to="/incidents" className="inline-flex items-center gap-1 text-xs font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400">
              View all <ArrowRight className="h-3 w-3" />
            </Link>
          </div>
          <div className="divide-y divide-slate-100 dark:divide-slate-800">
            {incidents && incidents.items.length > 0 ? (
              incidents.items.slice(0, 5).map(inc => (
                <Link
                  key={inc.id}
                  to={`/incidents/${inc.id}`}
                  className="flex items-center gap-3 px-5 py-3 transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50"
                >
                  <StatusBadge status={inc.severity} label={false} size="md" />
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-slate-900 dark:text-slate-100">{inc.checkName}</p>
                    <p className="truncate text-xs text-slate-500">{inc.message}</p>
                  </div>
                  <span className="shrink-0 text-xs text-slate-400">{relativeTime(inc.startedAt)}</span>
                </Link>
              ))
            ) : (
              <div className="px-5 py-8 text-center text-sm text-slate-400">No active incidents</div>
            )}
          </div>
        </div>

        {/* Latest Check Results */}
        <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
          <div className="flex items-center justify-between border-b border-slate-100 px-5 py-3.5 dark:border-slate-800">
            <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Latest Results</h2>
            <Link to="/checks" className="inline-flex items-center gap-1 text-xs font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400">
              All checks <ArrowRight className="h-3 w-3" />
            </Link>
          </div>
          <div className="divide-y divide-slate-100 dark:divide-slate-800">
            {summary.latest && summary.latest.length > 0 ? (
              summary.latest.map(result => (
                <Link
                  key={result.id}
                  to={`/checks/${result.checkId}`}
                  className="flex items-center gap-3 px-5 py-2.5 transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50"
                >
                  <StatusBadge status={result.status} label={false} />
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm text-slate-700 dark:text-slate-300">{result.name}</p>
                  </div>
                  <span className="rounded bg-slate-100 px-1.5 py-0.5 font-mono text-[10px] dark:bg-slate-800">{checkTypeLabel(result.type)}</span>
                  <span className="flex items-center gap-1 text-xs text-slate-400">
                    <Clock className="h-3 w-3" />
                    {formatDuration(result.durationMs)}
                  </span>
                </Link>
              ))
            ) : (
              <div className="px-5 py-8 text-center text-sm text-slate-400">No results yet</div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

/* ---- Helper components ---- */

function MiniMetric({ label, value, sub, warn, crit }: { label: string; value: string; sub?: string; warn?: boolean; crit?: boolean }) {
  return (
    <div>
      <p className="text-[10px] font-medium uppercase tracking-wider text-slate-400">{label}</p>
      <p className={cn(
        'text-base font-bold tabular-nums text-slate-900 dark:text-slate-100',
        crit && 'text-red-600 dark:text-red-400',
        warn && !crit && 'text-amber-600 dark:text-amber-400',
      )}>
        {value}
      </p>
      {sub && <p className="text-[10px] text-slate-400 tabular-nums">{sub}</p>}
    </div>
  )
}
