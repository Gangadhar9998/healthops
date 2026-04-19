import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Users as UsersIcon, Plus, Trash2, Shield, Eye, Pencil } from 'lucide-react'
import { useAuth } from '@/hooks/useAuth'
import { LoadingState } from '@/components/LoadingState'
import { ErrorState } from '@/components/ErrorState'
import { EmptyState } from '@/components/EmptyState'
import { useToast } from '@/components/Toast'

interface User {
  id: string
  username: string
  role: 'admin' | 'ops'
  displayName?: string
  createdAt: string
  updatedAt: string
}

function authHeaders(): Record<string, string> {
  const token = localStorage.getItem('healthops_token')
  return token ? { Authorization: `Bearer ${token}` } : {}
}

async function fetchUsers(): Promise<User[]> {
  const res = await fetch('/api/v1/users', { headers: authHeaders() })
  if (!res.ok) throw new Error('Failed to load users')
  const body = await res.json()
  return body.data
}

export default function UserManagement() {
  const { isAdmin } = useAuth()
  const toast = useToast()
  const queryClient = useQueryClient()
  const [showForm, setShowForm] = useState(false)
  const [editId, setEditId] = useState<string | null>(null)
  const [form, setForm] = useState({ username: '', password: '', role: 'ops', displayName: '' })

  const { data: users, isLoading, error } = useQuery({ queryKey: ['users'], queryFn: fetchUsers })

  const createMutation = useMutation({
    mutationFn: async (data: typeof form) => {
      const res = await fetch('/api/v1/users', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...authHeaders() },
        body: JSON.stringify(data),
      })
      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        throw new Error(body?.error?.message || 'Failed to create user')
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setShowForm(false)
      setForm({ username: '', password: '', role: 'ops', displayName: '' })
      toast.success('User created')
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const updateMutation = useMutation({
    mutationFn: async ({ id, data }: { id: string; data: Record<string, string | undefined> }) => {
      const res = await fetch(`/api/v1/users/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', ...authHeaders() },
        body: JSON.stringify(data),
      })
      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        throw new Error(body?.error?.message || 'Failed to update user')
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setEditId(null)
      setForm({ username: '', password: '', role: 'ops', displayName: '' })
      toast.success('User updated')
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const deleteMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`/api/v1/users/${id}`, {
        method: 'DELETE',
        headers: authHeaders(),
      })
      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        throw new Error(body?.error?.message || 'Failed to delete user')
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      toast.success('User deleted')
    },
    onError: (err: Error) => toast.error(err.message),
  })

  if (isLoading) return <LoadingState message="Loading users…" />
  if (error) return <ErrorState message="Failed to load users" retry={() => { queryClient.invalidateQueries({ queryKey: ['users'] }) }} />

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold text-slate-900 dark:text-slate-100">User Management</h1>
          <p className="text-sm text-slate-500">Manage user accounts and roles</p>
        </div>
        {isAdmin && (
          <button
            onClick={() => { setShowForm(true); setEditId(null); setForm({ username: '', password: '', role: 'ops', displayName: '' }) }}
            className="flex items-center gap-2 rounded-lg bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700"
          >
            <Plus className="h-4 w-4" /> Add User
          </button>
        )}
      </div>

      {/* Create/Edit Form */}
      {(showForm || editId) && isAdmin && (
        <div className="rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
          <h3 className="mb-3 text-sm font-medium text-slate-900 dark:text-slate-100">
            {editId ? 'Edit User' : 'Create New User'}
          </h3>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
            {!editId && (
              <input
                placeholder="Username"
                value={form.username}
                onChange={e => setForm(f => ({ ...f, username: e.target.value }))}
                className="rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
              />
            )}
            <input
              placeholder={editId ? 'New password (optional)' : 'Password'}
              type="password"
              value={form.password}
              onChange={e => setForm(f => ({ ...f, password: e.target.value }))}
              className="rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
            />
            <select
              value={form.role}
              onChange={e => setForm(f => ({ ...f, role: e.target.value }))}
              className="rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
            >
              <option value="ops">Ops (View Only)</option>
              <option value="admin">Admin (Full Access)</option>
            </select>
            <input
              placeholder="Display Name"
              value={form.displayName}
              onChange={e => setForm(f => ({ ...f, displayName: e.target.value }))}
              className="rounded-lg border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
            />
          </div>
          <div className="mt-3 flex gap-2">
            <button
              onClick={() => {
                if (editId) {
                  const data: Record<string, string | undefined> = {
                    role: form.role,
                    displayName: form.displayName,
                  }
                  if (form.password) data.password = form.password
                  updateMutation.mutate({ id: editId, data })
                } else {
                  createMutation.mutate(form)
                }
              }}
              disabled={createMutation.isPending || updateMutation.isPending}
              className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {editId ? 'Update' : 'Create'}
            </button>
            <button
              onClick={() => { setShowForm(false); setEditId(null) }}
              className="rounded-lg border border-slate-300 px-4 py-2 text-sm text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-400"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Users List */}
      {!users?.length ? (
        <EmptyState icon={<UsersIcon className="h-6 w-6" />} title="No users" description="No user accounts found" />
      ) : (
        <div className="overflow-hidden rounded-lg border border-slate-200 dark:border-slate-800">
          <table className="w-full">
            <thead className="bg-slate-50 dark:bg-slate-800/50">
              <tr>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase text-slate-500">User</th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase text-slate-500">Role</th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase text-slate-500">Created</th>
                {isAdmin && (
                  <th className="px-4 py-3 text-right text-xs font-medium uppercase text-slate-500">Actions</th>
                )}
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-200 bg-white dark:divide-slate-800 dark:bg-slate-900">
              {users.map(user => (
                <tr key={user.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/50">
                  <td className="px-4 py-3">
                    <div className="text-sm font-medium text-slate-900 dark:text-slate-100">{user.displayName || user.username}</div>
                    <div className="text-xs text-slate-500">@{user.username}</div>
                  </td>
                  <td className="px-4 py-3">
                    <span className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ${
                      user.role === 'admin'
                        ? 'bg-purple-100 text-purple-700 dark:bg-purple-950/50 dark:text-purple-400'
                        : 'bg-blue-100 text-blue-700 dark:bg-blue-950/50 dark:text-blue-400'
                    }`}>
                      {user.role === 'admin' ? <Shield className="h-3 w-3" /> : <Eye className="h-3 w-3" />}
                      {user.role === 'admin' ? 'Admin' : 'Ops'}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-sm text-slate-500">
                    {new Date(user.createdAt).toLocaleDateString()}
                  </td>
                  {isAdmin && (
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <button
                          onClick={() => {
                            setEditId(user.id)
                            setShowForm(false)
                            setForm({
                              username: user.username,
                              password: '',
                              role: user.role,
                              displayName: user.displayName || '',
                            })
                          }}
                          className="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-600 dark:hover:bg-slate-800"
                          title="Edit"
                        >
                          <Pencil className="h-4 w-4" />
                        </button>
                        <button
                          onClick={() => {
                            if (confirm(`Delete user "${user.username}"?`)) {
                              deleteMutation.mutate(user.id)
                            }
                          }}
                          className="rounded p-1 text-slate-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-950/50"
                          title="Delete"
                        >
                          <Trash2 className="h-4 w-4" />
                        </button>
                      </div>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
