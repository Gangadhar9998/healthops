import { API_BASE } from '@/lib/constants'
import type { APIResponse } from '@/types'

class APIError extends Error {
  constructor(public code: number, message: string) {
    super(message)
    this.name = 'APIError'
  }
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const url = `${API_BASE}${path}`
  const res = await fetch(url, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers,
    },
  })

  if (!res.ok) {
    let message = res.statusText
    try {
      const body = await res.json() as APIResponse
      if (body.error?.message) message = body.error.message
    } catch { /* ignore parse errors */ }
    throw new APIError(res.status, message)
  }

  const body = await res.json() as APIResponse<T>
  if (!body.success && body.error) {
    throw new APIError(body.error.code, body.error.message)
  }

  return body.data as T
}

export const api = {
  get: <T>(path: string) => request<T>(path),

  post: <T>(path: string, data?: unknown) =>
    request<T>(path, { method: 'POST', body: data ? JSON.stringify(data) : undefined }),

  put: <T>(path: string, data?: unknown) =>
    request<T>(path, { method: 'PUT', body: data ? JSON.stringify(data) : undefined }),

  delete: <T>(path: string) =>
    request<T>(path, { method: 'DELETE' }),
}

export { APIError }
