import { useState } from 'react'
import { Skull, Loader2, CheckCircle } from 'lucide-react'
import { mysqlApi } from '@/api/mysql'
import type { MySQLProcess } from '@/types'
import { cn } from '@/lib/utils'

interface Props {
  processes: MySQLProcess[]
  longRunning: MySQLProcess[]
  className?: string
}

/** Real-time process list with kill query action. */
export function LiveProcessList({ processes, longRunning, className }: Props) {
  const [killingId, setKillingId] = useState<number | null>(null)
  const [killedIds, setKilledIds] = useState<Set<number>>(new Set())
  const [killError, setKillError] = useState<string | null>(null)

  const handleKill = async (processId: number) => {
    if (!confirm(`Kill query on process ${processId}? This will cancel the running query.`)) return

    setKillingId(processId)
    setKillError(null)
    try {
      await mysqlApi.killQuery(processId)
      setKilledIds(prev => new Set(prev).add(processId))
      setTimeout(() => {
        setKilledIds(prev => {
          const next = new Set(prev)
          next.delete(processId)
          return next
        })
      }, 5000)
    } catch (err) {
      setKillError(err instanceof Error ? err.message : 'Failed to kill query')
    } finally {
      setKillingId(null)
    }
  }

  const rowColor = (p: MySQLProcess) => {
    if (killedIds.has(p.id)) return 'bg-emerald-50 dark:bg-emerald-950/30'
    if (p.time > 10) return 'bg-red-50 dark:bg-red-950/30'
    if (p.time > 5) return 'bg-amber-50 dark:bg-amber-950/30'
    if (p.command !== 'Sleep') return 'bg-blue-50 dark:bg-blue-950/30'
    return ''
  }

  return (
    <div className={cn('space-y-3', className)}>
      {/* Long-running alert banner */}
      {longRunning.length > 0 && (
        <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 dark:border-red-900 dark:bg-red-950/40">
          <div className="flex items-center gap-2 text-sm font-medium text-red-700 dark:text-red-400">
            <span className="relative flex h-2.5 w-2.5">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-400 opacity-75" />
              <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-red-500" />
            </span>
            {longRunning.length} long-running {longRunning.length === 1 ? 'query' : 'queries'} detected (&gt;5s)
          </div>
          <div className="mt-2 space-y-1">
            {longRunning.slice(0, 3).map(p => (
              <div key={p.id} className="flex items-center justify-between gap-2 text-xs text-red-600 dark:text-red-300">
                <span className="truncate font-mono">
                  [{p.id}] {p.user}@{p.host} — {p.info || p.command} ({p.time}s)
                </span>
                <button
                  onClick={() => handleKill(p.id)}
                  disabled={killingId === p.id || killedIds.has(p.id)}
                  className="shrink-0 rounded bg-red-600 px-2 py-0.5 text-white hover:bg-red-700 disabled:opacity-50"
                >
                  {killedIds.has(p.id) ? 'Killed' : killingId === p.id ? '...' : 'Kill'}
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      {killError && (
        <div className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/40 dark:text-red-400">
          {killError}
        </div>
      )}

      {/* Process list table */}
      <div className="overflow-x-auto rounded-lg border border-slate-200 dark:border-slate-800">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-slate-200 bg-slate-50 dark:border-slate-800 dark:bg-slate-900/50">
              <th className="px-3 py-2 text-left font-medium text-slate-500">ID</th>
              <th className="px-3 py-2 text-left font-medium text-slate-500">User</th>
              <th className="px-3 py-2 text-left font-medium text-slate-500">Host</th>
              <th className="px-3 py-2 text-left font-medium text-slate-500">DB</th>
              <th className="px-3 py-2 text-left font-medium text-slate-500">Command</th>
              <th className="px-3 py-2 text-right font-medium text-slate-500">Time</th>
              <th className="px-3 py-2 text-left font-medium text-slate-500">State</th>
              <th className="px-3 py-2 text-left font-medium text-slate-500">Query</th>
              <th className="px-3 py-2 text-center font-medium text-slate-500">Action</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
            {processes.map(p => (
              <tr key={p.id} className={cn('transition-colors', rowColor(p))}>
                <td className="px-3 py-2 font-mono text-xs text-slate-600 dark:text-slate-400">{p.id}</td>
                <td className="px-3 py-2 text-slate-700 dark:text-slate-300">{p.user}</td>
                <td className="px-3 py-2 text-xs text-slate-500">{p.host}</td>
                <td className="px-3 py-2 text-slate-500">{p.db || '—'}</td>
                <td className="px-3 py-2">
                  <span className={cn(
                    'inline-block rounded px-1.5 py-0.5 text-xs font-medium',
                    p.command === 'Sleep' ? 'bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400' :
                    p.command === 'Query' ? 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300' :
                    'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300'
                  )}>
                    {p.command}
                  </span>
                </td>
                <td className={cn(
                  'px-3 py-2 text-right font-mono text-xs',
                  p.time > 10 ? 'font-bold text-red-600' :
                  p.time > 5 ? 'font-semibold text-amber-600' :
                  'text-slate-500'
                )}>
                  {p.time}s
                </td>
                <td className="max-w-[120px] truncate px-3 py-2 text-xs text-slate-500">{p.state || '—'}</td>
                <td className="max-w-[200px] truncate px-3 py-2 font-mono text-xs text-slate-500" title={p.info}>
                  {p.info || '—'}
                </td>
                <td className="px-3 py-2 text-center">
                  {p.command !== 'Sleep' && p.command !== 'Daemon' ? (
                    <button
                      onClick={() => handleKill(p.id)}
                      disabled={killingId === p.id || killedIds.has(p.id)}
                      className={cn(
                        'inline-flex items-center gap-1 rounded px-2 py-1 text-xs font-medium transition-colors',
                        killedIds.has(p.id)
                          ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900 dark:text-emerald-300'
                          : 'bg-red-100 text-red-700 hover:bg-red-200 dark:bg-red-900 dark:text-red-300 dark:hover:bg-red-800',
                        'disabled:opacity-50'
                      )}
                    >
                      {killedIds.has(p.id) ? (
                        <><CheckCircle className="h-3 w-3" /> Killed</>
                      ) : killingId === p.id ? (
                        <><Loader2 className="h-3 w-3 animate-spin" /> Killing</>
                      ) : (
                        <><Skull className="h-3 w-3" /> Kill</>
                      )}
                    </button>
                  ) : null}
                </td>
              </tr>
            ))}
            {processes.length === 0 && (
              <tr>
                <td colSpan={9} className="px-4 py-8 text-center text-sm text-slate-400">
                  No active processes
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
