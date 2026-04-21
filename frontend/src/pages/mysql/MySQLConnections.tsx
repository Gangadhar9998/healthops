import { useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { mysqlApi } from '@/api/mysql'
import { DetailPageLayout } from '@/components/db/DetailPageLayout'
import { UtilizationBar } from '@/components/db/UtilizationBar'
import { BreakdownCard } from '@/components/db/BreakdownCard'
import { LiveProcessList } from '@/components/db/LiveProcessList'
import { LiveIndicator } from '@/components/db/LiveIndicator'
import { Sparkline } from '@/components/charts/Sparkline'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { cn } from '@/lib/utils'
import { REFETCH_INTERVAL } from '@/lib/constants'
import { useMySQLLive } from '@/hooks/useMySQLLive'
import type { MySQLUserStat, MySQLHostStat } from '@/types'

export default function MySQLConnections() {
  const { data: health, isLoading, error, refetch } = useQuery({
    queryKey: ['mysql', 'health'],
    queryFn: mysqlApi.health,
    refetchInterval: REFETCH_INTERVAL,
  })

  const { snapshot: live, history, connected: liveConnected } = useMySQLLive(!isLoading && !error)

  const [searchParams] = useSearchParams()
  const highlightRefused = searchParams.get('highlight') === 'refused'

  useEffect(() => {
    if (highlightRefused) {
      const el = document.getElementById('stat-connections-refused')
      if (el) setTimeout(() => el.scrollIntoView({ behavior: 'smooth', block: 'center' }), 150)
    }
  }, [highlightRefused])

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message="Failed to load MySQL connections" retry={() => refetch()} />
  if (!health) return null

  const processList = live?.processList ?? health.processList ?? []
  const longRunning = live?.longRunning ?? processList.filter(p => p.time > 5 && p.command !== 'Daemon')
  const userStats: MySQLUserStat[] = health.userStats || []
  const hostStats: MySQLHostStat[] = health.hostStats || []
  const activeProcesses = processList.filter(p => p.command !== 'Sleep' && p.command !== 'Daemon')
  const connHistory = history.map(s => s.connections)

  return (
    <DetailPageLayout backTo="/mysql" backLabel="Back to MySQL" title="Connections" subtitle={`${live?.connections ?? health.connections} of ${health.maxConnections} connections used`}>
      {/* Utilization + summary */}
      <div className="grid gap-4 lg:grid-cols-3">
        <div className="space-y-2">
          <UtilizationBar label="Connection Utilization" value={live?.connections ?? health.connections} max={health.maxConnections} />
          {connHistory.length > 3 && (
            <div className="rounded-xl border border-slate-200 bg-white p-3 dark:border-slate-800 dark:bg-slate-900">
              <div className="flex items-center justify-between mb-1">
                <span className="text-xs font-medium text-slate-500">Connection History</span>
                <LiveIndicator connected={liveConnected} />
              </div>
              <Sparkline data={connHistory} color="#6366f1" height={40} />
            </div>
          )}
        </div>
        <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
          <h2 className="mb-3 text-sm font-semibold text-slate-900 dark:text-slate-100">Connection Summary</h2>
          <div className="grid grid-cols-2 gap-3 text-sm">
            <div><span className="text-slate-500">Current</span><p className="font-semibold text-slate-900 dark:text-slate-100">{health.connections}</p></div>
            <div><span className="text-slate-500">Peak</span><p className="font-semibold text-slate-900 dark:text-slate-100">{health.maxUsedConnections}</p></div>
            <div><span className="text-slate-500">Aborted Connects</span><p className={cn('font-semibold', health.abortedConnects > 0 ? 'text-red-600' : 'text-slate-900 dark:text-slate-100')}>{health.abortedConnects}</p></div>
            <div><span className="text-slate-500">Aborted Clients</span><p className={cn('font-semibold', health.abortedClients > 0 ? 'text-amber-600' : 'text-slate-900 dark:text-slate-100')}>{health.abortedClients}</p></div>
            <div id="stat-connections-refused" className={cn('col-span-2 rounded-lg p-2 -m-2 transition-all', highlightRefused && 'ring-2 ring-blue-400/50 bg-blue-50/50 dark:bg-blue-900/20')}><span className="text-slate-500">Connections Refused</span><p className={cn('font-semibold', (health.connectionsRefused ?? 0) > 0 ? 'text-red-600' : 'text-slate-900 dark:text-slate-100')}>{health.connectionsRefused ?? 0}</p></div>
          </div>
        </div>
        <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
          <h2 className="mb-3 text-sm font-semibold text-slate-900 dark:text-slate-100">Thread Stats</h2>
          <div className="grid grid-cols-2 gap-3 text-sm">
            <div><span className="text-slate-500">Total Processes</span><p className="font-semibold text-slate-900 dark:text-slate-100">{processList.length}</p></div>
            <div><span className="text-slate-500">Max Used</span><p className="font-semibold text-slate-900 dark:text-slate-100">{health.maxUsedConnections}</p></div>
            <div><span className="text-slate-500">Active Queries</span><p className="font-semibold text-blue-600">{activeProcesses.length}</p></div>
            <div><span className="text-slate-500">Long Running</span><p className={cn('font-semibold', longRunning.length > 0 ? 'text-amber-600' : 'text-slate-900 dark:text-slate-100')}>{longRunning.length}</p></div>
          </div>
        </div>
      </div>

      {/* User + Host breakdown */}
      <div className="grid gap-4 lg:grid-cols-2">
        <BreakdownCard
          title="Connections by User"
          items={userStats.map(u => ({ label: u.user, value: u.currentConnections, total: u.totalConnections, maxValue: health.connections }))}
          emptyMessage="No user stats available"
        />
        <BreakdownCard
          title="Connections by Host"
          items={hostStats.map(h => ({ label: h.host, value: h.currentConnections, total: h.totalConnections, maxValue: health.connections }))}
          barColor={(v) => v > 20 ? 'bg-red-500' : v > 5 ? 'bg-amber-500' : 'bg-indigo-500'}
          emptyMessage="No host stats available"
          mono
        />
      </div>

      {/* Full process list with kill buttons */}
      <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
        <div className="border-b border-slate-100 px-5 py-3.5 dark:border-slate-800 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">All Connections ({processList.length})</h2>
          <div className="flex items-center gap-2">
            <LiveIndicator connected={liveConnected} />
            {longRunning.length > 0 && (
              <span className="rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-900/30 dark:text-amber-400">
                {longRunning.length} long-running ({'>'}5s)
              </span>
            )}
            {activeProcesses.length > 0 && (
              <span className="rounded-full bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-700 dark:bg-blue-900/30 dark:text-blue-400">
                {activeProcesses.length} active
              </span>
            )}
          </div>
        </div>
        <div className="p-4">
          <LiveProcessList processes={processList} longRunning={longRunning} />
        </div>
      </div>
    </DetailPageLayout>
  )
}
