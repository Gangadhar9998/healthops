import { useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { mysqlApi } from '@/api/mysql'
import { DetailPageLayout } from '@/components/db/DetailPageLayout'
import { UtilizationBar } from '@/components/db/UtilizationBar'
import { BreakdownCard } from '@/components/db/BreakdownCard'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { cn } from '@/lib/utils'
import { REFETCH_INTERVAL } from '@/lib/constants'
import type { MySQLProcess, MySQLUserStat, MySQLHostStat } from '@/types'

export default function MySQLConnections() {
  const { data: health, isLoading, error, refetch } = useQuery({
    queryKey: ['mysql', 'health'],
    queryFn: mysqlApi.health,
    refetchInterval: REFETCH_INTERVAL,
  })

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

  const processList: MySQLProcess[] = health.processList || []
  const userStats: MySQLUserStat[] = health.userStats || []
  const hostStats: MySQLHostStat[] = health.hostStats || []
  const activeProcesses = processList.filter(p => p.command !== 'Sleep' && p.command !== 'Daemon')
  const longRunning = processList.filter(p => p.time > 5 && p.command !== 'Daemon')

  return (
    <DetailPageLayout backTo="/mysql" backLabel="Back to MySQL" title="Connections" subtitle={`${health.connections} of ${health.maxConnections} connections used`}>
      {/* Utilization + summary */}
      <div className="grid gap-4 lg:grid-cols-3">
        <UtilizationBar label="Connection Utilization" value={health.connections} max={health.maxConnections} />
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

      {/* Full process list */}
      <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
        <div className="border-b border-slate-100 px-5 py-3.5 dark:border-slate-800 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">All Connections ({processList.length})</h2>
          <div className="flex items-center gap-2">
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
        {processList.length > 0 ? (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-100 bg-slate-50/50 dark:border-slate-800 dark:bg-slate-800/30">
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">ID</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">User</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">Host</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">DB</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">Command</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">Time</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">State</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">Query</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {processList.map((p) => (
                  <tr key={p.id} className={cn(
                    p.command !== 'Sleep' && p.command !== 'Daemon' && 'bg-blue-50/50 dark:bg-blue-900/10',
                    p.time > 10 && 'bg-red-50/50 dark:bg-red-900/10',
                    p.time > 5 && p.time <= 10 && 'bg-amber-50/50 dark:bg-amber-900/10',
                  )}>
                    <td className="px-4 py-2.5 font-mono text-xs text-slate-600 dark:text-slate-400">{p.id}</td>
                    <td className="px-4 py-2.5 text-xs font-medium text-slate-900 dark:text-slate-100">{p.user}</td>
                    <td className="px-4 py-2.5 text-xs text-slate-500">{p.host}</td>
                    <td className="px-4 py-2.5 text-xs text-slate-500">{p.db || '—'}</td>
                    <td className="px-4 py-2.5">
                      <span className={cn(
                        'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
                        p.command === 'Query' && 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
                        p.command === 'Sleep' && 'bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400',
                        p.command === 'Connect' && 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
                        p.command !== 'Query' && p.command !== 'Sleep' && p.command !== 'Connect' && 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-400',
                      )}>
                        {p.command}
                      </span>
                    </td>
                    <td className={cn('px-4 py-2.5 font-mono text-xs', p.time > 10 ? 'text-red-600 font-semibold' : p.time > 5 ? 'text-amber-600 font-semibold' : 'text-slate-500')}>{p.time}s</td>
                    <td className="px-4 py-2.5 text-xs text-slate-500">{p.state || '—'}</td>
                    <td className="px-4 py-2.5 max-w-xs">
                      {p.info ? (
                        <code className="block truncate rounded bg-slate-50 px-2 py-1 text-xs text-slate-700 dark:bg-slate-800 dark:text-slate-300" title={p.info}>{p.info}</code>
                      ) : <span className="text-xs text-slate-400">—</span>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="p-8 text-center text-sm text-slate-400">No active connections</div>
        )}
      </div>
    </DetailPageLayout>
  )
}
