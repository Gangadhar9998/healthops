import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Settings as SettingsIcon, Bell, Download, Key, Trash2 } from 'lucide-react'
import { settingsApi } from '@/api/settings'
import { aiApi } from '@/api/ai'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { cn } from '@/lib/utils'
import type { AlertRule, AIProviderConfig } from '@/types'

type Tab = 'general' | 'alerts' | 'ai' | 'export'

export default function Settings() {
  const [tab, setTab] = useState<Tab>('general')
  const tabs: { id: Tab; label: string; icon: React.ReactNode }[] = [
    { id: 'general', label: 'General', icon: <SettingsIcon className="h-4 w-4" /> },
    { id: 'alerts', label: 'Alert Rules', icon: <Bell className="h-4 w-4" /> },
    { id: 'ai', label: 'AI Providers', icon: <Key className="h-4 w-4" /> },
    { id: 'export', label: 'Export', icon: <Download className="h-4 w-4" /> },
  ]

  return (
    <div className="space-y-6 animate-fade-in">
      <div>
        <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">Settings</h1>
        <p className="text-sm text-slate-500">Manage monitoring configuration</p>
      </div>

      {/* Tab navigation */}
      <div className="flex gap-1 overflow-x-auto rounded-lg border border-slate-200 bg-slate-50 p-1 dark:border-slate-700 dark:bg-slate-800">
        {tabs.map(t => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={cn(
              'inline-flex items-center gap-1.5 rounded-md px-3 py-2 text-sm font-medium transition-colors',
              tab === t.id
                ? 'bg-white text-slate-900 shadow-sm dark:bg-slate-900 dark:text-slate-100'
                : 'text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-300',
            )}
          >
            {t.icon}
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'general' && <GeneralSettings />}
      {tab === 'alerts' && <AlertSettings />}
      {tab === 'ai' && <AISettings />}
      {tab === 'export' && <ExportSettings />}
    </div>
  )
}

function GeneralSettings() {
  const { data: config, isLoading, error, refetch } = useQuery({
    queryKey: ['settings', 'config'],
    queryFn: settingsApi.config,
  })

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message={error.message} retry={() => refetch()} />
  if (!config) return null

  return (
    <div className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
      <h2 className="mb-4 text-sm font-semibold text-slate-900 dark:text-slate-100">System Configuration</h2>
      <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {Object.entries(config).map(([key, value]) => (
          <div key={key} className="rounded-lg bg-slate-50 px-4 py-3 dark:bg-slate-800">
            <dt className="text-xs font-medium text-slate-500 dark:text-slate-400">{key}</dt>
            <dd className="mt-1 text-sm font-medium text-slate-900 dark:text-slate-100">
              {typeof value === 'object' ? JSON.stringify(value) : String(value)}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

function AlertSettings() {
  const queryClient = useQueryClient()
  const { data: rules, isLoading, error, refetch } = useQuery({
    queryKey: ['settings', 'alert-rules'],
    queryFn: settingsApi.alertRules,
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => settingsApi.deleteAlertRule(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['settings', 'alert-rules'] }),
  })

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message={error.message} retry={() => refetch()} />

  return (
    <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
      <div className="flex items-center justify-between border-b border-slate-100 px-5 py-3.5 dark:border-slate-800">
        <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Alert Rules</h2>
      </div>
      {rules && rules.length > 0 ? (
        <div className="divide-y divide-slate-100 dark:divide-slate-800">
          {rules.map((rule: AlertRule) => (
            <div key={rule.id} className="flex items-center justify-between px-5 py-3.5">
              <div>
                <p className="text-sm font-medium text-slate-900 dark:text-slate-100">{rule.name}</p>
                <p className="text-xs text-slate-500">
                  {rule.type} — threshold: {rule.threshold}, cooldown: {rule.cooldownMinutes}m,
                  breaches: {rule.consecutiveBreaches}
                </p>
              </div>
              <div className="flex items-center gap-2">
                <span className={cn(
                  'rounded-full px-2 py-0.5 text-[10px] font-semibold',
                  rule.enabled !== false
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-400'
                    : 'bg-slate-100 text-slate-500 dark:bg-slate-800',
                )}>
                  {rule.enabled !== false ? 'ACTIVE' : 'DISABLED'}
                </span>
                <button
                  onClick={() => deleteMutation.mutate(rule.id)}
                  className="rounded p-1 text-slate-400 hover:text-red-500"
                  title="Delete rule"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="px-5 py-8 text-center text-sm text-slate-500">No alert rules configured.</div>
      )}
    </div>
  )
}

function AISettings() {
  const queryClient = useQueryClient()
  const { data: config, isLoading, error, refetch } = useQuery({
    queryKey: ['ai', 'config'],
    queryFn: aiApi.config,
    retry: 1,
  })

  const toggleMutation = useMutation({
    mutationFn: (enabled: boolean) => aiApi.updateConfig({ enabled }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['ai', 'config'] }),
  })

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message="AI configuration not available." retry={() => refetch()} />

  const providers = config?.providers ?? []

  return (
    <div className="space-y-4">
      {/* Toggle */}
      <div className="flex items-center justify-between rounded-xl border border-slate-200 bg-white px-5 py-4 dark:border-slate-800 dark:bg-slate-900">
        <div>
          <h3 className="text-sm font-medium text-slate-900 dark:text-slate-100">AI Analysis</h3>
          <p className="text-xs text-slate-500">Automatically analyze incidents using configured providers</p>
        </div>
        <button
          onClick={() => toggleMutation.mutate(!config?.enabled)}
          className={cn(
            'relative inline-flex h-6 w-11 shrink-0 rounded-full transition-colors focus:outline-none',
            config?.enabled ? 'bg-blue-600' : 'bg-slate-300 dark:bg-slate-600',
          )}
        >
          <span className={cn(
            'inline-block h-5 w-5 transform rounded-full bg-white shadow transition-transform',
            config?.enabled ? 'translate-x-5.5' : 'translate-x-0.5',
          )} style={{ marginTop: '2px' }} />
        </button>
      </div>

      {/* Providers */}
      <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
        <div className="border-b border-slate-100 px-5 py-3.5 dark:border-slate-800">
          <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Providers</h2>
        </div>
        {providers.length > 0 ? (
          <div className="divide-y divide-slate-100 dark:divide-slate-800">
            {providers.map((p: AIProviderConfig) => (
              <div key={p.id} className="flex items-center justify-between px-5 py-3.5">
                <div>
                  <p className="text-sm font-medium text-slate-900 dark:text-slate-100">{p.name}</p>
                  <p className="text-xs text-slate-500">{p.provider} — {p.model} — Key: {p.apiKeyMasked || '****'}</p>
                </div>
                <div className="flex items-center gap-2">
                  {p.id === config?.activeProviderId && (
                    <span className="rounded-full bg-blue-100 px-2 py-0.5 text-[10px] font-semibold text-blue-700 dark:bg-blue-900/50 dark:text-blue-300">
                      ACTIVE
                    </span>
                  )}
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="px-5 py-8 text-center text-sm text-slate-500">No providers configured.</div>
        )}
      </div>
    </div>
  )
}

function ExportSettings() {
  const exports = [
    { label: 'Check Results (CSV)', url: settingsApi.exportResults('csv'), desc: 'All check results in CSV format' },
    { label: 'Check Results (JSON)', url: settingsApi.exportResults('json'), desc: 'All check results in JSON format' },
    { label: 'Incidents (CSV)', url: settingsApi.exportIncidents('csv'), desc: 'All incidents in CSV format' },
    { label: 'Incidents (JSON)', url: settingsApi.exportIncidents('json'), desc: 'All incidents in JSON format' },
    { label: 'MySQL Samples (CSV)', url: settingsApi.exportMysqlSamples('csv'), desc: 'MySQL monitoring samples' },
    { label: 'Audit Log (CSV)', url: settingsApi.exportAuditLog('csv'), desc: 'Security audit log' },
  ]

  return (
    <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
      <div className="border-b border-slate-100 px-5 py-3.5 dark:border-slate-800">
        <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Data Export</h2>
      </div>
      <div className="divide-y divide-slate-100 dark:divide-slate-800">
        {exports.map(e => (
          <div key={e.label} className="flex items-center justify-between px-5 py-3.5">
            <div>
              <p className="text-sm font-medium text-slate-900 dark:text-slate-100">{e.label}</p>
              <p className="text-xs text-slate-500">{e.desc}</p>
            </div>
            <a
              href={e.url}
              download
              className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 px-3 py-1.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800"
            >
              <Download className="h-3.5 w-3.5" />
              Download
            </a>
          </div>
        ))}
      </div>
    </div>
  )
}
