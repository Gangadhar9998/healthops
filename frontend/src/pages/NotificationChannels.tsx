import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Bell, Plus, Trash2, Pencil, ToggleLeft, ToggleRight, Send, Mail, Globe, MessageSquare, Hash, AlertTriangle, Filter } from 'lucide-react'
import { useAuth } from '@/hooks/useAuth'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { EmptyState } from '@/components/EmptyState'
import { useToast } from '@/components/Toast'
import { checksApi } from '@/api/checks'

interface NotificationChannel {
  id: string
  name: string
  type: string
  enabled: boolean
  webhookUrl?: string
  email?: string
  smtpHost?: string
  smtpPort?: number
  smtpUser?: string
  fromEmail?: string
  botToken?: string
  chatId?: string
  routingKey?: string
  severities?: string[]
  checkIds?: string[]
  checkTypes?: string[]
  servers?: string[]
  tags?: string[]
  cooldownMinutes?: number
  notifyOnResolve?: boolean
  headers?: Record<string, string>
  createdAt?: string
  updatedAt?: string
}

const CHANNEL_TYPES = [
  { value: 'email', label: 'Email', icon: Mail },
  { value: 'webhook', label: 'Webhook', icon: Globe },
  { value: 'slack', label: 'Slack', icon: Hash },
  { value: 'discord', label: 'Discord', icon: MessageSquare },
  { value: 'telegram', label: 'Telegram', icon: Send },
  { value: 'pagerduty', label: 'PagerDuty', icon: AlertTriangle },
]

function authHeaders(): Record<string, string> {
  const token = localStorage.getItem('healthops_token')
  return token ? { Authorization: `Bearer ${token}` } : {}
}

async function fetchChannels(): Promise<NotificationChannel[]> {
  const res = await fetch('/api/v1/notification-channels', { headers: authHeaders() })
  if (!res.ok) throw new Error('Failed to load channels')
  const body = await res.json()
  return body.data || []
}

const emptyForm: Partial<NotificationChannel> = {
  name: '', type: 'webhook', enabled: true, webhookUrl: '', email: '',
  smtpHost: '', smtpPort: 587, smtpUser: '', fromEmail: '',
  botToken: '', chatId: '', routingKey: '', cooldownMinutes: 5,
  severities: [], checkIds: [], checkTypes: [], servers: [], tags: [],
  notifyOnResolve: true,
}

export default function NotificationChannels() {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const queryClient = useQueryClient()
  const [showForm, setShowForm] = useState(false)
  const [editId, setEditId] = useState<string | null>(null)
  const [form, setForm] = useState<Partial<NotificationChannel>>(emptyForm)

  const { data: channels, isLoading, error } = useQuery({
    queryKey: ['notification-channels'],
    queryFn: fetchChannels,
  })

  const { data: checks } = useQuery({
    queryKey: ['settings', 'checks'],
    queryFn: checksApi.list,
  })

  const saveMutation = useMutation({
    mutationFn: async (data: Partial<NotificationChannel>) => {
      const url = editId ? `/api/v1/notification-channels/${editId}` : '/api/v1/notification-channels'
      const res = await fetch(url, {
        method: editId ? 'PUT' : 'POST',
        headers: { 'Content-Type': 'application/json', ...authHeaders() },
        body: JSON.stringify(data),
      })
      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        throw new Error(body?.error?.message || 'Failed to save channel')
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notification-channels'] })
      setShowForm(false)
      setEditId(null)
      setForm(emptyForm)
      toast.success(editId ? 'Channel updated' : 'Channel created')
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const toggleMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`/api/v1/notification-channels/${id}/toggle`, {
        method: 'POST',
        headers: authHeaders(),
      })
      if (!res.ok) throw new Error('Failed to toggle channel')
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['notification-channels'] }),
    onError: (err: Error) => toast.error(err.message),
  })

  const deleteMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`/api/v1/notification-channels/${id}`, {
        method: 'DELETE',
        headers: authHeaders(),
      })
      if (!res.ok) throw new Error('Failed to delete channel')
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notification-channels'] })
      toast.success('Channel deleted')
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const testMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch('/api/v1/notification-channels/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...authHeaders() },
        body: JSON.stringify({ channelId: id }),
      })
      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        throw new Error(body?.error?.message || 'Test failed')
      }
    },
    onSuccess: () => toast.success('Test notification sent'),
    onError: (err: Error) => toast.error(err.message),
  })

  if (isLoading) return <LoadingState message="Loading channels…" />
  if (error) return <ErrorState message="Failed to load channels" retry={() => { queryClient.invalidateQueries({ queryKey: ['notification-channels'] }) }} />

  const channelIcon = (type: string) => {
    const ct = CHANNEL_TYPES.find(c => c.value === type)
    return ct ? <ct.icon className="h-4 w-4" /> : <Bell className="h-4 w-4" />
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold text-slate-900 dark:text-slate-100">Notification Channels</h1>
          <p className="text-sm text-slate-500">Configure where alerts are sent</p>
        </div>
        {isAdmin && (
          <button
            onClick={() => { setShowForm(true); setEditId(null); setForm({ ...emptyForm }) }}
            className="flex items-center gap-2 rounded-lg bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700"
          >
            <Plus className="h-4 w-4" /> Add Channel
          </button>
        )}
      </div>

      {/* Form */}
      {(showForm || editId) && isAdmin && (
        <div className="rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
          <h3 className="mb-4 text-sm font-medium text-slate-900 dark:text-slate-100">
            {editId ? 'Edit Channel' : 'New Channel'}
          </h3>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Name</label>
              <input
                value={form.name || ''}
                onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
                placeholder="e.g. Ops Slack"
                className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Type</label>
              <select
                value={form.type || 'webhook'}
                onChange={e => setForm(f => ({ ...f, type: e.target.value }))}
                className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
              >
                {CHANNEL_TYPES.map(ct => (
                  <option key={ct.value} value={ct.value}>{ct.label}</option>
                ))}
              </select>
            </div>
          </div>

          {/* Type-specific fields */}
          <div className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
            {(form.type === 'webhook' || form.type === 'slack' || form.type === 'discord') && (
              <div className="sm:col-span-2">
                <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Webhook URL</label>
                <input
                  value={form.webhookUrl || ''}
                  onChange={e => setForm(f => ({ ...f, webhookUrl: e.target.value }))}
                  placeholder="https://hooks.slack.com/services/..."
                  className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
                />
              </div>
            )}

            {form.type === 'email' && (
              <>
                <div>
                  <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Recipients (comma-separated)</label>
                  <input
                    value={form.email || ''}
                    onChange={e => setForm(f => ({ ...f, email: e.target.value }))}
                    placeholder="ops@example.com, admin@example.com"
                    className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">From Email</label>
                  <input
                    value={form.fromEmail || ''}
                    onChange={e => setForm(f => ({ ...f, fromEmail: e.target.value }))}
                    placeholder="healthops@example.com"
                    className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">SMTP Host</label>
                  <input
                    value={form.smtpHost || ''}
                    onChange={e => setForm(f => ({ ...f, smtpHost: e.target.value }))}
                    placeholder="smtp.gmail.com"
                    className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">SMTP Port</label>
                  <input
                    type="number"
                    value={form.smtpPort || 587}
                    onChange={e => setForm(f => ({ ...f, smtpPort: parseInt(e.target.value) || 587 }))}
                    className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">SMTP Username</label>
                  <input
                    value={form.smtpUser || ''}
                    onChange={e => setForm(f => ({ ...f, smtpUser: e.target.value }))}
                    className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
                  />
                </div>
              </>
            )}

            {form.type === 'telegram' && (
              <>
                <div>
                  <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Bot Token</label>
                  <input
                    value={form.botToken || ''}
                    onChange={e => setForm(f => ({ ...f, botToken: e.target.value }))}
                    placeholder="123456:ABC-DEF..."
                    className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Chat ID</label>
                  <input
                    value={form.chatId || ''}
                    onChange={e => setForm(f => ({ ...f, chatId: e.target.value }))}
                    placeholder="-1001234567890"
                    className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
                  />
                </div>
              </>
            )}

            {form.type === 'pagerduty' && (
              <div className="sm:col-span-2">
                <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Routing Key</label>
                <input
                  value={form.routingKey || ''}
                  onChange={e => setForm(f => ({ ...f, routingKey: e.target.value }))}
                  placeholder="PagerDuty integration routing key"
                  className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
                />
              </div>
            )}
          </div>

          {/* Options */}
          <div className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-3">
            <div>
              <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Cooldown (minutes)</label>
              <input
                type="number"
                value={form.cooldownMinutes || 5}
                onChange={e => setForm(f => ({ ...f, cooldownMinutes: parseInt(e.target.value) || 5 }))}
                className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Severity Filter</label>
              <div className="flex flex-wrap gap-2 pt-1">
                {['critical', 'warning', 'info'].map(s => (
                  <label key={s} className="flex items-center gap-1 text-xs text-slate-600 dark:text-slate-400">
                    <input
                      type="checkbox"
                      checked={form.severities?.includes(s) ?? false}
                      onChange={e => {
                        setForm(f => ({
                          ...f,
                          severities: e.target.checked
                            ? [...(f.severities || []), s]
                            : (f.severities || []).filter(x => x !== s),
                        }))
                      }}
                      className="rounded"
                    />
                    {s}
                  </label>
                ))}
              </div>
            </div>
            <div className="flex items-end">
              <label className="flex items-center gap-2 text-sm text-slate-600 dark:text-slate-400">
                <input
                  type="checkbox"
                  checked={form.notifyOnResolve ?? true}
                  onChange={e => setForm(f => ({ ...f, notifyOnResolve: e.target.checked }))}
                  className="rounded"
                />
                Notify on resolve
              </label>
            </div>
          </div>

          {/* Smart Filters */}
          <div className="mt-4 rounded-lg border border-slate-200 p-4 dark:border-slate-700">
            <div className="mb-3 flex items-center gap-2 text-xs font-medium text-slate-700 dark:text-slate-300">
              <Filter className="h-3.5 w-3.5" /> Routing Filters
              <span className="text-slate-400 font-normal">(empty = send for all)</span>
            </div>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Health Checks</label>
                <div className="max-h-32 overflow-y-auto rounded border border-slate-200 p-2 dark:border-slate-700">
                  {(checks ?? []).map(c => (
                    <label key={c.id} className="flex items-center gap-2 py-0.5 text-xs text-slate-600 dark:text-slate-400">
                      <input
                        type="checkbox"
                        checked={form.checkIds?.includes(c.id) ?? false}
                        onChange={e => {
                          setForm(f => ({
                            ...f,
                            checkIds: e.target.checked
                              ? [...(f.checkIds || []), c.id]
                              : (f.checkIds || []).filter(x => x !== c.id),
                          }))
                        }}
                        className="rounded"
                      />
                      <span className="truncate">{c.name}</span>
                      <span className="ml-auto text-[10px] text-slate-400">{c.type}</span>
                    </label>
                  ))}
                  {!(checks?.length) && <p className="text-xs text-slate-400 italic">No checks configured</p>}
                </div>
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Check Types</label>
                <div className="flex flex-wrap gap-2 pt-1">
                  {['api', 'tcp', 'process', 'command', 'log', 'mysql', 'ssh'].map(t => (
                    <label key={t} className="flex items-center gap-1 text-xs text-slate-600 dark:text-slate-400">
                      <input
                        type="checkbox"
                        checked={form.checkTypes?.includes(t) ?? false}
                        onChange={e => {
                          setForm(f => ({
                            ...f,
                            checkTypes: e.target.checked
                              ? [...(f.checkTypes || []), t]
                              : (f.checkTypes || []).filter(x => x !== t),
                          }))
                        }}
                        className="rounded"
                      />
                      {t}
                    </label>
                  ))}
                </div>
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Servers</label>
                <input
                  value={(form.servers || []).join(', ')}
                  onChange={e => setForm(f => ({ ...f, servers: e.target.value.split(',').map(s => s.trim()).filter(Boolean) }))}
                  placeholder="production, staging"
                  className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
                />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Tags</label>
                <input
                  value={(form.tags || []).join(', ')}
                  onChange={e => setForm(f => ({ ...f, tags: e.target.value.split(',').map(s => s.trim()).filter(Boolean) }))}
                  placeholder="critical, database"
                  className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
                />
              </div>
            </div>
          </div>

          <div className="mt-4 flex gap-2">
            <button
              onClick={() => saveMutation.mutate(form)}
              disabled={saveMutation.isPending}
              className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {editId ? 'Update' : 'Create'}
            </button>
            <button
              onClick={() => { setShowForm(false); setEditId(null); setForm(emptyForm) }}
              className="rounded-lg border border-slate-300 px-4 py-2 text-sm text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-400"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Channel List */}
      {!channels?.length ? (
        <EmptyState icon={<Bell className="h-6 w-6" />} title="No channels configured" description="Add a notification channel to receive alerts via email, Slack, webhooks, and more" />
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {channels.map(ch => (
            <div key={ch.id} className="rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className={`flex h-9 w-9 items-center justify-center rounded-lg ${ch.enabled ? 'bg-blue-100 text-blue-600 dark:bg-blue-950/50 dark:text-blue-400' : 'bg-slate-100 text-slate-400 dark:bg-slate-800'}`}>
                    {channelIcon(ch.type)}
                  </div>
                  <div>
                    <h3 className="text-sm font-medium text-slate-900 dark:text-slate-100">{ch.name}</h3>
                    <p className="text-xs text-slate-500 capitalize">{ch.type}</p>
                  </div>
                </div>
                {isAdmin && (
                  <button
                    onClick={() => toggleMutation.mutate(ch.id)}
                    className="text-slate-400 hover:text-slate-600"
                    title={ch.enabled ? 'Disable' : 'Enable'}
                  >
                    {ch.enabled ? <ToggleRight className="h-5 w-5 text-blue-600" /> : <ToggleLeft className="h-5 w-5" />}
                  </button>
                )}
              </div>

              {/* Details */}
              <div className="mt-3 space-y-1 text-xs text-slate-500">
                {ch.webhookUrl && <p className="truncate">URL: {ch.webhookUrl}</p>}
                {ch.email && <p className="truncate">To: {ch.email}</p>}
                {ch.chatId && <p>Chat: {ch.chatId}</p>}
                {ch.severities && ch.severities.length > 0 && (
                  <p>Severities: {ch.severities.join(', ')}</p>
                )}
                {ch.checkIds && ch.checkIds.length > 0 && (
                  <p>Checks: {ch.checkIds.length} selected</p>
                )}
                {ch.checkTypes && ch.checkTypes.length > 0 && (
                  <p>Types: {ch.checkTypes.join(', ')}</p>
                )}
                {ch.servers && ch.servers.length > 0 && (
                  <p>Servers: {ch.servers.join(', ')}</p>
                )}
                {ch.tags && ch.tags.length > 0 && (
                  <p>Tags: {ch.tags.join(', ')}</p>
                )}
                {ch.cooldownMinutes ? <p>Cooldown: {ch.cooldownMinutes}min</p> : null}
              </div>

              {/* Actions */}
              {isAdmin && (
                <div className="mt-3 flex gap-1 border-t border-slate-100 pt-3 dark:border-slate-800">
                  <button
                    onClick={() => testMutation.mutate(ch.id)}
                    className="flex items-center gap-1 rounded px-2 py-1 text-xs text-slate-500 hover:bg-slate-50 hover:text-blue-600 dark:hover:bg-slate-800"
                    disabled={testMutation.isPending}
                  >
                    <Send className="h-3 w-3" /> Test
                  </button>
                  <button
                    onClick={() => {
                      setEditId(ch.id)
                      setShowForm(false)
                      setForm({ ...ch })
                    }}
                    className="flex items-center gap-1 rounded px-2 py-1 text-xs text-slate-500 hover:bg-slate-50 hover:text-blue-600 dark:hover:bg-slate-800"
                  >
                    <Pencil className="h-3 w-3" /> Edit
                  </button>
                  <button
                    onClick={() => {
                      if (confirm(`Delete channel "${ch.name}"?`)) deleteMutation.mutate(ch.id)
                    }}
                    className="flex items-center gap-1 rounded px-2 py-1 text-xs text-slate-500 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-950/50"
                  >
                    <Trash2 className="h-3 w-3" /> Delete
                  </button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
