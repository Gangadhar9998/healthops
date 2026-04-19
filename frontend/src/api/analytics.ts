import { api } from './client'
import type { UptimeStats, ResponseTimeBucket, FailureRateEntry, IncidentStats, StatusTimelineEntry } from '@/types'

export const analyticsApi = {
  uptime: (params?: { period?: string; checkId?: string }) => {
    const qs = new URLSearchParams()
    if (params?.period) qs.set('period', params.period)
    if (params?.checkId) qs.set('checkId', params.checkId)
    const q = qs.toString()
    return api.get<UptimeStats[]>(`/analytics/uptime${q ? `?${q}` : ''}`)
  },
  responseTimes: (params?: { checkId?: string; period?: string; interval?: string }) => {
    const qs = new URLSearchParams()
    if (params?.checkId) qs.set('checkId', params.checkId)
    if (params?.period) qs.set('period', params.period)
    if (params?.interval) qs.set('interval', params.interval)
    const q = qs.toString()
    return api.get<ResponseTimeBucket[]>(`/analytics/response-times${q ? `?${q}` : ''}`)
  },
  statusTimeline: (params?: { checkId?: string; period?: string }) => {
    const qs = new URLSearchParams()
    if (params?.checkId) qs.set('checkId', params.checkId)
    if (params?.period) qs.set('period', params.period)
    const q = qs.toString()
    return api.get<StatusTimelineEntry[]>(`/analytics/status-timeline${q ? `?${q}` : ''}`)
  },
  failureRate: (params?: { period?: string; interval?: string; groupBy?: string }) => {
    const qs = new URLSearchParams()
    if (params?.period) qs.set('period', params.period)
    if (params?.interval) qs.set('interval', params.interval)
    if (params?.groupBy) qs.set('groupBy', params.groupBy)
    const q = qs.toString()
    return api.get<FailureRateEntry[]>(`/analytics/failure-rate${q ? `?${q}` : ''}`)
  },
  incidents: () => api.get<IncidentStats>('/analytics/incidents'),
}
