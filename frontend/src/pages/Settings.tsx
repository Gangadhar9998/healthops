import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useState, useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'
import {
  Settings as SettingsIcon, Bell, Download, Key, Plus, Trash2,
  Pencil, X, Save, Globe, Terminal, FileText, Database, Activity,
  Eye, EyeOff, Zap, Monitor, Server, Play,
} from 'lucide-react'
import { settingsApi } from '@/api/settings'
import { aiApi } from '@/api/ai'
import { checksApi } from '@/api/checks'
import { serversApi } from '@/api/servers'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { useConfirm } from '@/components/ConfirmDialog'
import { useToast } from '@/components/Toast'
import { cn } from '@/lib/utils'
import type {
  CheckConfig, AlertRule, AlertCondition, AIProviderConfig,
  AIProviderType, RemoteServer,
} from '@/types'

type Tab = 'general' | 'servers' | 'checks' | 'alerts' | 'ai' | 'export'

export default function Settings() {
  const [searchParams] = useSearchParams()
  const initialTab = (searchParams.get('tab') as Tab) || 'general'
  const [tab, setTab] = useState<Tab>(initialTab)
  const tabs: { id: Tab; label: string; icon: React.ReactNode }[] = [
    { id: 'general', label: 'General', icon: <SettingsIcon className="h-4 w-4" /> },
    { id: 'servers', label: 'Servers', icon: <Server className="h-4 w-4" /> },
    { id: 'checks', label: 'Health Checks', icon: <Activity className="h-4 w-4" /> },
    { id: 'alerts', label: 'Alert Rules', icon: <Bell className="h-4 w-4" /> },
    { id: 'ai', label: 'AI Providers', icon: <Key className="h-4 w-4" /> },
    { id: 'export', label: 'Export', icon: <Download className="h-4 w-4" /> },
  ]

  return (
    <div className="space-y-6 animate-fade-in">
      <div>
        <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">Settings</h1>
        <p className="text-sm text-slate-500">Manage monitoring configuration, checks, alerts, and AI providers</p>
      </div>

      <div className="flex gap-1 overflow-x-auto rounded-lg border border-slate-200 bg-slate-50 p-1 dark:border-slate-700 dark:bg-slate-800">
        {tabs.map(t => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={cn(
              'inline-flex items-center gap-1.5 whitespace-nowrap rounded-md px-3 py-2 text-sm font-medium transition-colors',
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
      {tab === 'servers' && <ServersSettings />}
      {tab === 'checks' && <ChecksSettings />}
      {tab === 'alerts' && <AlertSettings />}
      {tab === 'ai' && <AISettings />}
      {tab === 'export' && <ExportSettings />}
    </div>
  )
}

/* ───────────────────────── MODAL SHELL ───────────────────────── */

function Modal({ open, onClose, title, wide, children }: {
  open: boolean; onClose: () => void; title: string; wide?: boolean; children: React.ReactNode
}) {
  if (!open) return null
  return (
    <div className="fixed inset-0 z-[80] flex items-start justify-center overflow-y-auto pt-[5vh] pb-10">
      <div className="fixed inset-0 bg-slate-900/50" onClick={onClose} />
      <div className={cn(
        'relative z-10 w-full rounded-xl border border-slate-200 bg-white p-6 shadow-xl dark:border-slate-700 dark:bg-slate-900 animate-slide-up',
        wide ? 'max-w-2xl' : 'max-w-lg',
      )}>
        <div className="mb-5 flex items-center justify-between">
          <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100">{title}</h3>
          <button onClick={onClose} className="text-slate-400 hover:text-slate-600 dark:hover:text-slate-300">
            <X className="h-5 w-5" />
          </button>
        </div>
        {children}
      </div>
    </div>
  )
}

/* ─── Form helpers ─── */

function Field({ label, required, hint, children }: {
  label: string; required?: boolean; hint?: string; children: React.ReactNode
}) {
  return (
    <label className="block">
      <span className="mb-1.5 block text-sm font-medium text-slate-700 dark:text-slate-300">
        {label}{required && <span className="text-red-500"> *</span>}
      </span>
      {children}
      {hint && <span className="mt-1 block text-xs text-slate-400">{hint}</span>}
    </label>
  )
}

const inputCls = 'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-100 dark:placeholder:text-slate-500'
const selectCls = inputCls + ' appearance-none'
const btnPrimary = 'inline-flex items-center gap-1.5 rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50'
const btnSecondary = 'inline-flex items-center gap-1.5 rounded-lg border border-slate-200 px-4 py-2 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800'

function Toggle({ checked, onChange }: { checked: boolean; onChange: () => void }) {
  return (
    <button type="button" onClick={onChange}
      className={cn(
        'relative inline-flex h-6 w-11 shrink-0 rounded-full transition-colors focus:outline-none',
        checked ? 'bg-blue-600' : 'bg-slate-300 dark:bg-slate-600',
      )}>
      <span className={cn(
        'inline-block h-5 w-5 transform rounded-full bg-white shadow transition-transform',
        checked ? 'translate-x-[22px]' : 'translate-x-[2px]',
      )} style={{ marginTop: '2px' }} />
    </button>
  )
}

/* ══════════════════════════════════════════════════════════════════
   1. GENERAL SETTINGS — editable config
   ══════════════════════════════════════════════════════════════════ */

function GeneralSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const { data: config, isLoading, error, refetch } = useQuery({
    queryKey: ['settings', 'config'],
    queryFn: settingsApi.config,
  })

  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState({ retentionDays: 7, checkIntervalSeconds: 60, workers: 8, allowCommandChecks: false })

  useEffect(() => {
    if (config) {
      setForm({
        retentionDays: config.retentionDays,
        checkIntervalSeconds: config.checkIntervalSeconds,
        workers: config.workers,
        allowCommandChecks: config.allowCommandChecks,
      })
    }
  }, [config])

  const updateMutation = useMutation({
    mutationFn: (data: Record<string, unknown>) => settingsApi.updateConfig(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings', 'config'] })
      toast.success('Configuration updated')
      setEditing(false)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message={error.message} retry={() => refetch()} />
  if (!config) return null

  return (
    <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
      <div className="flex items-center justify-between border-b border-slate-100 px-5 py-4 dark:border-slate-800">
        <div>
          <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">System Configuration</h2>
          <p className="text-xs text-slate-500 mt-0.5">Core monitoring settings</p>
        </div>
        {!editing ? (
          <button onClick={() => setEditing(true)} className={btnSecondary}>
            <Pencil className="h-3.5 w-3.5" /> Edit
          </button>
        ) : (
          <div className="flex gap-2">
            <button onClick={() => setEditing(false)} className={btnSecondary}>Cancel</button>
            <button
              onClick={() => updateMutation.mutate(form as unknown as Record<string, unknown>)}
              disabled={updateMutation.isPending}
              className={btnPrimary}
            >
              <Save className="h-3.5 w-3.5" /> {updateMutation.isPending ? 'Saving…' : 'Save'}
            </button>
          </div>
        )}
      </div>
      <div className="p-5">
        {!editing ? (
          <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <ConfigCard label="Server Address" value={config.server.addr} />
            <ConfigCard label="Auth Enabled" value={config.authEnabled ? 'Yes' : 'No'} />
            <ConfigCard label="Retention Days" value={String(config.retentionDays)} />
            <ConfigCard label="Check Interval" value={`${config.checkIntervalSeconds}s`} />
            <ConfigCard label="Workers" value={String(config.workers)} />
            <ConfigCard label="Command Checks" value={config.allowCommandChecks ? 'Allowed' : 'Disabled'} />
            <ConfigCard label="Total Checks" value={String(config.totalChecks)} />
            <ConfigCard label="Remote Servers" value={String(config.totalServers ?? 0)} />
            <ConfigCard label="Read Timeout" value={`${config.server.readTimeoutSeconds}s`} />
            <ConfigCard label="Idle Timeout" value={`${config.server.idleTimeoutSeconds}s`} />
          </dl>
        ) : (
          <div className="grid gap-5 sm:grid-cols-2">
            <Field label="Retention Days" required hint="1–365 days">
              <input type="number" min={1} max={365} value={form.retentionDays}
                onChange={e => setForm(f => ({ ...f, retentionDays: +e.target.value }))} className={inputCls} />
            </Field>
            <Field label="Check Interval (seconds)" required hint="5–3600 seconds">
              <input type="number" min={5} max={3600} value={form.checkIntervalSeconds}
                onChange={e => setForm(f => ({ ...f, checkIntervalSeconds: +e.target.value }))} className={inputCls} />
            </Field>
            <Field label="Workers" required hint="1–100 concurrent workers">
              <input type="number" min={1} max={100} value={form.workers}
                onChange={e => setForm(f => ({ ...f, workers: +e.target.value }))} className={inputCls} />
            </Field>
            <Field label="Allow Command Checks">
              <div className="flex items-center gap-3 pt-1.5">
                <Toggle checked={form.allowCommandChecks}
                  onChange={() => setForm(f => ({ ...f, allowCommandChecks: !f.allowCommandChecks }))} />
                <span className="text-sm text-slate-600 dark:text-slate-400">
                  {form.allowCommandChecks ? 'Enabled' : 'Disabled'}
                </span>
              </div>
            </Field>
          </div>
        )}
      </div>
    </div>
  )
}

function ConfigCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg bg-slate-50 px-4 py-3 dark:bg-slate-800">
      <dt className="text-xs font-medium text-slate-500 dark:text-slate-400">{label}</dt>
      <dd className="mt-1 text-sm font-semibold text-slate-900 dark:text-slate-100">{value}</dd>
    </div>
  )
}

/* ══════════════════════════════════════════════════════════════════
   1b. REMOTE SERVERS MANAGEMENT
   ══════════════════════════════════════════════════════════════════ */

function emptyServer(): Partial<RemoteServer> {
  return {
    id: '', name: '', host: '', port: 22, user: 'root',
    keyPath: '', keyEnv: '', password: '', passwordEnv: '',
    tags: [], enabled: true,
  }
}

function ServersSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const confirm = useConfirm()
  const [showForm, setShowForm] = useState(false)
  const [editServer, setEditServer] = useState<Partial<RemoteServer> | null>(null)
  const [testingId, setTestingId] = useState<string | null>(null)

  const { data: servers, isLoading, error, refetch } = useQuery({
    queryKey: ['settings', 'servers'],
    queryFn: serversApi.list,
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => serversApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings', 'servers'] })
      toast.success('Server deleted')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const createMutation = useMutation({
    mutationFn: (s: Partial<RemoteServer>) => serversApi.create(s),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings', 'servers'] })
      toast.success('Server added')
      setShowForm(false)
      setEditServer(null)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<RemoteServer> }) => serversApi.update(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings', 'servers'] })
      toast.success('Server updated')
      setShowForm(false)
      setEditServer(null)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const testMutation = useMutation({
    mutationFn: (id: string) => serversApi.test(id),
    onSuccess: (result) => {
      setTestingId(null)
      if (result.success) {
        toast.success(`SSH OK: ${result.output}`)
      } else {
        toast.error(`SSH failed: ${result.error}`)
      }
    },
    onError: (e: Error) => {
      setTestingId(null)
      toast.error(e.message)
    },
  })

  const handleDelete = async (s: RemoteServer) => {
    const ok = await confirm({
      title: 'Delete Server',
      message: `Delete "${s.name}"? Checks referencing this server will fail until reassigned.`,
      variant: 'danger',
      confirmLabel: 'Delete',
    })
    if (ok) deleteMutation.mutate(s.id)
  }

  const handleTest = (id: string) => {
    setTestingId(id)
    testMutation.mutate(id)
  }

  const openCreate = () => { setEditServer(emptyServer()); setShowForm(true) }
  const openEdit = (s: RemoteServer) => { setEditServer({ ...s }); setShowForm(true) }

  const handleSave = (data: Partial<RemoteServer>) => {
    if (editServer?.id && servers?.some(s => s.id === editServer.id)) {
      updateMutation.mutate({ id: editServer.id, data })
    } else {
      createMutation.mutate(data)
    }
  }

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message={error.message} retry={() => refetch()} />

  return (
    <>
      <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
        <div className="flex items-center justify-between border-b border-slate-100 px-5 py-4 dark:border-slate-800">
          <div>
            <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Remote Servers</h2>
            <p className="text-xs text-slate-500 mt-0.5">
              {servers?.length ?? 0} server{(servers?.length ?? 0) !== 1 ? 's' : ''} configured · SSH credentials for remote checks
            </p>
          </div>
          <button onClick={openCreate} className={btnPrimary}>
            <Plus className="h-3.5 w-3.5" /> Add Server
          </button>
        </div>

        {servers && servers.length > 0 ? (
          <div className="divide-y divide-slate-100 dark:divide-slate-800">
            {servers.map(s => (
              <div key={s.id} className="flex items-center gap-4 px-5 py-3.5 hover:bg-slate-50/50 dark:hover:bg-slate-800/50 transition-colors">
                <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-slate-100 dark:bg-slate-800">
                  <Server className="h-4 w-4 text-slate-500" />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-slate-900 dark:text-slate-100">{s.name}</p>
                  <p className="text-xs text-slate-500">
                    {s.user}@{s.host}:{s.port}
                    {s.tags && s.tags.length > 0 && ` · ${s.tags.join(', ')}`}
                  </p>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <span className={cn(
                    'rounded-full px-2 py-0.5 text-[10px] font-semibold',
                    s.enabled
                      ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-400'
                      : 'bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400',
                  )}>
                    {s.enabled ? 'ENABLED' : 'DISABLED'}
                  </span>
                  <button onClick={() => handleTest(s.id)} disabled={testingId === s.id}
                    className="rounded p-1.5 text-slate-400 hover:text-emerald-600 hover:bg-emerald-50 dark:hover:bg-emerald-950/40 transition-colors"
                    title="Test SSH Connection">
                    {testingId === s.id ? (
                      <div className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-slate-300 border-t-emerald-600" />
                    ) : (
                      <Play className="h-3.5 w-3.5" />
                    )}
                  </button>
                  <button onClick={() => openEdit(s)}
                    className="rounded p-1.5 text-slate-400 hover:text-blue-600 hover:bg-blue-50 dark:hover:bg-blue-950/40 transition-colors" title="Edit">
                    <Pencil className="h-3.5 w-3.5" />
                  </button>
                  <button onClick={() => handleDelete(s)}
                    className="rounded p-1.5 text-slate-400 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-950/40 transition-colors" title="Delete">
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="px-5 py-12 text-center">
            <Server className="mx-auto h-8 w-8 text-slate-300 dark:text-slate-600" />
            <p className="mt-2 text-sm text-slate-500">No remote servers configured</p>
            <p className="text-xs text-slate-400 mt-1">Add servers to run process, command, and log checks remotely via SSH</p>
            <button onClick={openCreate} className={cn(btnPrimary, 'mt-3')}>
              <Plus className="h-3.5 w-3.5" /> Add First Server
            </button>
          </div>
        )}
      </div>

      <Modal open={showForm} onClose={() => { setShowForm(false); setEditServer(null) }}
        title={editServer?.id && servers?.some(s => s.id === editServer.id) ? 'Edit Server' : 'Add Server'}>
        {editServer && (
          <ServerForm
            initial={editServer}
            isEdit={!!(editServer.id && servers?.some(s => s.id === editServer.id))}
            saving={createMutation.isPending || updateMutation.isPending}
            onSave={handleSave}
            onCancel={() => { setShowForm(false); setEditServer(null) }}
          />
        )}
      </Modal>
    </>
  )
}

/* ─── Server form ─── */

function ServerForm({ initial, isEdit, saving, onSave, onCancel }: {
  initial: Partial<RemoteServer>; isEdit: boolean; saving: boolean
  onSave: (data: Partial<RemoteServer>) => void; onCancel: () => void
}) {
  const [form, setForm] = useState(initial)
  const [tagsInput, setTagsInput] = useState((initial.tags ?? []).join(', '))

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const data = { ...form }
    data.tags = tagsInput.split(',').map(s => s.trim()).filter(Boolean)
    // Clean empty optional fields
    if (!data.keyPath) delete data.keyPath
    if (!data.keyEnv) delete data.keyEnv
    if (!data.password) delete data.password
    if (!data.passwordEnv) delete data.passwordEnv
    onSave(data)
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Server ID" required hint="Unique identifier (e.g., hetzner-prod)">
          <input value={form.id ?? ''} onChange={e => setForm(f => ({ ...f, id: e.target.value }))}
            disabled={isEdit} placeholder="hetzner-prod"
            className={cn(inputCls, isEdit && 'opacity-60')} required />
        </Field>
        <Field label="Display Name" required>
          <input value={form.name ?? ''} onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
            placeholder="Hetzner Production" className={inputCls} required />
        </Field>
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <Field label="Host" required hint="IP address or hostname">
          <input value={form.host ?? ''} onChange={e => setForm(f => ({ ...f, host: e.target.value }))}
            placeholder="192.168.1.100" className={inputCls} required />
        </Field>
        <Field label="SSH Port" hint="Default: 22">
          <input type="number" value={form.port ?? 22}
            onChange={e => setForm(f => ({ ...f, port: +e.target.value }))} className={inputCls} />
        </Field>
        <Field label="SSH User" required>
          <input value={form.user ?? ''} onChange={e => setForm(f => ({ ...f, user: e.target.value }))}
            placeholder="root" className={inputCls} required />
        </Field>
      </div>

      <div className="rounded-lg border border-slate-200 bg-slate-50 p-4 dark:border-slate-700 dark:bg-slate-800/50">
        <p className="text-xs font-medium text-slate-600 dark:text-slate-400 mb-3">Authentication (key or password)</p>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Key Path" hint="Path to SSH private key">
            <input value={form.keyPath ?? ''} onChange={e => setForm(f => ({ ...f, keyPath: e.target.value }))}
              placeholder="/root/.ssh/id_ed25519" className={inputCls} />
          </Field>
          <Field label="Key Env Variable" hint="Env var with key path">
            <input value={form.keyEnv ?? ''} onChange={e => setForm(f => ({ ...f, keyEnv: e.target.value }))}
              placeholder="SSH_KEY_PATH" className={inputCls} />
          </Field>
          <Field label="Password" hint="SSH password">
            <input type="password" value={form.password ?? ''}
              onChange={e => setForm(f => ({ ...f, password: e.target.value }))}
              placeholder="••••••••" className={inputCls} />
          </Field>
          <Field label="Password Env Variable" hint="Env var holding password">
            <input value={form.passwordEnv ?? ''} onChange={e => setForm(f => ({ ...f, passwordEnv: e.target.value }))}
              placeholder="SSH_PASSWORD" className={inputCls} />
          </Field>
        </div>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Tags" hint="Comma-separated labels">
          <input value={tagsInput} onChange={e => setTagsInput(e.target.value)}
            placeholder="production, hetzner" className={inputCls} />
        </Field>
        <Field label="Enabled">
          <div className="flex items-center gap-3 pt-1.5">
            <Toggle checked={form.enabled ?? true} onChange={() => setForm(f => ({ ...f, enabled: !f.enabled }))} />
            <span className="text-sm text-slate-600 dark:text-slate-400">
              {(form.enabled ?? true) ? 'Enabled' : 'Disabled'}
            </span>
          </div>
        </Field>
      </div>

      <div className="flex justify-end gap-2 pt-2 border-t border-slate-100 dark:border-slate-800">
        <button type="button" onClick={onCancel} className={btnSecondary}>Cancel</button>
        <button type="submit" disabled={saving} className={btnPrimary}>
          <Save className="h-3.5 w-3.5" /> {saving ? 'Saving…' : isEdit ? 'Update Server' : 'Add Server'}
        </button>
      </div>
    </form>
  )
}

/* ══════════════════════════════════════════════════════════════════
   2. HEALTH CHECKS MANAGEMENT
   ══════════════════════════════════════════════════════════════════ */

const CHECK_TYPES = ['api', 'tcp', 'process', 'command', 'log', 'mysql', 'ssh'] as const
type CheckType = typeof CHECK_TYPES[number]

const checkTypeInfo: Record<CheckType, { icon: React.ReactNode; label: string; color: string }> = {
  api:     { icon: <Globe className="h-4 w-4" />,    label: 'API / HTTP',  color: 'text-blue-600 dark:text-blue-400' },
  tcp:     { icon: <Zap className="h-4 w-4" />,      label: 'TCP Port',    color: 'text-purple-600 dark:text-purple-400' },
  process: { icon: <Activity className="h-4 w-4" />, label: 'Process',     color: 'text-emerald-600 dark:text-emerald-400' },
  command: { icon: <Terminal className="h-4 w-4" />,  label: 'Command',     color: 'text-amber-600 dark:text-amber-400' },
  log:     { icon: <FileText className="h-4 w-4" />,  label: 'Log File',    color: 'text-cyan-600 dark:text-cyan-400' },
  mysql:   { icon: <Database className="h-4 w-4" />,  label: 'MySQL',       color: 'text-orange-600 dark:text-orange-400' },
  ssh:     { icon: <Monitor className="h-4 w-4" />,   label: 'SSH Server',  color: 'text-rose-600 dark:text-rose-400' },
}

function emptyCheck(): Partial<CheckConfig> {
  return {
    id: '', name: '', type: 'api', target: '', host: '', port: undefined,
    command: '', path: '', expectedStatus: 200, expectedContains: '',
    timeoutSeconds: 5, warningThresholdMs: undefined, freshnessSeconds: undefined,
    intervalSeconds: undefined, retryCount: 0, retryDelaySeconds: undefined,
    server: '', application: '', enabled: true, tags: [],
    mysql: { dsnEnv: '' },
    ssh: { host: '', port: 22, user: '', keyPath: '', keyEnv: '', password: '', passwordEnv: '' },
  }
}

function ChecksSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const confirm = useConfirm()
  const [showForm, setShowForm] = useState(false)
  const [editCheck, setEditCheck] = useState<Partial<CheckConfig> | null>(null)
  const [search, setSearch] = useState('')
  const [typeFilter, setTypeFilter] = useState<string>('all')

  const { data: checks, isLoading, error, refetch } = useQuery({
    queryKey: ['settings', 'checks'],
    queryFn: checksApi.list,
  })

  const { data: servers } = useQuery({
    queryKey: ['settings', 'servers'],
    queryFn: serversApi.list,
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => checksApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings', 'checks'] })
      queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      toast.success('Check deleted')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const createMutation = useMutation({
    mutationFn: (c: Partial<CheckConfig>) => checksApi.create(c),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings', 'checks'] })
      queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      toast.success('Check created')
      setShowForm(false)
      setEditCheck(null)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<CheckConfig> }) => checksApi.update(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings', 'checks'] })
      queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      toast.success('Check updated')
      setShowForm(false)
      setEditCheck(null)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const handleDelete = async (c: CheckConfig) => {
    const ok = await confirm({
      title: 'Delete Health Check',
      message: `Are you sure you want to delete "${c.name}"? This cannot be undone.`,
      variant: 'danger',
      confirmLabel: 'Delete',
    })
    if (ok) deleteMutation.mutate(c.id)
  }

  const openCreate = () => { setEditCheck(emptyCheck()); setShowForm(true) }
  const openEdit = (c: CheckConfig) => { setEditCheck({ ...c }); setShowForm(true) }

  const handleSave = (data: Partial<CheckConfig>) => {
    if (editCheck?.id && checks?.some(c => c.id === editCheck.id)) {
      updateMutation.mutate({ id: editCheck.id, data })
    } else {
      createMutation.mutate(data)
    }
  }

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message={error.message} retry={() => refetch()} />

  const filtered = (checks ?? []).filter(c => {
    if (typeFilter !== 'all' && c.type !== typeFilter) return false
    if (search && !c.name.toLowerCase().includes(search.toLowerCase()) && !c.id.toLowerCase().includes(search.toLowerCase())) return false
    return true
  })

  return (
    <>
      <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
        <div className="flex flex-col gap-3 border-b border-slate-100 px-5 py-4 sm:flex-row sm:items-center sm:justify-between dark:border-slate-800">
          <div>
            <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Health Checks</h2>
            <p className="text-xs text-slate-500 mt-0.5">{checks?.length ?? 0} checks configured</p>
          </div>
          <button onClick={openCreate} className={btnPrimary}>
            <Plus className="h-3.5 w-3.5" /> Add Check
          </button>
        </div>

        <div className="flex flex-col gap-2 border-b border-slate-100 px-5 py-3 sm:flex-row dark:border-slate-800">
          <input type="text" placeholder="Search checks…" value={search}
            onChange={e => setSearch(e.target.value)} className={cn(inputCls, 'max-w-xs')} />
          <select value={typeFilter} onChange={e => setTypeFilter(e.target.value)}
            className={cn(selectCls, 'max-w-[160px]')}>
            <option value="all">All Types</option>
            {CHECK_TYPES.map(t => <option key={t} value={t}>{checkTypeInfo[t].label}</option>)}
          </select>
        </div>

        {filtered.length > 0 ? (
          <div className="divide-y divide-slate-100 dark:divide-slate-800">
            {filtered.map(c => (
              <div key={c.id} className="flex items-center gap-4 px-5 py-3.5 hover:bg-slate-50/50 dark:hover:bg-slate-800/50 transition-colors">
                <div className={cn('shrink-0', checkTypeInfo[c.type as CheckType]?.color)}>
                  {checkTypeInfo[c.type as CheckType]?.icon}
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-slate-900 dark:text-slate-100 truncate">{c.name}</p>
                  <p className="text-xs text-slate-500 truncate">
                    {c.type === 'api' && c.target}
                    {c.type === 'tcp' && `${c.host || c.target}:${c.port}`}
                    {c.type === 'process' && c.target}
                    {c.type === 'command' && c.command}
                    {c.type === 'log' && c.path}
                    {c.type === 'mysql' && `DSN: $${c.mysql?.dsnEnv || 'MYSQL_DSN'}`}
                    {c.type === 'ssh' && `${c.ssh?.user || 'root'}@${c.ssh?.host || '?'}:${c.ssh?.port || 22}`}
                    {c.serverId && ` · 🖥 ${servers?.find(s => s.id === c.serverId)?.name || c.serverId}`}
                    {!c.serverId && c.server && ` · ${c.server}`}
                  </p>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <span className={cn(
                    'rounded-full px-2 py-0.5 text-[10px] font-semibold',
                    c.enabled !== false
                      ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-400'
                      : 'bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400',
                  )}>
                    {c.enabled !== false ? 'ENABLED' : 'DISABLED'}
                  </span>
                  <button onClick={() => openEdit(c)}
                    className="rounded p-1.5 text-slate-400 hover:text-blue-600 hover:bg-blue-50 dark:hover:bg-blue-950/40 transition-colors" title="Edit">
                    <Pencil className="h-3.5 w-3.5" />
                  </button>
                  <button onClick={() => handleDelete(c)}
                    className="rounded p-1.5 text-slate-400 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-950/40 transition-colors" title="Delete">
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="px-5 py-12 text-center">
            <Activity className="mx-auto h-8 w-8 text-slate-300 dark:text-slate-600" />
            <p className="mt-2 text-sm text-slate-500">No checks found</p>
            <button onClick={openCreate} className={cn(btnPrimary, 'mt-3')}>
              <Plus className="h-3.5 w-3.5" /> Add Your First Check
            </button>
          </div>
        )}
      </div>

      <Modal open={showForm} onClose={() => { setShowForm(false); setEditCheck(null) }}
        title={editCheck?.id && checks?.some(c => c.id === editCheck.id) ? 'Edit Health Check' : 'Add Health Check'} wide>
        {editCheck && (
          <CheckForm
            initial={editCheck}
            isEdit={!!(editCheck.id && checks?.some(c => c.id === editCheck.id))}
            saving={createMutation.isPending || updateMutation.isPending}
            servers={servers ?? []}
            onSave={handleSave}
            onCancel={() => { setShowForm(false); setEditCheck(null) }}
          />
        )}
      </Modal>
    </>
  )
}

/* ─── Check form ─── */

function CheckForm({ initial, isEdit, saving, servers, onSave, onCancel }: {
  initial: Partial<CheckConfig>; isEdit: boolean; saving: boolean; servers: RemoteServer[]
  onSave: (data: Partial<CheckConfig>) => void; onCancel: () => void
}) {
  const [form, setForm] = useState(initial)
  const [tagsInput, setTagsInput] = useState((initial.tags ?? []).join(', '))
  const type = (form.type ?? 'api') as CheckType

  // Fetch notification channels to show which ones target this check
  const { data: channels } = useQuery({
    queryKey: ['notification-channels-raw'],
    queryFn: async () => {
      const token = localStorage.getItem('healthops_token')
      const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {}
      const res = await fetch('/api/v1/notification-channels', { headers })
      if (!res.ok) return []
      const body = await res.json()
      return (body.data || []) as Array<{
        id: string; name: string; type: string; enabled: boolean;
        checkIds?: string[]; severities?: string[];
      }>
    },
  })

  const set = <K extends keyof CheckConfig>(key: K, value: CheckConfig[K]) =>
    setForm(f => ({ ...f, [key]: value }))

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const data = { ...form }
    data.tags = tagsInput.split(',').map(s => s.trim()).filter(Boolean)
    if (type !== 'api') { delete data.expectedStatus; delete data.expectedContains }
    if (type !== 'tcp') { delete data.port }
    if (type !== 'log') { delete data.freshnessSeconds }
    if (type !== 'command') { delete data.command }
    if (type !== 'mysql') { delete data.mysql }
    if (type !== 'ssh') { delete data.ssh }
    if (type !== 'log' && type !== 'api') { delete data.path }
    if (type !== 'process' && type !== 'command' && type !== 'log') { delete data.serverId }
    onSave(data)
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      <div className="grid gap-4 sm:grid-cols-3">
        <Field label="ID" required hint="Unique identifier, lowercase">
          <input value={form.id ?? ''} onChange={e => set('id', e.target.value)}
            disabled={isEdit} placeholder="my-api-check"
            className={cn(inputCls, isEdit && 'opacity-60')} required />
        </Field>
        <Field label="Name" required>
          <input value={form.name ?? ''} onChange={e => set('name', e.target.value)}
            placeholder="My API Check" className={inputCls} required />
        </Field>
        <Field label="Type" required>
          <select value={type} onChange={e => set('type', e.target.value as CheckConfig['type'])}
            disabled={isEdit} className={cn(selectCls, isEdit && 'opacity-60')}>
            {CHECK_TYPES.map(t => <option key={t} value={t}>{checkTypeInfo[t].label}</option>)}
          </select>
        </Field>
      </div>

      {type === 'api' && (
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Target URL" required hint="https://example.com/healthz">
            <input value={form.target ?? ''} onChange={e => set('target', e.target.value)}
              placeholder="https://api.example.com/health" className={inputCls} required />
          </Field>
          <Field label="Expected Status" hint="Default: 200">
            <input type="number" value={form.expectedStatus ?? 200}
              onChange={e => set('expectedStatus', +e.target.value)} className={inputCls} />
          </Field>
          <Field label="Expected Contains" hint="Substring in response body">
            <input value={form.expectedContains ?? ''} onChange={e => set('expectedContains', e.target.value)}
              placeholder='"status":"ok"' className={inputCls} />
          </Field>
          <Field label="Warning Threshold (ms)" hint="Response time warning">
            <input type="number" value={form.warningThresholdMs ?? ''}
              onChange={e => set('warningThresholdMs', e.target.value ? +e.target.value : undefined)}
              placeholder="1000" className={inputCls} />
          </Field>
        </div>
      )}

      {type === 'tcp' && (
        <div className="grid gap-4 sm:grid-cols-3">
          <Field label="Host" required>
            <input value={form.host ?? ''} onChange={e => set('host', e.target.value)}
              placeholder="db.example.com" className={inputCls} required />
          </Field>
          <Field label="Port" required>
            <input type="number" value={form.port ?? ''} onChange={e => set('port', +e.target.value)}
              placeholder="3306" className={inputCls} required />
          </Field>
          <Field label="Warning Threshold (ms)">
            <input type="number" value={form.warningThresholdMs ?? ''}
              onChange={e => set('warningThresholdMs', e.target.value ? +e.target.value : undefined)}
              placeholder="500" className={inputCls} />
          </Field>
        </div>
      )}

      {type === 'process' && (
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Process Name" required hint="Keyword to match in ps output">
            <input value={form.target ?? ''} onChange={e => set('target', e.target.value)}
              placeholder="nginx" className={inputCls} required />
          </Field>
          <Field label="Remote Server" hint="Run on remote server via SSH">
            <select value={form.serverId ?? ''} onChange={e => set('serverId', e.target.value || undefined as unknown as string)}
              className={selectCls}>
              <option value="">Local (this server)</option>
              {servers.filter(s => s.enabled).map(s => (
                <option key={s.id} value={s.id}>{s.name} ({s.host})</option>
              ))}
            </select>
          </Field>
        </div>
      )}

      {type === 'command' && (
        <div className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Command" required hint="Shell command to execute">
              <input value={form.command ?? ''} onChange={e => set('command', e.target.value)}
                placeholder="systemctl is-active nginx" className={inputCls} required />
            </Field>
            <Field label="Expected Contains" hint="Substring in command output">
              <input value={form.expectedContains ?? ''} onChange={e => set('expectedContains', e.target.value)}
                placeholder="active" className={inputCls} />
            </Field>
          </div>
          <Field label="Remote Server" hint="Run on remote server via SSH">
            <select value={form.serverId ?? ''} onChange={e => set('serverId', e.target.value || undefined as unknown as string)}
              className={cn(selectCls, 'max-w-xs')}>
              <option value="">Local (this server)</option>
              {servers.filter(s => s.enabled).map(s => (
                <option key={s.id} value={s.id}>{s.name} ({s.host})</option>
              ))}
            </select>
          </Field>
        </div>
      )}

      {type === 'log' && (
        <div className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Log File Path" required>
              <input value={form.path ?? ''} onChange={e => set('path', e.target.value)}
                placeholder="/var/log/app/app.log" className={inputCls} required />
            </Field>
            <Field label="Freshness (seconds)" required hint="Max file age before alert">
              <input type="number" value={form.freshnessSeconds ?? ''}
                onChange={e => set('freshnessSeconds', +e.target.value)}
                placeholder="300" className={inputCls} required />
            </Field>
          </div>
          <Field label="Remote Server" hint="Check log file on remote server via SSH">
            <select value={form.serverId ?? ''} onChange={e => set('serverId', e.target.value || undefined as unknown as string)}
              className={cn(selectCls, 'max-w-xs')}>
              <option value="">Local (this server)</option>
              {servers.filter(s => s.enabled).map(s => (
                <option key={s.id} value={s.id}>{s.name} ({s.host})</option>
              ))}
            </select>
          </Field>
        </div>
      )}

      {type === 'mysql' && (
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="DSN Environment Variable" required hint="Env var containing MySQL DSN">
            <input value={form.mysql?.dsnEnv ?? ''} onChange={e => setForm(f => ({
              ...f, mysql: { ...f.mysql!, dsnEnv: e.target.value },
            }))} placeholder="MYSQL_DSN" className={inputCls} required />
          </Field>
          <Field label="Connect Timeout (s)">
            <input type="number" value={form.mysql?.connectTimeoutSeconds ?? ''}
              onChange={e => setForm(f => ({
                ...f, mysql: { ...f.mysql!, connectTimeoutSeconds: e.target.value ? +e.target.value : undefined },
              }))} placeholder="5" className={inputCls} />
          </Field>
        </div>
      )}

      {type === 'ssh' && (
        <div className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-3">
            <Field label="Host" required hint="IP address or hostname">
              <input value={form.ssh?.host ?? ''} onChange={e => setForm(f => ({
                ...f, ssh: { ...f.ssh!, host: e.target.value },
              }))} placeholder="192.168.1.100" className={inputCls} required />
            </Field>
            <Field label="Port" hint="Default: 22">
              <input type="number" value={form.ssh?.port ?? 22}
                onChange={e => setForm(f => ({
                  ...f, ssh: { ...f.ssh!, port: e.target.value ? +e.target.value : 22 },
                }))} className={inputCls} />
            </Field>
            <Field label="User" required hint="SSH username">
              <input value={form.ssh?.user ?? ''} onChange={e => setForm(f => ({
                ...f, ssh: { ...f.ssh!, user: e.target.value },
              }))} placeholder="monitor" className={inputCls} required />
            </Field>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Key Path" hint="Path to SSH private key file">
              <input value={form.ssh?.keyPath ?? ''} onChange={e => setForm(f => ({
                ...f, ssh: { ...f.ssh!, keyPath: e.target.value },
              }))} placeholder="/root/.ssh/id_rsa" className={inputCls} />
            </Field>
            <Field label="Key Env Variable" hint="Env var with path to key file">
              <input value={form.ssh?.keyEnv ?? ''} onChange={e => setForm(f => ({
                ...f, ssh: { ...f.ssh!, keyEnv: e.target.value },
              }))} placeholder="SSH_KEY_PATH" className={inputCls} />
            </Field>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Password" hint="SSH password (alternative to key)">
              <input type="password" value={form.ssh?.password ?? ''} onChange={e => setForm(f => ({
                ...f, ssh: { ...f.ssh!, password: e.target.value },
              }))} placeholder="••••••••" className={inputCls} />
            </Field>
            <Field label="Password Env Variable" hint="Env var holding the SSH password">
              <input value={form.ssh?.passwordEnv ?? ''} onChange={e => setForm(f => ({
                ...f, ssh: { ...f.ssh!, passwordEnv: e.target.value },
              }))} placeholder="SSH_PASSWORD" className={inputCls} />
            </Field>
          </div>
        </div>
      )}

      {/* Common fields */}
      <div className="grid gap-4 sm:grid-cols-3">
        <Field label="Server" hint="Logical server group">
          <input value={form.server ?? ''} onChange={e => set('server', e.target.value)}
            placeholder="production" className={inputCls} />
        </Field>
        <Field label="Application" hint="Application group">
          <input value={form.application ?? ''} onChange={e => set('application', e.target.value)}
            placeholder="backend" className={inputCls} />
        </Field>
        <Field label="Timeout (seconds)">
          <input type="number" value={form.timeoutSeconds ?? 5}
            onChange={e => set('timeoutSeconds', +e.target.value)} className={inputCls} />
        </Field>
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <Field label="Interval (seconds)" hint="Override global interval">
          <input type="number" value={form.intervalSeconds ?? ''}
            onChange={e => set('intervalSeconds', e.target.value ? +e.target.value : undefined)}
            placeholder="60" className={inputCls} />
        </Field>
        <Field label="Retry Count">
          <input type="number" min={0} value={form.retryCount ?? 0}
            onChange={e => set('retryCount', +e.target.value)} className={inputCls} />
        </Field>
        <Field label="Retry Delay (seconds)">
          <input type="number" min={1} value={form.retryDelaySeconds ?? ''}
            onChange={e => set('retryDelaySeconds', e.target.value ? +e.target.value : undefined)}
            placeholder="1" className={inputCls} />
        </Field>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Tags" hint="Comma-separated">
          <input value={tagsInput} onChange={e => setTagsInput(e.target.value)}
            placeholder="critical, production" className={inputCls} />
        </Field>
        <Field label="Enabled">
          <div className="flex items-center gap-3 pt-1.5">
            <Toggle checked={form.enabled ?? true} onChange={() => set('enabled', !(form.enabled ?? true))} />
            <span className="text-sm text-slate-600 dark:text-slate-400">
              {(form.enabled ?? true) ? 'Enabled' : 'Disabled'}
            </span>
          </div>
        </Field>
      </div>

      {/* Notification Channels linked to this check */}
      {isEdit && form.id && channels && channels.length > 0 && (
        <div className="rounded-lg border border-slate-200 p-4 dark:border-slate-700">
          <div className="mb-2 flex items-center gap-2">
            <Bell className="h-4 w-4 text-slate-500" />
            <h4 className="text-xs font-semibold text-slate-700 dark:text-slate-300">Notification Channels</h4>
            <span className="text-[10px] text-slate-400">(channels targeting this check)</span>
          </div>
          <div className="space-y-1">
            {channels.map(ch => {
              const linked = !ch.checkIds?.length || ch.checkIds.includes(form.id!)
              return (
                <div key={ch.id} className="flex items-center gap-2 rounded px-2 py-1.5 text-xs">
                  <span className={cn(
                    'h-2 w-2 rounded-full',
                    ch.enabled && linked ? 'bg-emerald-500' : 'bg-slate-300 dark:bg-slate-600',
                  )} />
                  <span className={cn(
                    'font-medium',
                    linked ? 'text-slate-700 dark:text-slate-300' : 'text-slate-400 dark:text-slate-500',
                  )}>
                    {ch.name}
                  </span>
                  <span className="text-[10px] text-slate-400 capitalize">{ch.type}</span>
                  {ch.severities && ch.severities.length > 0 && (
                    <span className="ml-auto text-[10px] text-slate-400">
                      {ch.severities.join(', ')}
                    </span>
                  )}
                  <span className={cn(
                    'ml-auto rounded-full px-1.5 py-0.5 text-[10px] font-medium',
                    linked
                      ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-400'
                      : 'bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400',
                  )}>
                    {linked ? 'ACTIVE' : 'NOT LINKED'}
                  </span>
                </div>
              )
            })}
          </div>
          <p className="mt-2 text-[10px] text-slate-400">
            Manage channel routing from Notification Channels page
          </p>
        </div>
      )}

      <div className="flex justify-end gap-2 pt-2 border-t border-slate-100 dark:border-slate-800">
        <button type="button" onClick={onCancel} className={btnSecondary}>Cancel</button>
        <button type="submit" disabled={saving} className={btnPrimary}>
          <Save className="h-3.5 w-3.5" /> {saving ? 'Saving…' : isEdit ? 'Update Check' : 'Create Check'}
        </button>
      </div>
    </form>
  )
}

/* ══════════════════════════════════════════════════════════════════
   3. ALERT RULES MANAGEMENT
   ══════════════════════════════════════════════════════════════════ */

function emptyRule(): Partial<AlertRule> {
  return {
    id: '', name: '', type: 'threshold', enabled: true,
    checkIds: [], conditions: [{ field: 'status', operator: 'equals', value: 'critical' }],
    severity: 'critical', channels: [], cooldownMinutes: 5,
    consecutiveBreaches: 3, description: '',
  }
}

function AlertSettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const confirm = useConfirm()
  const [showForm, setShowForm] = useState(false)
  const [editRule, setEditRule] = useState<Partial<AlertRule> | null>(null)

  const { data: rules, isLoading, error, refetch } = useQuery({
    queryKey: ['settings', 'alert-rules'],
    queryFn: settingsApi.alertRules,
  })

  const { data: checks } = useQuery({
    queryKey: ['settings', 'checks'],
    queryFn: checksApi.list,
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => settingsApi.deleteAlertRule(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings', 'alert-rules'] })
      toast.success('Alert rule deleted')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const createMutation = useMutation({
    mutationFn: (rule: Partial<AlertRule>) => settingsApi.createAlertRule(rule),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings', 'alert-rules'] })
      toast.success('Alert rule created')
      setShowForm(false)
      setEditRule(null)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<AlertRule> }) => settingsApi.updateAlertRule(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings', 'alert-rules'] })
      toast.success('Alert rule updated')
      setShowForm(false)
      setEditRule(null)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const toggleMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) => settingsApi.updateAlertRule(id, { enabled }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['settings', 'alert-rules'] }),
    onError: (e: Error) => toast.error(e.message),
  })

  const handleDelete = async (r: AlertRule) => {
    const ok = await confirm({
      title: 'Delete Alert Rule',
      message: `Delete "${r.name}"? This cannot be undone.`,
      variant: 'danger',
      confirmLabel: 'Delete',
    })
    if (ok) deleteMutation.mutate(r.id)
  }

  const openCreate = () => { setEditRule(emptyRule()); setShowForm(true) }
  const openEdit = (r: AlertRule) => { setEditRule({ ...r }); setShowForm(true) }

  const handleSave = (data: Partial<AlertRule>) => {
    if (editRule?.id && rules?.some(r => r.id === editRule.id)) {
      updateMutation.mutate({ id: editRule.id, data })
    } else {
      createMutation.mutate(data)
    }
  }

  if (isLoading) return <LoadingState />
  if (error) return <ErrorState message={error.message} retry={() => refetch()} />

  return (
    <>
      <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
        <div className="flex items-center justify-between border-b border-slate-100 px-5 py-4 dark:border-slate-800">
          <div>
            <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Alert Rules</h2>
            <p className="text-xs text-slate-500 mt-0.5">{rules?.length ?? 0} rules configured</p>
          </div>
          <button onClick={openCreate} className={btnPrimary}>
            <Plus className="h-3.5 w-3.5" /> Add Rule
          </button>
        </div>

        {rules && rules.length > 0 ? (
          <div className="divide-y divide-slate-100 dark:divide-slate-800">
            {rules.map((rule: AlertRule) => (
              <div key={rule.id} className="flex items-center gap-4 px-5 py-3.5 hover:bg-slate-50/50 dark:hover:bg-slate-800/50 transition-colors">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <p className="text-sm font-medium text-slate-900 dark:text-slate-100">{rule.name}</p>
                    <span className={cn(
                      'rounded-full px-1.5 py-0.5 text-[10px] font-semibold uppercase',
                      rule.severity === 'critical'
                        ? 'bg-red-100 text-red-700 dark:bg-red-950/40 dark:text-red-400'
                        : 'bg-amber-100 text-amber-700 dark:bg-amber-950/40 dark:text-amber-400',
                    )}>
                      {rule.severity}
                    </span>
                  </div>
                  <p className="text-xs text-slate-500 mt-0.5">
                    {rule.description || `${rule.conditions?.length ?? 0} conditions · cooldown ${rule.cooldownMinutes}m · ${rule.consecutiveBreaches ?? 1} breaches`}
                  </p>
                  {rule.checkIds && rule.checkIds.length > 0 && (
                    <div className="mt-1 flex flex-wrap gap-1">
                      {rule.checkIds.slice(0, 3).map(id => (
                        <span key={id} className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-600 dark:bg-slate-800 dark:text-slate-400">
                          {id}
                        </span>
                      ))}
                      {rule.checkIds.length > 3 && (
                        <span className="text-[10px] text-slate-400">+{rule.checkIds.length - 3} more</span>
                      )}
                    </div>
                  )}
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <Toggle checked={rule.enabled}
                    onChange={() => toggleMutation.mutate({ id: rule.id, enabled: !rule.enabled })} />
                  <button onClick={() => openEdit(rule)}
                    className="rounded p-1.5 text-slate-400 hover:text-blue-600 hover:bg-blue-50 dark:hover:bg-blue-950/40 transition-colors" title="Edit">
                    <Pencil className="h-3.5 w-3.5" />
                  </button>
                  <button onClick={() => handleDelete(rule)}
                    className="rounded p-1.5 text-slate-400 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-950/40 transition-colors" title="Delete">
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="px-5 py-12 text-center">
            <Bell className="mx-auto h-8 w-8 text-slate-300 dark:text-slate-600" />
            <p className="mt-2 text-sm text-slate-500">No alert rules configured</p>
            <button onClick={openCreate} className={cn(btnPrimary, 'mt-3')}>
              <Plus className="h-3.5 w-3.5" /> Create First Rule
            </button>
          </div>
        )}
      </div>

      <Modal open={showForm} onClose={() => { setShowForm(false); setEditRule(null) }}
        title={editRule?.id && rules?.some(r => r.id === editRule.id) ? 'Edit Alert Rule' : 'Add Alert Rule'} wide>
        {editRule && (
          <AlertRuleForm
            initial={editRule}
            isEdit={!!(editRule.id && rules?.some(r => r.id === editRule.id))}
            saving={createMutation.isPending || updateMutation.isPending}
            checks={checks ?? []}
            onSave={handleSave}
            onCancel={() => { setShowForm(false); setEditRule(null) }}
          />
        )}
      </Modal>
    </>
  )
}

/* ─── Alert Rule form ─── */

function AlertRuleForm({ initial, isEdit, saving, checks, onSave, onCancel }: {
  initial: Partial<AlertRule>; isEdit: boolean; saving: boolean; checks: CheckConfig[]
  onSave: (data: Partial<AlertRule>) => void; onCancel: () => void
}) {
  const [form, setForm] = useState(initial)
  const [conditions, setConditions] = useState<AlertCondition[]>(initial.conditions ?? [])
  const [selectedChecks, setSelectedChecks] = useState<Set<string>>(new Set(initial.checkIds ?? []))

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    onSave({ ...form, conditions, checkIds: Array.from(selectedChecks) })
  }

  const addCondition = () => setConditions(c => [...c, { field: 'status', operator: 'equals', value: 'critical' }])
  const removeCondition = (i: number) => setConditions(c => c.filter((_, idx) => idx !== i))
  const updateCondition = (i: number, patch: Partial<AlertCondition>) =>
    setConditions(c => c.map((cond, idx) => idx === i ? { ...cond, ...patch } : cond))

  const toggleCheck = (id: string) => {
    setSelectedChecks(prev => {
      const s = new Set(prev)
      if (s.has(id)) s.delete(id); else s.add(id)
      return s
    })
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Rule ID" required>
          <input value={form.id ?? ''} onChange={e => setForm(f => ({ ...f, id: e.target.value }))}
            disabled={isEdit} placeholder="high-latency-api"
            className={cn(inputCls, isEdit && 'opacity-60')} required />
        </Field>
        <Field label="Rule Name" required>
          <input value={form.name ?? ''} onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
            placeholder="High Latency Alert" className={inputCls} required />
        </Field>
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <Field label="Severity" required>
          <select value={form.severity ?? 'critical'}
            onChange={e => setForm(f => ({ ...f, severity: e.target.value }))} className={selectCls}>
            <option value="critical">Critical</option>
            <option value="warning">Warning</option>
          </select>
        </Field>
        <Field label="Cooldown (minutes)">
          <input type="number" min={0} value={form.cooldownMinutes ?? 5}
            onChange={e => setForm(f => ({ ...f, cooldownMinutes: +e.target.value }))} className={inputCls} />
        </Field>
        <Field label="Consecutive Breaches">
          <input type="number" min={1} value={form.consecutiveBreaches ?? 3}
            onChange={e => setForm(f => ({ ...f, consecutiveBreaches: +e.target.value }))} className={inputCls} />
        </Field>
      </div>

      <Field label="Description">
        <input value={form.description ?? ''} onChange={e => setForm(f => ({ ...f, description: e.target.value }))}
          placeholder="Alert when API response time exceeds threshold" className={inputCls} />
      </Field>

      {/* Conditions builder */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <span className="text-sm font-medium text-slate-700 dark:text-slate-300">Conditions</span>
          <button type="button" onClick={addCondition}
            className="inline-flex items-center gap-1 text-xs text-blue-600 hover:text-blue-700">
            <Plus className="h-3 w-3" /> Add Condition
          </button>
        </div>
        <div className="space-y-2">
          {conditions.map((c, i) => (
            <div key={i} className="flex items-center gap-2 rounded-lg border border-slate-200 bg-slate-50 p-2.5 dark:border-slate-700 dark:bg-slate-800">
              <input value={c.field} onChange={e => updateCondition(i, { field: e.target.value })}
                placeholder="Field (status, durationMs)" className={cn(inputCls, 'flex-1')} />
              <select value={c.operator} onChange={e => updateCondition(i, { operator: e.target.value as AlertCondition['operator'] })}
                className={cn(selectCls, 'w-36')}>
                <option value="equals">equals</option>
                <option value="not_equals">not equals</option>
                <option value="greater_than">greater than</option>
                <option value="less_than">less than</option>
              </select>
              <input value={String(c.value ?? '')} onChange={e => updateCondition(i, { value: e.target.value })}
                placeholder="Value" className={cn(inputCls, 'w-32')} />
              <button type="button" onClick={() => removeCondition(i)}
                className="shrink-0 rounded p-1 text-slate-400 hover:text-red-500">
                <X className="h-4 w-4" />
              </button>
            </div>
          ))}
          {conditions.length === 0 && (
            <p className="text-xs text-slate-400 py-2">No conditions defined yet.</p>
          )}
        </div>
      </div>

      {/* Target checks */}
      <div>
        <span className="mb-2 block text-sm font-medium text-slate-700 dark:text-slate-300">
          Apply to Checks <span className="text-xs text-slate-400 font-normal">(empty = all checks)</span>
        </span>
        <div className="max-h-40 overflow-y-auto rounded-lg border border-slate-200 dark:border-slate-700">
          {checks.length > 0 ? checks.map(c => (
            <label key={c.id}
              className="flex items-center gap-2.5 px-3 py-2 hover:bg-slate-50 dark:hover:bg-slate-800 cursor-pointer border-b border-slate-100 dark:border-slate-800 last:border-0">
              <input type="checkbox" checked={selectedChecks.has(c.id)} onChange={() => toggleCheck(c.id)}
                className="rounded border-slate-300 text-blue-600 focus:ring-blue-500" />
              <span className="text-sm text-slate-700 dark:text-slate-300">{c.name}</span>
              <span className="text-[10px] text-slate-400 ml-auto">{c.type}</span>
            </label>
          )) : (
            <p className="px-3 py-4 text-xs text-slate-400 text-center">No checks available</p>
          )}
        </div>
      </div>

      <Field label="Enabled">
        <div className="flex items-center gap-3 pt-1.5">
          <Toggle checked={form.enabled ?? true} onChange={() => setForm(f => ({ ...f, enabled: !f.enabled }))} />
          <span className="text-sm text-slate-600 dark:text-slate-400">
            {form.enabled ? 'Enabled' : 'Disabled'}
          </span>
        </div>
      </Field>

      <div className="flex justify-end gap-2 pt-2 border-t border-slate-100 dark:border-slate-800">
        <button type="button" onClick={onCancel} className={btnSecondary}>Cancel</button>
        <button type="submit" disabled={saving} className={btnPrimary}>
          <Save className="h-3.5 w-3.5" /> {saving ? 'Saving…' : isEdit ? 'Update Rule' : 'Create Rule'}
        </button>
      </div>
    </form>
  )
}

/* ══════════════════════════════════════════════════════════════════
   4. AI PROVIDERS MANAGEMENT
   ══════════════════════════════════════════════════════════════════ */

const PROVIDER_TYPES: { value: AIProviderType; label: string }[] = [
  { value: 'openai',    label: 'OpenAI' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'google',    label: 'Google Gemini' },
  { value: 'ollama',    label: 'Ollama (Local)' },
  { value: 'custom',    label: 'Custom (OpenAI-compatible)' },
]

function emptyProvider(): Partial<AIProviderConfig> {
  return {
    id: '', provider: 'openai', name: '', apiKey: '', baseURL: '',
    model: 'gpt-4o', maxTokens: 4096, temperature: 0.7, enabled: true, isDefault: false,
  }
}

function AISettings() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const confirm = useConfirm()
  const [showForm, setShowForm] = useState(false)
  const [editProvider, setEditProvider] = useState<Partial<AIProviderConfig> | null>(null)

  const { data: config, isLoading: configLoading, error: configError, refetch: refetchConfig } = useQuery({
    queryKey: ['ai', 'config'],
    queryFn: aiApi.config,
    retry: 1,
  })

  const { data: providers, isLoading: providersLoading } = useQuery({
    queryKey: ['ai', 'providers'],
    queryFn: aiApi.providers,
    retry: 1,
  })

  const toggleAIMutation = useMutation({
    mutationFn: (data: { enabled?: boolean; autoAnalyze?: boolean }) => aiApi.updateConfig(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['ai', 'config'] })
      toast.success('AI config updated')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const createMutation = useMutation({
    mutationFn: (p: Partial<AIProviderConfig>) => aiApi.addProvider(p),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['ai', 'providers'] })
      queryClient.invalidateQueries({ queryKey: ['ai', 'config'] })
      toast.success('Provider added')
      setShowForm(false)
      setEditProvider(null)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<AIProviderConfig> }) => aiApi.updateProvider(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['ai', 'providers'] })
      queryClient.invalidateQueries({ queryKey: ['ai', 'config'] })
      toast.success('Provider updated')
      setShowForm(false)
      setEditProvider(null)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => aiApi.deleteProvider(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['ai', 'providers'] })
      queryClient.invalidateQueries({ queryKey: ['ai', 'config'] })
      toast.success('Provider deleted')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const handleDelete = async (p: AIProviderConfig) => {
    const ok = await confirm({
      title: 'Delete AI Provider',
      message: `Delete "${p.name}"? This will remove the provider and its API key.`,
      variant: 'danger',
      confirmLabel: 'Delete',
    })
    if (ok) deleteMutation.mutate(p.id)
  }

  const openCreate = () => { setEditProvider(emptyProvider()); setShowForm(true) }
  const openEdit = (p: AIProviderConfig) => { setEditProvider({ ...p, apiKey: '' }); setShowForm(true) }

  const handleSave = (data: Partial<AIProviderConfig>) => {
    if (editProvider?.id && providerList.some(p => p.id === editProvider.id)) {
      if (!data.apiKey) delete data.apiKey
      updateMutation.mutate({ id: editProvider.id, data })
    } else {
      createMutation.mutate(data)
    }
  }

  const isLoading = configLoading || providersLoading
  if (isLoading) return <LoadingState />
  if (configError) return <ErrorState message="AI configuration not available." retry={() => refetchConfig()} />

  const providerList = providers ?? config?.providers ?? []

  return (
    <>
      <div className="space-y-4">
        {/* AI Global Toggles */}
        <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
          <div className="px-5 py-4 space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <h3 className="text-sm font-medium text-slate-900 dark:text-slate-100">AI Analysis</h3>
                <p className="text-xs text-slate-500">Enable AI-powered incident and MySQL analysis</p>
              </div>
              <Toggle checked={config?.enabled ?? false}
                onChange={() => toggleAIMutation.mutate({ enabled: !config?.enabled })} />
            </div>
            <div className="flex items-center justify-between">
              <div>
                <h3 className="text-sm font-medium text-slate-900 dark:text-slate-100">Auto-Analyze Incidents</h3>
                <p className="text-xs text-slate-500">Automatically analyze new incidents when created</p>
              </div>
              <Toggle checked={config?.autoAnalyze ?? false}
                onChange={() => toggleAIMutation.mutate({ autoAnalyze: !config?.autoAnalyze })} />
            </div>
          </div>
        </div>

        {/* Providers list */}
        <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
          <div className="flex items-center justify-between border-b border-slate-100 px-5 py-4 dark:border-slate-800">
            <div>
              <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">AI Providers</h2>
              <p className="text-xs text-slate-500 mt-0.5">{providerList.length} provider{providerList.length !== 1 ? 's' : ''} configured</p>
            </div>
            <button onClick={openCreate} className={btnPrimary}>
              <Plus className="h-3.5 w-3.5" /> Add Provider
            </button>
          </div>

          {providerList.length > 0 ? (
            <div className="divide-y divide-slate-100 dark:divide-slate-800">
              {providerList.map((p: AIProviderConfig) => (
                <div key={p.id} className="flex items-center gap-4 px-5 py-4 hover:bg-slate-50/50 dark:hover:bg-slate-800/50 transition-colors">
                  <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-gradient-to-br from-blue-50 to-indigo-50 dark:from-blue-950/40 dark:to-indigo-950/40">
                    <ProviderIcon provider={p.provider} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <p className="text-sm font-medium text-slate-900 dark:text-slate-100">{p.name}</p>
                      {p.isDefault && (
                        <span className="rounded-full bg-blue-100 px-1.5 py-0.5 text-[10px] font-semibold text-blue-700 dark:bg-blue-950/40 dark:text-blue-400">
                          DEFAULT
                        </span>
                      )}
                    </div>
                    <p className="text-xs text-slate-500 mt-0.5">
                      {p.provider} · {p.model} · Key: {p.apiKeyMasked || '••••'}
                      {p.baseURL && ` · ${p.baseURL}`}
                    </p>
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <span className={cn(
                      'rounded-full px-2 py-0.5 text-[10px] font-semibold',
                      p.enabled
                        ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-400'
                        : 'bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400',
                    )}>
                      {p.enabled ? 'ACTIVE' : 'DISABLED'}
                    </span>
                    <button onClick={() => openEdit(p)}
                      className="rounded p-1.5 text-slate-400 hover:text-blue-600 hover:bg-blue-50 dark:hover:bg-blue-950/40 transition-colors" title="Edit">
                      <Pencil className="h-3.5 w-3.5" />
                    </button>
                    <button onClick={() => handleDelete(p)}
                      className="rounded p-1.5 text-slate-400 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-950/40 transition-colors" title="Delete">
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="px-5 py-12 text-center">
              <Key className="mx-auto h-8 w-8 text-slate-300 dark:text-slate-600" />
              <p className="mt-2 text-sm text-slate-500">No AI providers configured</p>
              <p className="text-xs text-slate-400 mt-1">Add an OpenAI, Anthropic, or custom provider to enable AI analysis</p>
              <button onClick={openCreate} className={cn(btnPrimary, 'mt-3')}>
                <Plus className="h-3.5 w-3.5" /> Add Provider
              </button>
            </div>
          )}
        </div>
      </div>

      <Modal open={showForm} onClose={() => { setShowForm(false); setEditProvider(null) }}
        title={editProvider?.id && providerList.some(p => p.id === editProvider.id) ? 'Edit AI Provider' : 'Add AI Provider'}>
        {editProvider && (
          <ProviderForm
            initial={editProvider}
            isEdit={!!(editProvider.id && providerList.some(p => p.id === editProvider.id))}
            saving={createMutation.isPending || updateMutation.isPending}
            onSave={handleSave}
            onCancel={() => { setShowForm(false); setEditProvider(null) }}
          />
        )}
      </Modal>
    </>
  )
}

function ProviderIcon({ provider }: { provider: string }) {
  switch (provider) {
    case 'openai':    return <span className="text-lg font-bold text-emerald-600">AI</span>
    case 'anthropic': return <span className="text-lg font-bold text-orange-600">A</span>
    case 'google':    return <span className="text-lg font-bold text-blue-600">G</span>
    case 'ollama':    return <span className="text-lg font-bold text-purple-600">🦙</span>
    default:          return <span className="text-lg font-bold text-slate-600">⚡</span>
  }
}

/* ─── Provider form ─── */

function ProviderForm({ initial, isEdit, saving, onSave, onCancel }: {
  initial: Partial<AIProviderConfig>; isEdit: boolean; saving: boolean
  onSave: (data: Partial<AIProviderConfig>) => void; onCancel: () => void
}) {
  const [form, setForm] = useState(initial)
  const [showKey, setShowKey] = useState(false)

  const needsBaseURL = form.provider === 'custom' || form.provider === 'ollama'
  const needsKey = form.provider !== 'ollama'

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const data = { ...form }
    if (!needsBaseURL) delete data.baseURL
    onSave(data)
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Provider ID" required hint="Unique identifier">
          <input value={form.id ?? ''} onChange={e => setForm(f => ({ ...f, id: e.target.value }))}
            disabled={isEdit} placeholder="my-openai" className={cn(inputCls, isEdit && 'opacity-60')} required />
        </Field>
        <Field label="Display Name" required>
          <input value={form.name ?? ''} onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
            placeholder="OpenAI GPT-4o" className={inputCls} required />
        </Field>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Provider Type" required>
          <select value={form.provider ?? 'openai'}
            onChange={e => setForm(f => ({ ...f, provider: e.target.value as AIProviderType }))} className={selectCls}>
            {PROVIDER_TYPES.map(t => <option key={t.value} value={t.value}>{t.label}</option>)}
          </select>
        </Field>
        <Field label="Model" required>
          <input value={form.model ?? ''} onChange={e => setForm(f => ({ ...f, model: e.target.value }))}
            placeholder="gpt-4o" className={inputCls} required />
        </Field>
      </div>

      {needsKey && (
        <Field label="API Key" required={!isEdit} hint={isEdit ? 'Leave blank to keep existing key' : undefined}>
          <div className="relative">
            <input type={showKey ? 'text' : 'password'} value={form.apiKey ?? ''}
              onChange={e => setForm(f => ({ ...f, apiKey: e.target.value }))}
              placeholder={isEdit ? '••••••••••••' : 'sk-...'}
              className={cn(inputCls, 'pr-10')} required={!isEdit} autoComplete="off" />
            <button type="button" onClick={() => setShowKey(!showKey)}
              className="absolute right-2.5 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600">
              {showKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            </button>
          </div>
        </Field>
      )}

      {needsBaseURL && (
        <Field label="Base URL" required hint="OpenAI-compatible API endpoint">
          <input value={form.baseURL ?? ''} onChange={e => setForm(f => ({ ...f, baseURL: e.target.value }))}
            placeholder="https://api.example.com/v1" className={inputCls} required />
        </Field>
      )}

      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Max Tokens">
          <input type="number" min={1} value={form.maxTokens ?? 4096}
            onChange={e => setForm(f => ({ ...f, maxTokens: +e.target.value }))} className={inputCls} />
        </Field>
        <Field label="Temperature" hint="0.0 – 2.0">
          <input type="number" min={0} max={2} step={0.1} value={form.temperature ?? 0.7}
            onChange={e => setForm(f => ({ ...f, temperature: +e.target.value }))} className={inputCls} />
        </Field>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Enabled">
          <div className="flex items-center gap-3 pt-1.5">
            <Toggle checked={form.enabled ?? true} onChange={() => setForm(f => ({ ...f, enabled: !f.enabled }))} />
            <span className="text-sm text-slate-600 dark:text-slate-400">
              {form.enabled ? 'Enabled' : 'Disabled'}
            </span>
          </div>
        </Field>
        <Field label="Set as Default">
          <div className="flex items-center gap-3 pt-1.5">
            <Toggle checked={form.isDefault ?? false} onChange={() => setForm(f => ({ ...f, isDefault: !f.isDefault }))} />
            <span className="text-sm text-slate-600 dark:text-slate-400">
              {form.isDefault ? 'Default provider' : 'Not default'}
            </span>
          </div>
        </Field>
      </div>

      <div className="flex justify-end gap-2 pt-2 border-t border-slate-100 dark:border-slate-800">
        <button type="button" onClick={onCancel} className={btnSecondary}>Cancel</button>
        <button type="submit" disabled={saving} className={btnPrimary}>
          <Save className="h-3.5 w-3.5" /> {saving ? 'Saving…' : isEdit ? 'Update Provider' : 'Add Provider'}
        </button>
      </div>
    </form>
  )
}

/* ══════════════════════════════════════════════════════════════════
   5. EXPORT SETTINGS
   ══════════════════════════════════════════════════════════════════ */

function ExportSettings() {
  const exports = [
    { label: 'Check Results', desc: 'Historical check execution results', csv: settingsApi.exportResults('csv'), json: settingsApi.exportResults('json') },
    { label: 'Incidents', desc: 'All incidents and their lifecycle events', csv: settingsApi.exportIncidents('csv'), json: settingsApi.exportIncidents('json') },
    { label: 'MySQL Samples', desc: 'MySQL monitoring metric samples', csv: settingsApi.exportMysqlSamples('csv'), json: settingsApi.exportMysqlSamples('json') },
    { label: 'Audit Log', desc: 'Security and configuration audit trail', csv: settingsApi.exportAuditLog('csv'), json: settingsApi.exportAuditLog('json') },
  ]

  return (
    <div className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
      <div className="border-b border-slate-100 px-5 py-4 dark:border-slate-800">
        <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Data Export</h2>
        <p className="text-xs text-slate-500 mt-0.5">Download monitoring data in CSV or JSON format</p>
      </div>
      <div className="divide-y divide-slate-100 dark:divide-slate-800">
        {exports.map(e => (
          <div key={e.label} className="flex items-center justify-between px-5 py-4">
            <div>
              <p className="text-sm font-medium text-slate-900 dark:text-slate-100">{e.label}</p>
              <p className="text-xs text-slate-500">{e.desc}</p>
            </div>
            <div className="flex items-center gap-2">
              <a href={e.csv} download
                className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 px-3 py-1.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800">
                <Download className="h-3 w-3" /> CSV
              </a>
              <a href={e.json} download
                className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 px-3 py-1.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800">
                <Download className="h-3 w-3" /> JSON
              </a>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
