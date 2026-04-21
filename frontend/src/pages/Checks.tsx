import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useState, useMemo } from 'react'
import { Search, ArrowUpDown, Server, X, Plus } from 'lucide-react'
import { checksApi } from '@/api/checks'
import { StatusBadge } from '@/components/StatusBadge'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { EmptyState } from '@/components/EmptyState'
import { ExportButton } from '@/components/ExportButton'
import { useToast } from '@/components/Toast'
import { cn, formatDuration, relativeTime, checkTypeLabel } from '@/lib/utils'
import { settingsApi } from '@/api/settings'
import { REFETCH_INTERVAL, CHECK_TYPES } from '@/lib/constants'
import { useExport } from '@/hooks/useExport'
import type { CheckConfig, CheckResult } from '@/types'

type SortKey = 'name' | 'type' | 'status' | 'durationMs'

export default function Checks() {
  const location = useLocation()
  const navigate = useNavigate()
  const toast = useToast()
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [typeFilter, setTypeFilter] = useState<string>('all')
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [sortKey, setSortKey] = useState<SortKey>('name')
  const [sortAsc, setSortAsc] = useState(true)
  const [showAddModal, setShowAddModal] = useState(false)

  // Derive server filter from URL param
  const serverFilter = useMemo(() => {
    const params = new URLSearchParams(location.search)
    return params.get('server') || 'all'
  }, [location.search])

  // Update URL when server filter changes
  const handleServerFilterChange = (val: string) => {
    if (val === 'all') {
      navigate('/checks', { replace: true })
    } else {
      navigate(`/checks?server=${encodeURIComponent(val)}`, { replace: true })
    }
  }

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

  // Derive unique server names for filter dropdown (must be before early returns)
  const serverNames = useMemo(() => {
    if (!checks) return []
    const names = new Set<string>()
    for (const c of checks) {
      if (c.server) names.add(c.server)
    }
    return Array.from(names).sort()
  }, [checks])

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
    if (serverFilter !== 'all' && c.server !== serverFilter) return false
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
          <button
            onClick={() => setShowAddModal(true)}
            className="inline-flex items-center gap-1.5 rounded-lg bg-blue-600 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700"
          >
            <Plus className="h-4 w-4" />
            Add Check
          </button>
          <ExportButton onExportCSV={handleExport} downloadUrl={settingsApi.exportResults('csv')} />
        </div>
      </div>

      {/* Server filter banner */}
      {serverFilter !== 'all' && (
        <div className="flex items-center gap-3 rounded-lg border border-blue-200 bg-blue-50 px-4 py-2.5 dark:border-blue-900 dark:bg-blue-950/30">
          <Server className="h-4 w-4 text-blue-600 dark:text-blue-400" />
          <span className="text-sm font-medium text-blue-800 dark:text-blue-300">
            Showing checks for server: <span className="font-bold">{serverFilter}</span>
          </span>
          <span className="text-xs text-blue-600 dark:text-blue-400">
            ({filtered.length} check{filtered.length !== 1 ? 's' : ''})
          </span>
          <button
            onClick={() => handleServerFilterChange('all')}
            className="ml-auto rounded-md p-1 text-blue-600 hover:bg-blue-100 dark:text-blue-400 dark:hover:bg-blue-900/50"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      )}

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
        <select
          value={serverFilter}
          onChange={(e) => handleServerFilterChange(e.target.value)}
          className={cn(
            "rounded-lg border px-3 py-2 text-sm focus:border-blue-500 focus:outline-none",
            serverFilter !== 'all'
              ? "border-blue-300 bg-blue-50 text-blue-700 dark:border-blue-700 dark:bg-blue-950/30 dark:text-blue-300"
              : "border-slate-200 bg-white text-slate-600 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300",
          )}
        >
          <option value="all">All servers</option>
          {serverNames.map(s => <option key={s} value={s}>{s}</option>)}
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

      {showAddModal && (
        <AddCheckModal
          defaultServer={serverFilter !== 'all' ? serverFilter : undefined}
          onClose={() => setShowAddModal(false)}
          onCreated={() => {
            setShowAddModal(false)
            queryClient.invalidateQueries({ queryKey: ['checks'] })
            toast.success('Check created')
          }}
        />
      )}
    </div>
  )
}

const TARGET_PLACEHOLDERS: Record<string, string> = {
  api: 'https://example.com/healthz',
  tcp: 'hostname:port',
  process: 'process name',
  command: '/usr/bin/check-script.sh',
  log: '/var/log/app.log',
  mysql: 'DSN env variable name',
  ssh: 'hostname:port',
}

function AddCheckModal({
  defaultServer,
  onClose,
  onCreated,
}: {
  defaultServer?: string
  onClose: () => void
  onCreated: () => void
}) {
  const [name, setName] = useState('')
  const [type, setType] = useState<CheckConfig['type']>('api')
  const [server, setServer] = useState(defaultServer ?? '')
  const [target, setTarget] = useState('')
  const [enabled, setEnabled] = useState(true)

  const mutation = useMutation({
    mutationFn: (check: Partial<CheckConfig>) => checksApi.create(check),
    onSuccess: () => onCreated(),
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    mutation.mutate({
      name,
      type,
      server: server || undefined,
      target,
      enabled,
    })
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div
        className="w-full max-w-lg rounded-xl bg-white p-6 shadow-xl dark:bg-slate-800"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="mb-4 text-lg font-semibold text-slate-900 dark:text-white">Add Check</h2>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="mb-1 block text-sm font-medium text-slate-700 dark:text-slate-300">Name</label>
            <input
              type="text"
              required
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="My health check"
              className="w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-slate-600 dark:bg-slate-700 dark:text-white dark:placeholder:text-slate-500"
            />
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium text-slate-700 dark:text-slate-300">Type</label>
            <select
              value={type}
              onChange={(e) => setType(e.target.value as CheckConfig['type'])}
              className="w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-slate-600 dark:bg-slate-700 dark:text-white"
            >
              {CHECK_TYPES.map((t) => (
                <option key={t} value={t}>{t}</option>
              ))}
            </select>
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium text-slate-700 dark:text-slate-300">Server</label>
            <input
              type="text"
              value={server}
              onChange={(e) => setServer(e.target.value)}
              placeholder="server name"
              className="w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-slate-600 dark:bg-slate-700 dark:text-white dark:placeholder:text-slate-500"
            />
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium text-slate-700 dark:text-slate-300">Target</label>
            <input
              type="text"
              required
              value={target}
              onChange={(e) => setTarget(e.target.value)}
              placeholder={TARGET_PLACEHOLDERS[type] ?? ''}
              className="w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-slate-600 dark:bg-slate-700 dark:text-white dark:placeholder:text-slate-500"
            />
          </div>

          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setEnabled(!enabled)}
              className={cn(
                'relative h-5 w-9 rounded-full transition-colors',
                enabled ? 'bg-blue-600' : 'bg-slate-300 dark:bg-slate-600'
              )}
            >
              <span
                className={cn(
                  'absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-white transition-transform',
                  enabled && 'translate-x-4'
                )}
              />
            </button>
            <span className="text-sm text-slate-700 dark:text-slate-300">Enabled</span>
          </div>

          {mutation.isError && (
            <p className="text-sm text-red-600 dark:text-red-400">
              {mutation.error instanceof Error ? mutation.error.message : 'Failed to create check'}
            </p>
          )}

          <div className="flex justify-end gap-3 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-600 dark:text-slate-300 dark:hover:bg-slate-700"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={mutation.isPending}
              className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {mutation.isPending ? 'Creating...' : 'Create Check'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
