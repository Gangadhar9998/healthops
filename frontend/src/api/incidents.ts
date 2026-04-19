import { api } from './client'
import type { Incident, IncidentSnapshot } from '@/types'

export const incidentsApi = {
  list: (params?: { status?: string; severity?: string; limit?: number; offset?: number }) => {
    const qs = new URLSearchParams()
    if (params?.status) qs.set('status', params.status)
    if (params?.severity) qs.set('severity', params.severity)
    if (params?.limit) qs.set('limit', String(params.limit))
    if (params?.offset) qs.set('offset', String(params.offset))
    const q = qs.toString()
    return api.get<Incident[]>(`/incidents${q ? `?${q}` : ''}`)
  },
  get: (id: string) => api.get<Incident>(`/incidents/${encodeURIComponent(id)}`),
  acknowledge: (id: string) => api.post(`/incidents/${encodeURIComponent(id)}/acknowledge`),
  resolve: (id: string) => api.post(`/incidents/${encodeURIComponent(id)}/resolve`),
  snapshots: (id: string) => api.get<IncidentSnapshot[]>(`/incidents/${encodeURIComponent(id)}/snapshots`),
}
