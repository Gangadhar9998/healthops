import { useEffect, useRef, useCallback, useState } from 'react'
import type { SSEPayload } from '@/types'
import { SSE_RECONNECT_DELAY } from '@/lib/constants'

export function useSSE(onMessage: (payload: SSEPayload) => void) {
  const [connected, setConnected] = useState(false)
  const esRef = useRef<EventSource | null>(null)
  const onMessageRef = useRef(onMessage)
  onMessageRef.current = onMessage

  const connect = useCallback(() => {
    if (esRef.current) esRef.current.close()

    const es = new EventSource('/api/v1/events')
    esRef.current = es

    es.onopen = () => setConnected(true)

    es.onmessage = (event) => {
      try {
        const payload = JSON.parse(event.data) as SSEPayload
        onMessageRef.current(payload)
      } catch { /* ignore malformed events */ }
    }

    es.onerror = () => {
      setConnected(false)
      es.close()
      setTimeout(connect, SSE_RECONNECT_DELAY)
    }
  }, [])

  useEffect(() => {
    connect()
    return () => {
      esRef.current?.close()
    }
  }, [connect])

  return { connected }
}
