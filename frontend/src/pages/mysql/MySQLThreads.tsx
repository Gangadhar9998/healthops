import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Skull, Loader2, CheckCircle } from 'lucide-react'
import { mysqlApi } from '@/api/mysql'
import { DetailPageLayout } from '@/components/db/DetailPageLayout'
import { LiveIndicator } from '@/components/db/LiveIndicator'
import { Sparkline } from '@/components/charts/Sparkline'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { cn } from '@/lib/utils'
import { REFETCH_INTERVAL } from '@/lib/constants'
import { useMySQLLive } from '@/hooks/useMySQLLive'
import type { MySQLProcess } from '@/types'

export default function MySQLThreads() {
  const { data: health, isLoading, error, refetch } = useQuery({
    queryKey: ['mysql', 'health'],
    queryFn: mysqlApi.health,
    refetchInterval: REFETCH_INTERVAL,
  })

  const { snapshot: live, history, connected: liveConnected } = useMySQLLive(!isLoading && !error)
  const [killingId, setKillingId] = useState<number | null>(null)
  const [killedIds, setKilledIds] = useState<Set<number>>(new Set())

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message="Failed to load thread data" retry={() => refetch()} />
  if (!health) return null

  const processList: MySQLProcess[] = live?.processList ?? health.processList ?? []
  const activeThreads = processList.filter(p => p.command !== 'Sleep' && p.command !== 'Daemon')
  const sleepingThreads = processList.filter(p => p.command === 'Sleep')
  const longRunning = live?.longRunning ?? processList.filter(p => p.time > 5 && p.command !== 'Daemon')
  const threadsHistory = history.map(s => s.threadsRunning)

  const handleKill = async (processId: number) => {
    if (!confirm(`Kill query on process ${processId}?`)) return
    setKillingId(processId)
    try {
      await mysqlApi.killQuery(processId)
      setKilledIds(prev => new Set(prev).add(processId))
      setTimeout(() => setKilledIds(prev => { const n = new Set(prev); n.delete(processId); return n }), 5000)
    } catch { /* ignore */ }
    setKillingId(null)
  }

  const KillButton = ({ processId }: { processId: number }) => {
    if (killedIds.has(processId)) return <span className="inline-flex items-center gap-1 text-xs text-emerald-600"><CheckCircle className="h-3 w-3" /> Killed</span>
    return (
      <button onClick={() => handleKill(processId)} disabled={killingId === processId}
        className="inline-flex items-center gap-1 rounded bg-red-100 px-2 py-1 text-xs font-medium text-red-700 hover:bg-red-200 disabled:opacity-50 dark:bg-red-900 dark:text-red-300">
        {killingId === processId ? <><Loader2 className="h-3 w-3 animate-spin" /> Killing</> : <><Skull className="h-3 w-3" /> Kill</>}
      </button>
    )
  }

  return (
    <DetailPageLayout backTo="/mysql" backLabel="Back to MySQL" title="Threads & Processes" subtitle={`${activeThreads.length} active · ${sleepingThreads.length} sleeping · ${processList.length} total`}>
      {/* Long-running query alerts */}
      {longRunning.length > 0 && (
        <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 dark:border-red-900 dark:bg-red-950/40">
          <div className="flex items-center gap-2 text-sm font-medium text-red-700 dark:text-red-400">
            <span className="relative flex h-2.5 w-2.5">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-400 opacity-75" />
              <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-red-500" />
            </span>
            {longRunning.length} long-running {longRunning.length === 1 ? 'query' : 'queries'} detected (&gt;5s) — consider killing or optimizing
          </div>
        </div>
      )}

      {/* Summary + sparkline */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-5">
        <StatCard label="Total Processes" value={processList.length} />
        <StatCard label="Active" value={activeThreads.length} warn={activeThreads.length > 10} />
        <StatCard label="Sleeping" value={sleepingThreads.length} />
        <StatCard label="Long Running (>5s)" value={longRunning.length} warn={longRunning.length > 0} />
        <div className="rounded-xl border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
          <div className="flex items-center justify-between mb-1">
            <span className="text-xs font-medium text-slate-500">Threads Running</span>
            <LiveIndicator connected={liveConnected} />
          </div>
          {threadsHistory.length > 3 ? (
            <Sparkline data={threadsHistory} color="#f59e0b" height={36} />
          ) : (
            <p className="text-lg font-bold text-slate-900 dark:text-slate-100">{live?.threadsRunning ?? health.threadsRunning}</p>
          )}
        </div>
      </div>

      {/* Long-running queries first */}
      {longRunning.length > 0 && (
        <div className="rounded-xl border border-amber-200 bg-amber-50/50 dark:border-amber-900 dark:bg-amber-950/20">
          <div className="border-b border-amber-200 px-5 py-3.5 dark:border-amber-900">
            <h2 className="text-sm font-semibold text-amber-700 dark:text-amber-400">Long Running Queries ({longRunning.length})</h2>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-amber-200 dark:border-amber-900">
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-amber-600">ID</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-amber-600">User</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-amber-600">Host</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-amber-600">Time</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-amber-600">State</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-amber-600">Query</th>
                  <th className="px-4 py-2.5 text-center text-xs font-semibold uppercase text-amber-600">Action</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-amber-100 dark:divide-amber-900/50">
                {longRunning.map((p) => (
                  <tr key={p.id}>
                    <td className="px-4 py-2.5 font-mono text-xs">{p.id}</td>
                    <td className="px-4 py-2.5 text-xs font-medium">{p.user}</td>
                    <td className="px-4 py-2.5 text-xs">{p.host}</td>
                    <td className="px-4 py-2.5 font-mono text-xs text-red-600 font-semibold">{p.time}s</td>
                    <td className="px-4 py-2.5 text-xs">{p.state || '—'}</td>
                    <td className="px-4 py-2.5 max-w-md">
                      {p.info ? <code className="block whitespace-pre-wrap break-all rounded bg-amber-100 px-2 py-1 text-xs dark:bg-amber-900/30">{p.info}</code> : '—'}
                    </td>
                    <td className="px-4 py-2.5 text-center"><KillButton processId={p.id} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Active threads */}
      <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
        <div className="border-b border-slate-100 px-5 py-3.5 dark:border-slate-800">
          <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Active Threads ({activeThreads.length})</h2>
        </div>
        {activeThreads.length > 0 ? (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-100 bg-slate-50/50 dark:border-slate-800 dark:bg-slate-800/30">
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">ID</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">User</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">DB</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">Command</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">Time</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">State</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">Query</th>
                  <th className="px-4 py-2.5 text-center text-xs font-semibold uppercase text-slate-500">Action</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {activeThreads.map((p) => (
                  <tr key={p.id} className="bg-blue-50/50 dark:bg-blue-900/10">
                    <td className="px-4 py-2.5 font-mono text-xs">{p.id}</td>
                    <td className="px-4 py-2.5 text-xs font-medium text-slate-900 dark:text-slate-100">{p.user}</td>
                    <td className="px-4 py-2.5 text-xs text-slate-500">{p.db || '—'}</td>
                    <td className="px-4 py-2.5"><span className="rounded-full bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-700 dark:bg-blue-900/30 dark:text-blue-400">{p.command}</span></td>
                    <td className={cn('px-4 py-2.5 font-mono text-xs', p.time > 5 ? 'text-amber-600 font-semibold' : 'text-slate-500')}>{p.time}s</td>
                    <td className="px-4 py-2.5 text-xs text-slate-500">{p.state || '—'}</td>
                    <td className="px-4 py-2.5 max-w-sm">{p.info ? <code className="block truncate rounded bg-slate-50 px-2 py-1 text-xs dark:bg-slate-800" title={p.info}>{p.info}</code> : '—'}</td>
                    <td className="px-4 py-2.5 text-center"><KillButton processId={p.id} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="p-6 text-center text-sm text-slate-400">No active threads</div>
        )}
      </div>

      {/* Sleeping connections */}
      {sleepingThreads.length > 0 && (
        <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
          <div className="border-b border-slate-100 px-5 py-3.5 dark:border-slate-800">
            <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Sleeping Connections ({sleepingThreads.length})</h2>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-100 bg-slate-50/50 dark:border-slate-800 dark:bg-slate-800/30">
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">ID</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">User</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">Host</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">DB</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">Idle Time</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {sleepingThreads.map((p) => (
                  <tr key={p.id}>
                    <td className="px-4 py-2.5 font-mono text-xs text-slate-400">{p.id}</td>
                    <td className="px-4 py-2.5 text-xs text-slate-600 dark:text-slate-400">{p.user}</td>
                    <td className="px-4 py-2.5 text-xs text-slate-500">{p.host}</td>
                    <td className="px-4 py-2.5 text-xs text-slate-500">{p.db || '—'}</td>
                    <td className={cn('px-4 py-2.5 font-mono text-xs', p.time > 300 ? 'text-amber-600' : 'text-slate-400')}>{p.time}s</td>
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

function StatCard({ label, value, warn }: { label: string; value: number; warn?: boolean }) {
  return (
    <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
      <p className="text-sm font-medium text-slate-500 dark:text-slate-400">{label}</p>
      <p className={cn('mt-1 text-2xl font-bold', warn ? 'text-amber-600' : 'text-slate-900 dark:text-slate-100')}>{value}</p>
    </div>
  )
}
