import { api } from './client'
import type { AIServiceConfig, AIProviderConfig, AIPromptTemplate, AIAnalysisResult } from '@/types'

export const aiApi = {
  config: () => api.get<AIServiceConfig>('/ai/config'),
  updateConfig: (data: Partial<AIServiceConfig>) => api.put<AIServiceConfig>('/ai/config', data),
  providers: () => api.get<AIProviderConfig[]>('/ai/providers'),
  addProvider: (p: Partial<AIProviderConfig>) => api.post<AIProviderConfig>('/ai/providers', p),
  updateProvider: (id: string, p: Partial<AIProviderConfig>) => api.put<AIProviderConfig>(`/ai/providers/${encodeURIComponent(id)}`, p),
  deleteProvider: (id: string) => api.delete(`/ai/providers/${encodeURIComponent(id)}`),
  prompts: () => api.get<AIPromptTemplate[]>('/ai/prompts'),
  addPrompt: (p: Partial<AIPromptTemplate>) => api.post<AIPromptTemplate>('/ai/prompts', p),
  updatePrompt: (id: string, p: Partial<AIPromptTemplate>) => api.put<AIPromptTemplate>(`/ai/prompts/${encodeURIComponent(id)}`, p),
  deletePrompt: (id: string) => api.delete(`/ai/prompts/${encodeURIComponent(id)}`),
  analyze: (incidentId: string) => api.post<AIAnalysisResult>(`/ai/analyze/${encodeURIComponent(incidentId)}`),
  health: () => api.get<unknown>('/ai/health'),
  providerHealth: (providerId: string) => api.get<unknown>(`/ai/providers/${encodeURIComponent(providerId)}/health`),
  results: (incidentId: string) => api.get<AIAnalysisResult>(`/ai/results/${encodeURIComponent(incidentId)}`),
  allResults: () => api.get<AIAnalysisResult[]>('/ai/results'),
}
