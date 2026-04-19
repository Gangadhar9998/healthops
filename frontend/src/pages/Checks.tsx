import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { useState } from 'react'
import { Search, ArrowUpDown } from 'lucide-react'
import { checksApi } from '@/api/checks'
import { StatusBadge } from '@/components/StatusBadge'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { EmptyState } from '@/components/EmptyState'
import { ExportButton } from '@/components/ExportButton'
import { cn, formatDuration, relativeTime, checkTypeLabel } from '@/lib/utils'
import { settingsApi } from '@/api/settings'
import { REFETCH_INTERVAL, CHECK_TYPES } from '@/lib/constants'
import { useExport } from '@/hooks/useExport'
import type { CheckConfig, CheckResult } from '@/types'

type SortKey = 'name' | 'type' | 'status' | 'durationMs'

export default function Checks() {
  const [search, setSearch] = useState('')
  const [typeFilter, setTypeFilter] = useState<string>('all')
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [sortKey, setSortKey] = useState<SortKey>('name')
  const [sortAsc, setSortAsc] = useState(true)

  const { data: checks, isLoading, error, refetch } = useQuery({
    queryKey: ['checks'],
    queryFn: checksApi.list,
    refetchInterval: REFETCH_INTERVAL,
  })

  const { data: results } = useQuery({
    queryKey: ['results'],
    queryFn: () => checksApi.results(),
    refetchInterval: REFETCH_INTERVAL,
  })

  const { exportCSV } = useExport()

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message={error.message} retry={() => refetch()} />
  if (!checks || checks.length === 0) return <EmptyState title="No checks configured" description="Add your first health check to start monitoring." />

  // Build a map of latest result per check
  const latestByCheck = new Map<string, CheckResult>()
  if (results) {
    for (const r of results) {
      const existing = latestByCheck.get(r.checkId)
      if (!existing || new Date(r.finishedAt) > new Date(existing.finishedAt)) {
        latestByCheck.set(r.checkId, r)
      }
    }
  }

  // Filter
  let filtered = checks.filter((c: CheckConfig) => {
    if (search && !c.name.toLowerCase().includes(search.toLowerCase()) && !c.id.toLowerCase().includes(search.toLowerCase())) return false
    if (typeFilter !== 'all' && c.type !== typeFilter) return false
    if (statusFilter !== 'all') {
      const lr = latestByCheck.get(c.id)
      if (!lr || lr.status !== statusFilter) return false
    }
    return true
  })

  // Sort
  filtered = [...filtered].sort((a, b) => {
    let cmp = 0
    switch (sortKey) {
      case 'name': cmp = a.name.localeCompare(b.name); break
      case 'type': cmp = a.type.localeCompare(b.type); break
      case 'status': {
        const sa = latestByCheck.get(a.id)?.status ?? 'unknown'
        const sb = latestByCheck.get(b.id)?.status ?? 'unknown'
        cmp = sa.localeCompare(sb); break
      }
      case 'durationMs': {
        const da = latestByCheck.get(a.id)?.durationMs ?? 0
        const db = latestByCheck.get(b.id)?.durationMs ?? 0
        cmp = da - db; break
      }
    }
    return sortAsc ? cmp : -cmp
  })

  const handleSort = (key: SortKey) => {
    if (sortKey === key) { setSortAsc(!sortAsc) }
    else { setSortKey(key); setSortAsc(true) }
  }

  const handleExport = () => {
    const rows = filtered.map(c => {
      const lr = latestByCheck.get(c.id)
      return {
        id: c.id, name: c.name, type: c.type, server: c.server ?? '',
        status: lr?.status ?? 'unknown', durationMs: lr?.durationMs ?? '',
        lastCheck: lr?.finishedAt ?? '', message: lr?.message ?? '',
      }
    })
    exportCSV(rows, 'healthops-checks.csv')
  }

  return (
    <div className="space-y-5 animate-fade-in">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">Checks</h1>
          <p className="text-sm text-slate-500">{checks.length} total, {filtered.length} shown</p>
        </div>
        <div className="flex items-center gap-2">
          <ExportButton onExportCSV={handleExport} downloadUrl={settingsApi.exportResults('csv')} />
        </div>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative flex-1 sm:max-w-xs">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" />
          <input
            type="text"
            placeholder="Search checks…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full rounded-lg border border-slate-200 bg-white py-2 pl-9 pr-3 text-sm text-slate-900 placeholder:text-slate-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
          />
        </div>
        <select
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value)}
          className="rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-600 focus:border-blue-500 focus:outline-none dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300"
        >
          <option value="all">All types</option>
          {CHECK_TYPES.map(t => <option key={t} value={t}>{checkTypeLabel(t)}</option>)}
        </select>
        <select
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          className="rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-600 focus:border-blue-500 focus:outline-none dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300"
        >
          <option value="all">All statuses</option>
          <option value="healthy">Healthy</option>
          <option value="warning">Warning</option>
          <option value="critical">Critical</option>
          <option value="unknown">Unknown</option>
        </select>
      </div>

      {/* Table */}
      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-100 bg-slate-50/50 dark:border-slate-800 dark:bg-slate-800/30">
                {([
                  ['Status', 'status'], ['Name', 'name'], ['Type', 'type'], ['Server', null], ['Response', 'durationMs'], ['Last Check', null],
                ] as [string, SortKey | null][]).map(([label, key]) => (
                  <th key={label} className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400">
                    {key ? (
                      <button onClick={() => handleSort(key)} className="inline-flex items-center gap-1 hover:text-slate-700 dark:hover:text-slate-200">
                        {label}
                        <ArrowUpDown className={cn('h-3 w-3', sortKey === key ? 'text-blue-500' : 'text-slate-300')} />
                      </button>
                    ) : label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {filtered.map((check) => {
                const lr = latestByCheck.get(check.id)
                return (
                  <tr key={check.id} className="transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50">
                    <td className="px-4 py-3">
                      <StatusBadge status={lr?.status ?? 'unknown'} />
                    </td>
                    <td className="px-4 py-3">
                      <Link to={`/checks/${check.id}`} className="font-medium text-slate-900 hover:text-blue-600 dark:text-slate-100 dark:hover:text-blue-400">
                        {check.name}
                      </Link>
                      {check.enabled === false && (
                        <span className="ml-2 rounded bg-slate-100 px-1.5 py-0.5 text-[10px] font-medium text-slate-400 dark:bg-slate-800">
                          DISABLED
                        </span>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <span className="rounded bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600 dark:bg-slate-800 dark:text-slate-400">
                        {checkTypeLabel(check.type)}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-slate-500 dark:text-slate-400">{check.server || '—'}</td>
                    <td className="px-4 py-3 font-mono text-xs text-slate-600 dark:text-slate-400">
                      {lr ? formatDuration(lr.durationMs) : '—'}
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-400">
                      {lr ? relativeTime(lr.finishedAt) : 'Never'}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
