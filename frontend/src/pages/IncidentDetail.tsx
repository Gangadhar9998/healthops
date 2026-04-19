import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useParams, Link } from 'react-router-dom'
import { ArrowLeft, CheckCircle, Eye, Brain } from 'lucide-react'
import { incidentsApi } from '@/api/incidents'
import { aiApi } from '@/api/ai'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { cn, relativeTime, formatDate, incidentStatusLabel, severityColor } from '@/lib/utils'

export default function IncidentDetail() {
  const { id } = useParams<{ id: string }>()
  const queryClient = useQueryClient()

  const { data: incident, isLoading, error, refetch } = useQuery({
    queryKey: ['incidents', id],
    queryFn: () => incidentsApi.get(id!),
    enabled: !!id,
  })

  const { data: snapshots } = useQuery({
    queryKey: ['incidents', id, 'snapshots'],
    queryFn: () => incidentsApi.snapshots(id!),
    enabled: !!id,
  })

  const { data: aiResult } = useQuery({
    queryKey: ['ai', 'results', id],
    queryFn: () => aiApi.results(id!),
    enabled: !!id,
    retry: false,
  })

  const ackMutation = useMutation({
    mutationFn: () => incidentsApi.acknowledge(id!),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['incidents'] }) },
  })

  const resolveMutation = useMutation({
    mutationFn: () => incidentsApi.resolve(id!),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['incidents'] }) },
  })

  const analyzeMutation = useMutation({
    mutationFn: () => aiApi.analyze(id!),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['ai', 'results', id] }) },
  })

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message={error.message} retry={() => refetch()} />
  if (!incident) return null

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Header */}
      <div className="flex items-start gap-3">
        <Link to="/incidents" className="mt-1 rounded-md p-1 text-slate-400 transition-colors hover:text-slate-600">
          <ArrowLeft className="h-5 w-5" />
        </Link>
        <div className="flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">{incident.checkName}</h1>
            <span className={cn(
              'rounded-full px-2.5 py-0.5 text-xs font-semibold uppercase',
              incident.status === 'open' ? 'bg-red-100 text-red-700 dark:bg-red-950/40 dark:text-red-400' :
              incident.status === 'acknowledged' ? 'bg-amber-100 text-amber-700 dark:bg-amber-950/40 dark:text-amber-400' :
              'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-400'
            )}>
              {incidentStatusLabel(incident.status)}
            </span>
            <span className={cn('text-sm font-medium capitalize', severityColor(incident.severity))}>
              {incident.severity}
            </span>
          </div>
          <p className="mt-1 text-sm text-slate-500">{incident.message}</p>
        </div>
      </div>

      {/* Actions */}
      {incident.status !== 'resolved' && (
        <div className="flex gap-2">
          {incident.status === 'open' && (
            <button
              onClick={() => ackMutation.mutate()}
              disabled={ackMutation.isPending}
              className="inline-flex items-center gap-1.5 rounded-lg border border-amber-200 bg-amber-50 px-4 py-2 text-sm font-medium text-amber-700 transition-colors hover:bg-amber-100 disabled:opacity-50 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-400"
            >
              <Eye className="h-4 w-4" />
              Acknowledge
            </button>
          )}
          <button
            onClick={() => resolveMutation.mutate()}
            disabled={resolveMutation.isPending}
            className="inline-flex items-center gap-1.5 rounded-lg border border-emerald-200 bg-emerald-50 px-4 py-2 text-sm font-medium text-emerald-700 transition-colors hover:bg-emerald-100 disabled:opacity-50 dark:border-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-400"
          >
            <CheckCircle className="h-4 w-4" />
            Resolve
          </button>
          <button
            onClick={() => analyzeMutation.mutate()}
            disabled={analyzeMutation.isPending}
            className="inline-flex items-center gap-1.5 rounded-lg border border-blue-200 bg-blue-50 px-4 py-2 text-sm font-medium text-blue-700 transition-colors hover:bg-blue-100 disabled:opacity-50 dark:border-blue-800 dark:bg-blue-950/30 dark:text-blue-400"
          >
            <Brain className="h-4 w-4" />
            {analyzeMutation.isPending ? 'Analyzing…' : 'AI Analysis'}
          </button>
        </div>
      )}

      {/* Timeline */}
      <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
        <h2 className="mb-4 text-sm font-semibold text-slate-900 dark:text-slate-100">Timeline</h2>
        <div className="space-y-4">
          <TimelineEntry
            label="Started"
            time={incident.startedAt}
            detail={incident.message}
            color="bg-red-500"
          />
          {incident.acknowledgedAt && (
            <TimelineEntry
              label="Acknowledged"
              time={incident.acknowledgedAt}
              detail={incident.acknowledgedBy ? `by ${incident.acknowledgedBy}` : undefined}
              color="bg-amber-500"
            />
          )}
          {incident.resolvedAt && (
            <TimelineEntry
              label="Resolved"
              time={incident.resolvedAt}
              detail={incident.resolvedBy ? `by ${incident.resolvedBy}` : undefined}
              color="bg-emerald-500"
            />
          )}
        </div>
      </div>

      {/* AI Analysis */}
      {aiResult && (
        <div className="rounded-xl border border-blue-200 bg-blue-50/50 p-5 dark:border-blue-900 dark:bg-blue-950/20">
          <div className="mb-3 flex items-center gap-2">
            <Brain className="h-4 w-4 text-blue-600 dark:text-blue-400" />
            <h2 className="text-sm font-semibold text-blue-900 dark:text-blue-300">AI Analysis</h2>
            <span className="text-xs text-blue-500">{relativeTime(aiResult.createdAt)}</span>
          </div>
          <p className="text-sm leading-relaxed text-slate-700 dark:text-slate-300 whitespace-pre-wrap">{aiResult.analysis}</p>
          {aiResult.suggestions && aiResult.suggestions.length > 0 && (
            <div className="mt-4">
              <h3 className="text-xs font-semibold uppercase text-blue-600 dark:text-blue-400">Suggestions</h3>
              <ul className="mt-2 space-y-1">
                {aiResult.suggestions.map((s, i) => (
                  <li key={i} className="text-sm text-slate-600 dark:text-slate-400">• {s}</li>
                ))}
              </ul>
            </div>
          )}
        </div>
      )}

      {/* Evidence snapshots */}
      {snapshots && snapshots.length > 0 && (
        <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
          <h2 className="mb-3 text-sm font-semibold text-slate-900 dark:text-slate-100">
            Evidence Snapshots ({snapshots.length})
          </h2>
          <div className="space-y-3">
            {snapshots.map((snap, i) => (
              <details key={i} className="group rounded-lg bg-slate-50 dark:bg-slate-800/50">
                <summary className="cursor-pointer px-4 py-2.5 text-sm font-medium text-slate-700 dark:text-slate-300">
                  {snap.snapshotType} — {formatDate(snap.timestamp)}
                </summary>
                <pre className="overflow-x-auto border-t border-slate-200 px-4 py-3 font-mono text-xs text-slate-600 dark:border-slate-700 dark:text-slate-400">
                  {(() => { try { return JSON.stringify(JSON.parse(snap.payloadJson), null, 2) } catch { return snap.payloadJson } })()}
                </pre>
              </details>
            ))}
          </div>
        </div>
      )}

      {/* Metadata */}
      {incident.metadata && Object.keys(incident.metadata).length > 0 && (
        <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
          <h2 className="mb-3 text-sm font-semibold text-slate-900 dark:text-slate-100">Metadata</h2>
          <dl className="grid gap-2 sm:grid-cols-2">
            {Object.entries(incident.metadata).map(([k, v]) => (
              <div key={k} className="text-sm">
                <dt className="font-medium text-slate-500">{k}</dt>
                <dd className="text-slate-700 dark:text-slate-300">{v}</dd>
              </div>
            ))}
          </dl>
        </div>
      )}
    </div>
  )
}

function TimelineEntry({ label, time, detail, color }: { label: string; time: string; detail?: string; color: string }) {
  return (
    <div className="flex gap-3">
      <div className="flex flex-col items-center">
        <div className={cn('h-2.5 w-2.5 rounded-full', color)} />
        <div className="w-px flex-1 bg-slate-200 dark:bg-slate-700" />
      </div>
      <div className="pb-4">
        <p className="text-sm font-medium text-slate-900 dark:text-slate-100">{label}</p>
        <p className="text-xs text-slate-400">{formatDate(time, 'MMM d, yyyy HH:mm:ss')}</p>
        {detail && <p className="mt-0.5 text-xs text-slate-500">{detail}</p>}
      </div>
    </div>
  )
}
