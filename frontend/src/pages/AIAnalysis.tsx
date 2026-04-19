import { useQuery, useMutation } from '@tanstack/react-query'
import { Brain, Zap, TestTube } from 'lucide-react'
import { aiApi } from '@/api/ai'
import { MetricCard } from '@/components/MetricCard'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { EmptyState } from '@/components/EmptyState'
import { cn, relativeTime } from '@/lib/utils'
import { REFETCH_INTERVAL } from '@/lib/constants'
import type { AIProviderConfig, AIAnalysisResult } from '@/types'

export default function AIAnalysis() {
  const { data: config, isLoading, error, refetch } = useQuery({
    queryKey: ['ai', 'config'],
    queryFn: aiApi.config,
    retry: 1,
  })

  const { data: results } = useQuery({
    queryKey: ['ai', 'results'],
    queryFn: () => aiApi.allResults(),
    refetchInterval: REFETCH_INTERVAL,
    retry: 1,
  })

  const healthMutation = useMutation({
    mutationFn: (providerId: string) => aiApi.providerHealth(providerId),
  })

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message="AI configuration not available." retry={() => refetch()} />

  const providers = config?.providers ?? []
  const activeProvider = providers.find((p: AIProviderConfig) => p.id === config?.activeProviderId)

  return (
    <div className="space-y-6 animate-fade-in">
      <div>
        <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">AI Analysis</h1>
        <p className="text-sm text-slate-500">
          {config?.enabled ? 'AI-powered incident analysis is active' : 'AI analysis is disabled'}
        </p>
      </div>

      {/* Status cards */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <MetricCard
          label="Status"
          value={config?.enabled ? 'Enabled' : 'Disabled'}
          icon={<Brain className="h-5 w-5" />}
          className={config?.enabled ? 'ring-1 ring-emerald-200 dark:ring-emerald-900' : ''}
        />
        <MetricCard label="Providers" value={providers.length} icon={<Zap className="h-5 w-5" />} />
        <MetricCard label="Active Provider" value={activeProvider?.name ?? 'None'} />
        <MetricCard label="Analyses Run" value={results?.length ?? 0} />
      </div>

      {/* Providers list */}
      <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Configured Providers</h2>
        </div>
        {providers.length === 0 ? (
          <p className="text-sm text-slate-500">No providers configured. Add one in Settings.</p>
        ) : (
          <div className="space-y-3">
            {providers.map((provider: AIProviderConfig) => (
              <div
                key={provider.id}
                className={cn(
                  'flex items-center justify-between rounded-lg border px-4 py-3',
                  provider.id === config?.activeProviderId
                    ? 'border-blue-200 bg-blue-50/50 dark:border-blue-900 dark:bg-blue-950/20'
                    : 'border-slate-200 dark:border-slate-700'
                )}
              >
                <div>
                  <p className="text-sm font-medium text-slate-900 dark:text-slate-100">
                    {provider.name}
                    {provider.id === config?.activeProviderId && (
                      <span className="ml-2 rounded-full bg-blue-100 px-2 py-0.5 text-[10px] font-semibold text-blue-700 dark:bg-blue-900/50 dark:text-blue-300">
                        ACTIVE
                      </span>
                    )}
                  </p>
                  <p className="text-xs text-slate-500">{provider.provider} — {provider.model}</p>
                </div>
                <button
                  onClick={() => healthMutation.mutate(provider.id)}
                  disabled={healthMutation.isPending}
                  className="inline-flex items-center gap-1 rounded-lg border border-slate-200 px-2.5 py-1.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:text-slate-400"
                >
                  <TestTube className="h-3 w-3" />
                  Test
                </button>
              </div>
            ))}
            {healthMutation.isSuccess && (
              <p className="text-xs text-emerald-600">Provider is healthy and reachable.</p>
            )}
            {healthMutation.isError && (
              <p className="text-xs text-red-600">Provider health check failed: {healthMutation.error?.message}</p>
            )}
          </div>
        )}
      </div>

      {/* Recent analyses */}
      <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
        <div className="border-b border-slate-100 px-5 py-3.5 dark:border-slate-800">
          <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Recent AI Analyses</h2>
        </div>
        {results && results.length > 0 ? (
          <div className="divide-y divide-slate-100 dark:divide-slate-800">
            {results.slice(0, 20).map((r: AIAnalysisResult, i: number) => (
              <div key={i} className="px-5 py-4">
                <div className="flex items-center gap-2 text-xs text-slate-500">
                  <Brain className="h-3.5 w-3.5 text-blue-500" />
                  <span>Incident {r.incidentId}</span>
                  <span>•</span>
                  <span>{r.provider} / {r.model}</span>
                  <span className="ml-auto">{relativeTime(r.createdAt)}</span>
                </div>
                <p className="mt-2 text-sm leading-relaxed text-slate-700 dark:text-slate-300 line-clamp-3 whitespace-pre-wrap">
                  {r.analysis}
                </p>
                {r.suggestions && r.suggestions.length > 0 && (
                  <div className="mt-2 flex flex-wrap gap-1">
                    {r.suggestions.slice(0, 3).map((s, j) => (
                      <span key={j} className="rounded-full bg-blue-50 px-2 py-0.5 text-[10px] text-blue-600 dark:bg-blue-950/30 dark:text-blue-400">
                        {s.length > 60 ? s.slice(0, 60) + '…' : s}
                      </span>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
        ) : (
          <div className="px-5 py-8">
            <EmptyState title="No analyses yet" description="AI analyses will appear here when incidents are processed." icon={<Brain className="h-6 w-6" />} />
          </div>
        )}
      </div>
    </div>
  )
}
