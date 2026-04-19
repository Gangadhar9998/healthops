import { useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { mysqlApi } from '@/api/mysql'
import { DetailPageLayout } from '@/components/db/DetailPageLayout'
import { ServerStatsCard } from '@/components/db/ServerStatsCard'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { cn, relativeTime } from '@/lib/utils'
import { REFETCH_INTERVAL } from '@/lib/constants'
import { Server, Zap, Clock, Activity, Lock, Shield, HardDrive, Database, AlertTriangle } from 'lucide-react'

export default function MySQLServer() {
  const { data: health, isLoading, error, refetch } = useQuery({
    queryKey: ['mysql', 'health'],
    queryFn: mysqlApi.health,
    refetchInterval: REFETCH_INTERVAL,
  })

  const { data: deltas } = useQuery({
    queryKey: ['mysql', 'deltas'],
    queryFn: () => mysqlApi.deltas({ limit: 30 }),
    refetchInterval: REFETCH_INTERVAL,
  })

  const [searchParams] = useSearchParams()
  const section = searchParams.get('section')

  useEffect(() => {
    if (section) {
      const el = document.getElementById(`section-${section}`)
      if (el) setTimeout(() => el.scrollIntoView({ behavior: 'smooth', block: 'center' }), 150)
    }
  }, [section])

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message="Failed to load server stats" retry={() => refetch()} />
  if (!health) return null

  return (
    <DetailPageLayout backTo="/mysql" backLabel="Back to MySQL" title="Server Overview" subtitle={`Uptime: ${formatUptime(health.uptime)} · Status: ${health.status}`}>
      {/* Stats */}
      <div className="grid gap-4 lg:grid-cols-2">
        <ServerStatsCard
          title="Performance"
          stats={[
            { icon: <Server className="h-4 w-4" />, label: 'Uptime', value: formatUptime(health.uptime) },
            { icon: <Zap className="h-4 w-4" />, label: 'Total Queries', value: formatNumber(health.questions) },
            { icon: <Zap className="h-4 w-4" />, label: 'Queries/sec', value: health.queriesPerSec.toFixed(1) },
            { icon: <Activity className="h-4 w-4" />, label: 'Threads Running', value: String(health.threadsRunning), warn: health.threadsRunning > 10 },
          ]}
        />
        <ServerStatsCard
          title="Issues & Locks"
          stats={[
            { icon: <Clock className="h-4 w-4" />, label: 'Slow Queries', value: String(health.totalSlowQueries), warn: health.totalSlowQueries > 0 },
            { icon: <Clock className="h-4 w-4" />, label: 'Slow/sec', value: health.slowQueries > 0 ? health.slowQueries.toFixed(3) : '0', warn: health.slowQueries > 0 },
            { icon: <Lock className="h-4 w-4" />, label: 'Row Lock Waits', value: String(health.innodbRowLockWaits), warn: health.innodbRowLockWaits > 0 },
            { icon: <Lock className="h-4 w-4" />, label: 'Lock Wait Time', value: `${health.innodbRowLockTime}ms` },
            { icon: <Shield className="h-4 w-4" />, label: 'Aborted Connects', value: String(health.abortedConnects), warn: health.abortedConnects > 0 },
            { icon: <Shield className="h-4 w-4" />, label: 'Aborted Clients', value: String(health.abortedClients), warn: health.abortedClients > 0 },
          ]}
        />
      </div>

      {/* Danger Indicators */}
      <div className="grid gap-4 lg:grid-cols-2">
        <ServerStatsCard
          id="section-query-efficiency"
          highlighted={section === 'query-efficiency'}
          title="Query Efficiency"
          stats={[
            { icon: <AlertTriangle className="h-4 w-4" />, label: 'Full Table Scans', value: formatNumber(health.selectScan ?? 0), warn: (health.selectScan ?? 0) > 1000 },
            { icon: <AlertTriangle className="h-4 w-4" />, label: 'Full Joins (no index)', value: formatNumber(health.selectFullJoin ?? 0), warn: (health.selectFullJoin ?? 0) > 0 },
            { icon: <AlertTriangle className="h-4 w-4" />, label: 'Sort Merge Passes', value: formatNumber(health.sortMergePasses ?? 0), warn: (health.sortMergePasses ?? 0) > 10 },
            { icon: <Database className="h-4 w-4" />, label: 'Buffer Pool Hit Rate', value: `${(health.bufferPoolHitRate ?? 0).toFixed(2)}%`, warn: (health.bufferPoolHitRate ?? 100) < 99 },
          ]}
        />
        <ServerStatsCard
          id="section-resources"
          highlighted={section === 'resources'}
          title="Resources"
          stats={[
            { icon: <Lock className="h-4 w-4" />, label: 'Table Lock Waits', value: formatNumber(health.tableLocksWaited ?? 0), warn: (health.tableLocksWaited ?? 0) > 0 },
            { icon: <Lock className="h-4 w-4" />, label: 'Table Locks (imm)', value: formatNumber(health.tableLocksImmediate ?? 0) },
            { icon: <HardDrive className="h-4 w-4" />, label: 'Open Files', value: `${health.openFiles ?? 0}/${health.openFilesLimit ?? 0}` },
            { icon: <HardDrive className="h-4 w-4" />, label: 'Open Tables', value: `${health.openTables ?? 0}/${health.tableOpenCache ?? 0}`, warn: (health.tableOpenCache ?? 0) > 0 && (health.openTables ?? 0) / (health.tableOpenCache ?? 1) > 0.9 },
            { icon: <Database className="h-4 w-4" />, label: 'Tables Opened', value: formatNumber(health.openedTables ?? 0), warn: (health.openedTables ?? 0) > 10000 },
            { icon: <Shield className="h-4 w-4" />, label: 'Connections Refused', value: formatNumber(health.connectionsRefused ?? 0), warn: (health.connectionsRefused ?? 0) > 0 },
          ]}
        />
      </div>

      {/* Full delta history */}
      {deltas && Array.isArray(deltas) && deltas.length > 0 && (
        <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
          <div className="border-b border-slate-100 px-5 py-3.5 dark:border-slate-800">
            <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Recent Activity ({(deltas as Record<string, unknown>[]).length} samples)</h2>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-100 bg-slate-50/50 dark:border-slate-800 dark:bg-slate-800/30">
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">Time</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-slate-500">Queries/s</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-slate-500">Slow/s</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-slate-500">Lock Waits/s</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-slate-500">Threads Created</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-slate-500">Aborted/s</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {(deltas as Record<string, unknown>[]).map((d, i) => (
                  <tr key={i}>
                    <td className="px-4 py-2.5 text-xs text-slate-500">{d.timestamp ? relativeTime(String(d.timestamp)) : '—'}</td>
                    <td className="px-4 py-2.5 text-right font-mono text-xs">{formatRate(d.questionsPerSec)}</td>
                    <td className={cn('px-4 py-2.5 text-right font-mono text-xs', Number(d.slowQueriesPerSec || 0) > 0 && 'text-amber-600 font-semibold')}>{formatRate(d.slowQueriesPerSec)}</td>
                    <td className={cn('px-4 py-2.5 text-right font-mono text-xs', Number(d.rowLockWaitsPerSec || 0) > 0 && 'text-amber-600')}>{formatRate(d.rowLockWaitsPerSec)}</td>
                    <td className="px-4 py-2.5 text-right font-mono text-xs">{d.threadsCreatedDelta != null ? String(d.threadsCreatedDelta) : '0'}</td>
                    <td className={cn('px-4 py-2.5 text-right font-mono text-xs', Number(d.abortedConnectsPerSec || 0) > 0 && 'text-red-600')}>{formatRate(d.abortedConnectsPerSec)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </DetailPageLayout>
  )
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const mins = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}d ${hours}h ${mins}m`
  if (hours > 0) return `${hours}h ${mins}m`
  return `${mins}m`
}

function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

function formatRate(val: unknown): string {
  const n = Number(val || 0)
  if (n === 0) return '0'
  if (n < 0.01) return n.toFixed(4)
  if (n < 1) return n.toFixed(2)
  return n.toFixed(1)
}
