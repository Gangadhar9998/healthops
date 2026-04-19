import { useQuery } from '@tanstack/react-query'
import { mysqlApi } from '@/api/mysql'
import { DetailPageLayout } from '@/components/db/DetailPageLayout'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { cn } from '@/lib/utils'
import { REFETCH_INTERVAL } from '@/lib/constants'
import type { MySQLProcess } from '@/types'

export default function MySQLThreads() {
  const { data: health, isLoading, error, refetch } = useQuery({
    queryKey: ['mysql', 'health'],
    queryFn: mysqlApi.health,
    refetchInterval: REFETCH_INTERVAL,
  })

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message="Failed to load thread data" retry={() => refetch()} />
  if (!health) return null

  const processList: MySQLProcess[] = health.processList || []
  const activeThreads = processList.filter(p => p.command !== 'Sleep' && p.command !== 'Daemon')
  const sleepingThreads = processList.filter(p => p.command === 'Sleep')
  const longRunning = processList.filter(p => p.time > 5 && p.command !== 'Daemon')

  return (
    <DetailPageLayout backTo="/mysql" backLabel="Back to MySQL" title="Threads & Processes" subtitle={`${activeThreads.length} active · ${sleepingThreads.length} sleeping · ${processList.length} total`}>
      {/* Summary */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard label="Total Processes" value={processList.length} />
        <StatCard label="Active" value={activeThreads.length} warn={activeThreads.length > 10} />
        <StatCard label="Sleeping" value={sleepingThreads.length} />
        <StatCard label="Long Running (>5s)" value={longRunning.length} warn={longRunning.length > 0} />
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
