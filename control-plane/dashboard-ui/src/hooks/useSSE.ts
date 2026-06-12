import { useCallback, useEffect, useRef, useState } from 'react'
import type { SSEEvent, ConnectionState } from '../types'

const MIN_BACKOFF_MS = 1_000
const MAX_BACKOFF_MS = 30_000

interface UseSSEResult {
  events: SSEEvent[]
  connectionState: ConnectionState
  lastEventId: string | null
}

export function useSSE(url: string): UseSSEResult {
  const [events, setEvents] = useState<SSEEvent[]>([])
  const [connectionState, setConnectionState] =
    useState<ConnectionState>('disconnected')
  const [lastEventId, setLastEventId] = useState<string | null>(null)

  const esRef = useRef<EventSource | null>(null)
  const backoffRef = useRef<number>(MIN_BACKOFF_MS)
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const lastEventIdRef = useRef<string | null>(null)

  const connect = useCallback(() => {
    if (esRef.current) {
      esRef.current.close()
    }

    const fullUrl = lastEventIdRef.current
      ? `${url}?lastEventID=${encodeURIComponent(lastEventIdRef.current)}`
      : url

    const es = new EventSource(fullUrl)
    esRef.current = es
    setConnectionState('reconnecting')

    es.onopen = () => {
      setConnectionState('connected')
      backoffRef.current = MIN_BACKOFF_MS
    }

    es.onmessage = (e: MessageEvent<string>) => {
      if (e.lastEventId) {
        lastEventIdRef.current = e.lastEventId
        setLastEventId(e.lastEventId)
      }
      try {
        const parsed = JSON.parse(e.data) as SSEEvent
        setEvents((prev) => {
          // keep last 500 events in memory
          const next = [...prev, parsed]
          return next.length > 500 ? next.slice(next.length - 500) : next
        })
      } catch {
        // ignore unparseable frames
      }
    }

    es.onerror = () => {
      es.close()
      esRef.current = null
      setConnectionState('disconnected')

      const delay = backoffRef.current
      backoffRef.current = Math.min(delay * 2, MAX_BACKOFF_MS)
      setConnectionState('reconnecting')

      retryTimerRef.current = setTimeout(() => {
        connect()
      }, delay)
    }
  }, [url])

  useEffect(() => {
    connect()

    return () => {
      if (retryTimerRef.current) clearTimeout(retryTimerRef.current)
      if (esRef.current) {
        esRef.current.close()
        esRef.current = null
      }
    }
  }, [connect])

  return { events, connectionState, lastEventId }
}
