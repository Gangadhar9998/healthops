import { api } from './client'
import type { MySQLHealthCard } from '@/types'

export interface MySQLAIResponse {
  answer: string
  suggestions?: string[]
  severity?: 'info' | 'warning' | 'critical'
  provider: string
  model: string
  answeredAt: string
}

export const mysqlApi = {
  health: () => api.get<MySQLHealthCard>('/mysql/health'),
  samples: (params?: { limit?: number }) => {
    const qs = params?.limit ? `?limit=${params.limit}` : ''
    return api.get<unknown[]>(`/mysql/samples${qs}`)
  },
  deltas: (params?: { limit?: number }) => {
    const qs = params?.limit ? `?limit=${params.limit}` : ''
    return api.get<unknown[]>(`/mysql/deltas${qs}`)
  },
  timeseries: (params?: { metric?: string; period?: string }) => {
    const qs = new URLSearchParams()
    if (params?.metric) qs.set('metric', params.metric)
    if (params?.period) qs.set('period', params.period)
    const q = qs.toString()
    return api.get<unknown[]>(`/mysql/timeseries${q ? `?${q}` : ''}`)
  },
  aiAsk: (question?: string, providerId?: string) =>
    api.post<MySQLAIResponse>('/mysql/ai/ask', { question: question || '', providerId: providerId || '' }),
}
