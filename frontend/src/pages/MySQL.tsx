import { useQuery } from '@tanstack/react-query'
import { Database, Users, Gauge, Clock, Server, Shield, Activity, AlertTriangle } from 'lucide-react'
import { Link } from 'react-router-dom'
import { mysqlApi } from '@/api/mysql'
import { ClickableMetricCard } from '@/components/db/ClickableMetricCard'
import { UtilizationBar } from '@/components/db/UtilizationBar'
import { BreakdownCard } from '@/components/db/BreakdownCard'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { EmptyState } from '@/components/EmptyState'
import { ExportButton } from '@/components/ExportButton'
import { MySQLAIPanel } from '@/components/db/MySQLAIPanel'
import { relativeTime } from '@/lib/utils'
import { settingsApi } from '@/api/settings'
import { REFETCH_INTERVAL } from '@/lib/constants'

export default function MySQL() {
  const { data: health, isLoading, error, refetch } = useQuery({
    queryKey: ['mysql', 'health'],
    queryFn: mysqlApi.health,
    refetchInterval: REFETCH_INTERVAL,
    retry: 1,
  })

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message="MySQL monitoring not available. Ensure a mysql check is configured." retry={() => refetch()} />
  if (!health) return <EmptyState title="No MySQL data" description="Configure a MySQL check to enable monitoring." icon={<Database className="h-6 w-6" />} />

  const connPct = health.connectionUtilPct
  const statusMap: Record<string, 'healthy' | 'warning' | 'critical' | 'neutral'> = {
    healthy: 'healthy', warning: 'warning', critical: 'critical',
  }
  const userStats = health.userStats || []
  const hostStats = health.hostStats || []
  const topQueries = health.topQueries || []

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">MySQL Monitoring</h1>
          <p className="text-sm text-slate-500">
            {health.lastSampleAt ? `Last sample ${relativeTime(health.lastSampleAt)}` : 'No samples yet'}
            {' · Click any card for details'}
          </p>
        </div>
        <ExportButton downloadUrl={settingsApi.exportMysqlSamples('csv')} />
      </div>

      {/* Clickable metric cards */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-5">
        <ClickableMetricCard
          to="/mysql/server"
          label="Status"
          value={health.status.charAt(0).toUpperCase() + health.status.slice(1)}
          subValue={`Uptime: ${formatUptime(health.uptime)}`}
          icon={<Database className="h-5 w-5" />}
          status={statusMap[health.status] || 'neutral'}
        />
        <ClickableMetricCard
          to="/mysql/connections"
          label="Connections"
          value={`${health.connections}/${health.maxConnections}`}
          subValue={`${connPct.toFixed(1)}% utilized`}
          icon={<Users className="h-5 w-5" />}
          status={connPct > 80 ? 'critical' : connPct > 60 ? 'warning' : 'healthy'}
        />
        <ClickableMetricCard
          to="/mysql/queries"
          label="Queries/sec"
          value={health.queriesPerSec.toFixed(1)}
          subValue={`${formatNumber(health.questions)} total`}
          icon={<Gauge className="h-5 w-5" />}
        />
        <ClickableMetricCard
          to="/mysql/queries?filter=slow"
          label="Slow Queries"
          value={health.totalSlowQueries}
          subValue={health.slowQueries > 0 ? `${health.slowQueries.toFixed(2)}/sec` : 'None recent'}
          icon={<Clock className="h-5 w-5" />}
          status={health.totalSlowQueries > 0 ? 'warning' : 'neutral'}
        />
        <ClickableMetricCard
          to="/mysql/threads"
          label="Active Threads"
          value={(health.processList || []).filter((p: { command: string }) => p.command !== 'Sleep' && p.command !== 'Daemon').length}
          subValue={`${(health.processList || []).length} total processes`}
          icon={<Activity className="h-5 w-5" />}
          status={(health.processList || []).filter((p: { command: string }) => p.command !== 'Sleep' && p.command !== 'Daemon').length > 10 ? 'warning' : 'neutral'}
        />
      </div>

      {/* Connection utilization bar + quick preview */}
      <div className="grid gap-4 lg:grid-cols-3">
        <UtilizationBar label="Connection Utilization" value={health.connections} max={health.maxConnections} />

        {/* Quick user breakdown preview (top 3) */}
        <BreakdownCard
          title="Top Users"
          items={userStats.slice(0, 3).map(u => ({ label: u.user, value: u.currentConnections, total: u.totalConnections, maxValue: health.connections }))}
          emptyMessage="No user stats"
        />

        {/* Quick host breakdown preview (top 3) */}
        <BreakdownCard
          title="Top Hosts"
          items={hostStats.slice(0, 3).map(h => ({ label: h.host, value: h.currentConnections, total: h.totalConnections, maxValue: health.connections }))}
          barColor={(v) => v > 20 ? 'bg-red-500' : v > 5 ? 'bg-amber-500' : 'bg-indigo-500'}
          emptyMessage="No host stats"
          mono
        />
      </div>

      {/* Quick top queries preview (top 5) */}
      {topQueries.length > 0 && (
        <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
          <div className="border-b border-slate-100 px-5 py-3.5 dark:border-slate-800 flex items-center justify-between">
            <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">
              Top Queries (preview)
            </h2>
            <a href="/mysql/queries" className="text-xs font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400">
              View all →
            </a>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-100 bg-slate-50/50 dark:border-slate-800 dark:bg-slate-800/30">
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">Query</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-slate-500">Calls</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-slate-500">Avg (s)</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {topQueries.slice(0, 5).map((q, i) => (
                  <tr key={i}>
                    <td className="px-4 py-2.5 max-w-md">
                      <code className="block truncate rounded bg-slate-50 px-2 py-1 text-xs text-slate-700 dark:bg-slate-800 dark:text-slate-300" title={q.digestText}>
                        {q.digestText}
                      </code>
                    </td>
                    <td className="px-4 py-2.5 text-right font-mono text-xs">{formatNumber(q.countStar)}</td>
                    <td className="px-4 py-2.5 text-right font-mono text-xs">{q.avgTimerWait.toFixed(4)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Danger Signals - things that need attention */}
      <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
        <div className="flex items-center gap-2 mb-4">
          <AlertTriangle className="h-4 w-4 text-amber-500" />
          <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Danger Signals</h2>
        </div>
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4 text-sm">
          <DangerStat
            to="/mysql/server?section=query-efficiency"
            label="Buffer Pool Hit Rate"
            value={`${(health.bufferPoolHitRate ?? 0).toFixed(2)}%`}
            severity={health.bufferPoolHitRate != null ? (health.bufferPoolHitRate < 95 ? 'critical' : health.bufferPoolHitRate < 99 ? 'warning' : 'ok') : 'ok'}
            hint={health.bufferPoolHitRate != null && health.bufferPoolHitRate < 99 ? 'Reads going to disk instead of buffer' : 'Good — mostly served from memory'}
          />
          <DangerStat
            to="/mysql/queries?highlight=full-scans"
            label="Full Table Scans"
            value={formatNumber(health.selectScan ?? 0)}
            severity={(health.selectScan ?? 0) > 10000 ? 'critical' : (health.selectScan ?? 0) > 1000 ? 'warning' : 'ok'}
            hint="SELECT operations doing full scans"
          />
          <DangerStat
            to="/mysql/queries?highlight=full-joins"
            label="Full Joins (no index)"
            value={formatNumber(health.selectFullJoin ?? 0)}
            severity={(health.selectFullJoin ?? 0) > 0 ? 'critical' : 'ok'}
            hint="JOINs with no index — very expensive"
          />
          <DangerStat
            to="/mysql/queries?highlight=sort-merge"
            label="Sort Merge Passes"
            value={formatNumber(health.sortMergePasses ?? 0)}
            severity={(health.sortMergePasses ?? 0) > 100 ? 'critical' : (health.sortMergePasses ?? 0) > 10 ? 'warning' : 'ok'}
            hint="Sorts spilling to disk"
          />
          <DangerStat
            to="/mysql/server?section=resources"
            label="Table Lock Waits"
            value={formatNumber(health.tableLocksWaited ?? 0)}
            severity={(health.tableLocksWaited ?? 0) > 100 ? 'critical' : (health.tableLocksWaited ?? 0) > 0 ? 'warning' : 'ok'}
            hint={`${formatNumber(health.tableLocksImmediate ?? 0)} immediate locks`}
          />
          <DangerStat
            to="/mysql/connections?highlight=refused"
            label="Connections Refused"
            value={formatNumber(health.connectionsRefused ?? 0)}
            severity={(health.connectionsRefused ?? 0) > 0 ? 'critical' : 'ok'}
            hint="Rejected due to max_connections"
          />
          <DangerStat
            to="/mysql/server?section=resources"
            label="Open Files"
            value={`${health.openFiles ?? 0}/${health.openFilesLimit ?? 0}`}
            severity={(health.openFilesLimit ?? 0) > 0 && (health.openFiles ?? 0) / (health.openFilesLimit ?? 1) > 0.9 ? 'critical' : (health.openFilesLimit ?? 0) > 0 && (health.openFiles ?? 0) / (health.openFilesLimit ?? 1) > 0.7 ? 'warning' : 'ok'}
            hint="Open files vs system limit"
          />
          <DangerStat
            to="/mysql/server?section=resources"
            label="Table Cache"
            value={`${health.openTables ?? 0}/${health.tableOpenCache ?? 0}`}
            severity={(health.tableOpenCache ?? 0) > 0 && (health.openTables ?? 0) / (health.tableOpenCache ?? 1) > 0.9 ? 'warning' : 'ok'}
            hint={`${formatNumber(health.openedTables ?? 0)} tables opened since start`}
          />
        </div>
      </div>

      {/* Inefficient Queries Preview */}
      {(() => {
        const inefficient = topQueries
          .filter(q => q.sumRowsSent > 0 && q.sumRowsExam / q.sumRowsSent > 100)
          .sort((a, b) => (b.sumRowsExam / b.sumRowsSent) - (a.sumRowsExam / a.sumRowsSent))
          .slice(0, 3)
        const errQueries = topQueries.filter(q => (q.sumErrors ?? 0) > 0).slice(0, 3)
        if (inefficient.length === 0 && errQueries.length === 0) return null
        return (
          <div className="rounded-xl border border-red-200 bg-red-50/30 dark:border-red-900/50 dark:bg-red-950/20">
            <div className="border-b border-red-100 px-5 py-3.5 dark:border-red-900/40 flex items-center justify-between">
              <div className="flex items-center gap-2">
                <AlertTriangle className="h-4 w-4 text-red-500" />
                <h2 className="text-sm font-semibold text-red-900 dark:text-red-200">Queries Needing Attention</h2>
              </div>
              <a href="/mysql/queries?filter=inefficient" className="text-xs font-medium text-red-600 hover:text-red-700 dark:text-red-400">
                View all →
              </a>
            </div>
            <div className="divide-y divide-red-100 dark:divide-red-900/30">
              {inefficient.map((q, i) => {
                const ratio = q.sumRowsSent > 0 ? Math.round(q.sumRowsExam / q.sumRowsSent) : 0
                return (
                  <div key={`ineff-${i}`} className="px-5 py-3 flex items-start gap-3">
                    <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded bg-amber-100 text-amber-700 text-xs font-bold dark:bg-amber-900/50 dark:text-amber-300">!</span>
                    <div className="min-w-0 flex-1">
                      <code className="block truncate text-xs text-slate-700 dark:text-slate-300">{q.digestText}</code>
                      <p className="mt-0.5 text-xs text-red-600 dark:text-red-400">
                        Examines <strong>{formatNumber(q.sumRowsExam)}</strong> rows to return <strong>{formatNumber(q.sumRowsSent)}</strong> ({ratio}:1 ratio) — needs index optimization
                      </p>
                    </div>
                  </div>
                )
              })}
              {errQueries.map((q, i) => (
                <div key={`err-${i}`} className="px-5 py-3 flex items-start gap-3">
                  <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded bg-red-100 text-red-700 text-xs font-bold dark:bg-red-900/50 dark:text-red-300">✕</span>
                  <div className="min-w-0 flex-1">
                    <code className="block truncate text-xs text-slate-700 dark:text-slate-300">{q.digestText}</code>
                    <p className="mt-0.5 text-xs text-red-600 dark:text-red-400">
                      <strong>{formatNumber(q.sumErrors ?? 0)}</strong> errors · <strong>{formatNumber(q.sumWarnings ?? 0)}</strong> warnings across {formatNumber(q.countStar)} executions
                    </p>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )
      })()}

      {/* Server quick stats */}
      <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Server Quick Stats</h2>
          <a href="/mysql/server" className="text-xs font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400">
            Full details →
          </a>
        </div>
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4 text-sm">
          <QuickStat icon={<Server className="h-4 w-4" />} label="Uptime" value={formatUptime(health.uptime)} />
          <QuickStat icon={<Gauge className="h-4 w-4" />} label="Queries/sec" value={health.queriesPerSec.toFixed(1)} />
          <QuickStat icon={<Shield className="h-4 w-4" />} label="Lock Waits" value={String(health.innodbRowLockWaits)} warn={health.innodbRowLockWaits > 0} />
          <QuickStat icon={<Clock className="h-4 w-4" />} label="Slow Queries" value={String(health.totalSlowQueries)} warn={health.totalSlowQueries > 0} />
        </div>
      </div>

      {/* AI Assistant */}
      <MySQLAIPanel />
    </div>
  )
}

function QuickStat({ icon, label, value, warn }: { icon: React.ReactNode; label: string; value: string; warn?: boolean }) {
  return (
    <div className="flex items-center gap-3">
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400">{icon}</div>
      <div>
        <p className="text-xs text-slate-500 dark:text-slate-400">{label}</p>
        <p className={`text-sm font-semibold ${warn ? 'text-amber-600' : 'text-slate-900 dark:text-slate-100'}`}>{value}</p>
      </div>
    </div>
  )
}

function DangerStat({ label, value, severity, hint, to }: { label: string; value: string; severity: 'ok' | 'warning' | 'critical'; hint?: string; to: string }) {
  const colorMap = {
    ok: 'text-emerald-600 dark:text-emerald-400',
    warning: 'text-amber-600 dark:text-amber-400',
    critical: 'text-red-600 dark:text-red-400',
  }
  const bgMap = {
    ok: 'bg-emerald-50 dark:bg-emerald-900/20',
    warning: 'bg-amber-50 dark:bg-amber-900/20',
    critical: 'bg-red-50 dark:bg-red-900/20',
  }
  const dotMap = {
    ok: 'bg-emerald-500',
    warning: 'bg-amber-500',
    critical: 'bg-red-500',
  }
  return (
    <Link to={to} className={`block rounded-lg p-3 transition-all hover:ring-2 hover:ring-slate-300 dark:hover:ring-slate-600 hover:shadow-sm cursor-pointer ${bgMap[severity]}`}>
      <div className="flex items-center gap-1.5">
        <span className={`inline-block h-1.5 w-1.5 rounded-full ${dotMap[severity]}`} />
        <p className="text-xs font-medium text-slate-600 dark:text-slate-400">{label}</p>
      </div>
      <p className={`mt-1 text-lg font-bold ${colorMap[severity]}`}>{value}</p>
      {hint && <p className="mt-0.5 text-[11px] text-slate-400 dark:text-slate-500">{hint}</p>}
    </Link>
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
