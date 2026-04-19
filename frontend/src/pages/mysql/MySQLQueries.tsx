import { useState, useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { mysqlApi } from '@/api/mysql'
import { DetailPageLayout } from '@/components/db/DetailPageLayout'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { cn } from '@/lib/utils'
import { REFETCH_INTERVAL } from '@/lib/constants'
import type { MySQLDigestStat } from '@/types'
import { AlertTriangle, Search, XCircle, Filter } from 'lucide-react'

export default function MySQLQueries() {
  const { data: health, isLoading, error, refetch } = useQuery({
    queryKey: ['mysql', 'health'],
    queryFn: mysqlApi.health,
    refetchInterval: REFETCH_INTERVAL,
  })

  const [searchParams] = useSearchParams()
  const highlight = searchParams.get('highlight')
  const filterParam = searchParams.get('filter')
  const [activeFilter, setActiveFilter] = useState(filterParam || 'all')

  useEffect(() => {
    if (filterParam) setActiveFilter(filterParam)
  }, [filterParam])

  useEffect(() => {
    if (highlight) {
      const el = document.getElementById(`stat-${highlight}`)
      if (el) setTimeout(() => el.scrollIntoView({ behavior: 'smooth', block: 'center' }), 150)
    }
  }, [highlight])

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message="Failed to load MySQL queries" retry={() => refetch()} />
  if (!health) return null

  const topQueries: MySQLDigestStat[] = health.topQueries || []

  // Identify dangerous queries: examines >> sent (needs index)
  const inefficientQueries = topQueries
    .filter(q => q.sumRowsSent > 0 && q.sumRowsExam / q.sumRowsSent > 100)
    .sort((a, b) => (b.sumRowsExam / b.sumRowsSent) - (a.sumRowsExam / a.sumRowsSent))

  // Queries with errors
  const errorQueries = topQueries
    .filter(q => (q.sumErrors ?? 0) > 0)
    .sort((a, b) => (b.sumErrors ?? 0) - (a.sumErrors ?? 0))

  const filteredQueries = (() => {
    switch (activeFilter) {
      case 'inefficient': return topQueries.filter(q => q.sumRowsSent > 0 && q.sumRowsExam / q.sumRowsSent > 100)
      case 'errors': return topQueries.filter(q => (q.sumErrors ?? 0) > 0)
      case 'slow': return topQueries.filter(q => q.avgTimerWait > 0.1)
      default: return topQueries
    }
  })()

  const filterLabels: Record<string, string> = { all: 'All Queries', inefficient: 'Inefficient', errors: 'With Errors', slow: 'Slow (>100ms)' }
  const filterCounts: Record<string, number> = {
    all: topQueries.length,
    inefficient: inefficientQueries.length,
    errors: errorQueries.length,
    slow: topQueries.filter(q => q.avgTimerWait > 0.1).length,
  }

  return (
    <DetailPageLayout backTo="/mysql" backLabel="Back to MySQL" title="Slow Queries & Digests" subtitle={`${health.totalSlowQueries} total slow queries · ${health.queriesPerSec.toFixed(1)} queries/sec`}>
      {/* Summary cards */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-3 xl:grid-cols-6">
        <SummaryCard label="Queries/sec" value={health.queriesPerSec.toFixed(1)} />
        <SummaryCard label="Total Slow Queries" value={String(health.totalSlowQueries)} warn={health.totalSlowQueries > 0} />
        <SummaryCard label="Slow/sec" value={health.slowQueries > 0 ? health.slowQueries.toFixed(3) : '0'} warn={health.slowQueries > 0} />
        <SummaryCard id="stat-full-scans" highlighted={highlight === 'full-scans'} label="Full Table Scans" value={formatNumber(health.selectScan ?? 0)} warn={(health.selectScan ?? 0) > 1000} />
        <SummaryCard id="stat-full-joins" highlighted={highlight === 'full-joins'} label="Full Joins (no idx)" value={formatNumber(health.selectFullJoin ?? 0)} warn={(health.selectFullJoin ?? 0) > 0} />
        <SummaryCard id="stat-sort-merge" highlighted={highlight === 'sort-merge'} label="Sort Merge Passes" value={formatNumber(health.sortMergePasses ?? 0)} warn={(health.sortMergePasses ?? 0) > 10} />
      </div>

      {/* Inefficient Queries - DANGER SECTION */}
      {inefficientQueries.length > 0 && (
        <div className="rounded-xl border-2 border-amber-300 bg-amber-50/50 dark:border-amber-700 dark:bg-amber-950/20">
          <div className="border-b border-amber-200 px-5 py-3.5 dark:border-amber-800 flex items-center gap-2">
            <AlertTriangle className="h-4 w-4 text-amber-600" />
            <h2 className="text-sm font-semibold text-amber-900 dark:text-amber-200">
              Inefficient Queries — Examining Many Rows, Returning Few ({inefficientQueries.length})
            </h2>
          </div>
          <div className="p-3 text-xs text-amber-700 dark:text-amber-400 bg-amber-50 dark:bg-amber-950/30 border-b border-amber-100 dark:border-amber-900/50">
            <Search className="inline h-3 w-3 mr-1" />
            These queries examine far more rows than they return. They likely need index optimization or query rewriting.
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-amber-200 bg-amber-50/80 dark:border-amber-800 dark:bg-amber-900/20">
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-amber-700 dark:text-amber-400">Query Digest</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-amber-700 dark:text-amber-400">Rows Examined</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-amber-700 dark:text-amber-400">Rows Sent</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-amber-700 dark:text-amber-400">Ratio</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-amber-700 dark:text-amber-400">Calls</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-amber-700 dark:text-amber-400">Avg Time</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-amber-100 dark:divide-amber-900/30">
                {inefficientQueries.map((q, i) => {
                  const ratio = Math.round(q.sumRowsExam / q.sumRowsSent)
                  return (
                    <tr key={i} className={cn(ratio > 1000 && 'bg-red-50/50 dark:bg-red-900/10')}>
                      <td className="px-4 py-2.5 max-w-lg">
                        <code className="block whitespace-pre-wrap break-all rounded bg-white/60 px-2 py-1 text-xs text-slate-700 dark:bg-slate-800 dark:text-slate-300">
                          {q.digestText}
                        </code>
                      </td>
                      <td className="px-4 py-2.5 text-right font-mono text-xs font-semibold text-red-600">{formatNumber(q.sumRowsExam)}</td>
                      <td className="px-4 py-2.5 text-right font-mono text-xs">{formatNumber(q.sumRowsSent)}</td>
                      <td className="px-4 py-2.5 text-right font-mono text-xs">
                        <span className={cn('rounded px-1.5 py-0.5 font-bold', ratio > 1000 ? 'bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-300' : 'bg-amber-100 text-amber-700 dark:bg-amber-900/50 dark:text-amber-300')}>
                          {ratio}:1
                        </span>
                      </td>
                      <td className="px-4 py-2.5 text-right font-mono text-xs">{formatNumber(q.countStar)}</td>
                      <td className="px-4 py-2.5 text-right font-mono text-xs">{q.avgTimerWait.toFixed(4)}s</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Error Queries */}
      {errorQueries.length > 0 && (
        <div className="rounded-xl border-2 border-red-300 bg-red-50/30 dark:border-red-800 dark:bg-red-950/20">
          <div className="border-b border-red-200 px-5 py-3.5 dark:border-red-800 flex items-center gap-2">
            <XCircle className="h-4 w-4 text-red-600" />
            <h2 className="text-sm font-semibold text-red-900 dark:text-red-200">
              Queries Producing Errors ({errorQueries.length})
            </h2>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-red-200 bg-red-50/80 dark:border-red-800 dark:bg-red-900/20">
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-red-700 dark:text-red-400">Query Digest</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-red-700 dark:text-red-400">Errors</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-red-700 dark:text-red-400">Warnings</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-red-700 dark:text-red-400">Total Calls</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-red-700 dark:text-red-400">Error Rate</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-red-100 dark:divide-red-900/30">
                {errorQueries.map((q, i) => {
                  const errRate = q.countStar > 0 ? ((q.sumErrors ?? 0) / q.countStar * 100) : 0
                  return (
                    <tr key={i}>
                      <td className="px-4 py-2.5 max-w-lg">
                        <code className="block whitespace-pre-wrap break-all rounded bg-white/60 px-2 py-1 text-xs text-slate-700 dark:bg-slate-800 dark:text-slate-300">
                          {q.digestText}
                        </code>
                      </td>
                      <td className="px-4 py-2.5 text-right font-mono text-xs font-semibold text-red-600">{formatNumber(q.sumErrors ?? 0)}</td>
                      <td className="px-4 py-2.5 text-right font-mono text-xs text-amber-600">{formatNumber(q.sumWarnings ?? 0)}</td>
                      <td className="px-4 py-2.5 text-right font-mono text-xs">{formatNumber(q.countStar)}</td>
                      <td className="px-4 py-2.5 text-right font-mono text-xs">
                        <span className={cn('rounded px-1.5 py-0.5 font-bold', errRate > 10 ? 'bg-red-100 text-red-700' : 'bg-amber-100 text-amber-700')}>
                          {errRate.toFixed(1)}%
                        </span>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Full query digest table with filters */}
      <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
        <div className="border-b border-slate-100 px-5 py-3.5 dark:border-slate-800">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">
              {activeFilter === 'all' ? `Top Queries by Execution Time (${filteredQueries.length})` : `${filterLabels[activeFilter]} (${filteredQueries.length} of ${topQueries.length})`}
            </h2>
            <div className="flex items-center gap-1.5">
              <Filter className="h-3.5 w-3.5 text-slate-400" />
              {Object.entries(filterLabels).map(([key, label]) => (
                <button key={key} onClick={() => setActiveFilter(key)} className={cn(
                  'rounded-lg px-2.5 py-1 text-xs font-medium transition-all',
                  activeFilter === key
                    ? 'bg-blue-600 text-white shadow-sm'
                    : 'bg-slate-100 text-slate-500 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-400 dark:hover:bg-slate-700'
                )}>
                  {label}{filterCounts[key] > 0 && key !== 'all' ? ` (${filterCounts[key]})` : ''}
                </button>
              ))}
            </div>
          </div>
        </div>
        {filteredQueries.length > 0 ? (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-100 bg-slate-50/50 dark:border-slate-800 dark:bg-slate-800/30">
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">#</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold uppercase text-slate-500">Query Digest</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-slate-500">Calls</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-slate-500">Total Time</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-slate-500">Avg Time</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-slate-500">Rows Sent</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-slate-500">Rows Examined</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-slate-500">Exam/Sent</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-slate-500">Errors</th>
                  <th className="px-4 py-2.5 text-right text-xs font-semibold uppercase text-slate-500">Last Seen</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {filteredQueries.map((q, i) => {
                  const examSentRatio = q.sumRowsSent > 0 ? Math.round(q.sumRowsExam / q.sumRowsSent) : 0
                  const isInefficient = examSentRatio > 100
                  return (
                    <tr key={i} className={cn(isInefficient && 'bg-amber-50/50 dark:bg-amber-900/10', q.avgTimerWait > 1 && !isInefficient && 'bg-red-50/30 dark:bg-red-900/10')}>
                    <td className="px-4 py-2.5 font-mono text-xs text-slate-400">{i + 1}</td>
                    <td className="px-4 py-2.5 max-w-lg">
                      <code className="block whitespace-pre-wrap break-all rounded bg-slate-50 px-2 py-1 text-xs text-slate-700 dark:bg-slate-800 dark:text-slate-300">
                        {q.digestText}
                      </code>
                    </td>
                    <td className="px-4 py-2.5 text-right font-mono text-xs">{formatNumber(q.countStar)}</td>
                    <td className={cn('px-4 py-2.5 text-right font-mono text-xs', q.sumTimerWait > 10 ? 'text-red-600 font-semibold' : q.sumTimerWait > 1 ? 'text-amber-600' : '')}>
                      {q.sumTimerWait.toFixed(4)}s
                    </td>
                    <td className={cn('px-4 py-2.5 text-right font-mono text-xs', q.avgTimerWait > 1 ? 'text-red-600 font-semibold' : q.avgTimerWait > 0.1 ? 'text-amber-600' : '')}>
                      {q.avgTimerWait.toFixed(4)}s
                    </td>
                    <td className="px-4 py-2.5 text-right font-mono text-xs">{formatNumber(q.sumRowsSent)}</td>
                    <td className="px-4 py-2.5 text-right font-mono text-xs">{formatNumber(q.sumRowsExam)}</td>
                    <td className="px-4 py-2.5 text-right font-mono text-xs">
                      {examSentRatio > 0 ? (
                        <span className={cn('rounded px-1.5 py-0.5 text-[10px] font-bold', isInefficient ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/50 dark:text-amber-300' : 'text-slate-400')}>
                          {examSentRatio > 0 ? `${examSentRatio}:1` : '—'}
                        </span>
                      ) : '—'}
                    </td>
                    <td className={cn('px-4 py-2.5 text-right font-mono text-xs', (q.sumErrors ?? 0) > 0 && 'text-red-600 font-semibold')}>
                      {(q.sumErrors ?? 0) > 0 ? formatNumber(q.sumErrors) : '—'}
                    </td>
                    <td className="px-4 py-2.5 text-right text-xs text-slate-500">{q.lastSeen ? new Date(q.lastSeen).toLocaleTimeString() : '—'}</td>
                  </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="p-8 text-center text-sm text-slate-400">
            {activeFilter === 'all' ? 'No query digests available' : (
              <div>
                <p>No queries match the &ldquo;{filterLabels[activeFilter]}&rdquo; filter</p>
                <button onClick={() => setActiveFilter('all')} className="mt-2 text-blue-600 hover:text-blue-700 text-xs font-medium">Show all queries &rarr;</button>
              </div>
            )}
          </div>
        )}
      </div>
    </DetailPageLayout>
  )
}

function SummaryCard({ label, value, warn, id, highlighted }: { label: string; value: string; warn?: boolean; id?: string; highlighted?: boolean }) {
  return (
    <div id={id} className={cn(
      'rounded-xl border bg-white p-5 dark:bg-slate-900 transition-all',
      highlighted
        ? 'border-blue-400 ring-2 ring-blue-400/50 shadow-lg shadow-blue-100 dark:border-blue-500 dark:ring-blue-500/30 dark:shadow-blue-900/30'
        : 'border-slate-200 dark:border-slate-800'
    )}>
      <p className="text-sm font-medium text-slate-500 dark:text-slate-400">{label}</p>
      <p className={cn('mt-1 text-2xl font-bold tracking-tight', warn ? 'text-amber-600' : 'text-slate-900 dark:text-slate-100')}>{value}</p>
    </div>
  )
}

function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}
