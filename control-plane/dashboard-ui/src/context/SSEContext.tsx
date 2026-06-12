import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from 'react'
import type {
  SSEEvent,
  SSETaskUpdate,
  SSEWorkerUpdate,
  SSESecurityEvent,
  SSEBrownieUpdate,
  SSESystemMetrics,
  ConnectionState,
} from '../types'
import { useSSE } from '../hooks/useSSE'

// ---------------------------------------------------------------------------
// Subscriber registry – consumers register callbacks by event type
// ---------------------------------------------------------------------------

type EventTypeMap = {
  task_update: SSETaskUpdate
  worker_update: SSEWorkerUpdate
  security_event: SSESecurityEvent
  brownie_update: SSEBrownieUpdate
  system_metrics: SSESystemMetrics
}

type SubscriberMap = {
  [K in keyof EventTypeMap]: Set<(event: EventTypeMap[K]) => void>
}

interface SSEContextValue {
  connectionState: ConnectionState
  lastEventId: string | null
  subscribe<K extends keyof EventTypeMap>(
    type: K,
    handler: (event: EventTypeMap[K]) => void,
  ): () => void
  latestEvents: SSEEvent[]
}

const SSEContext = createContext<SSEContextValue | null>(null)

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

export function SSEProvider({ children }: { children: React.ReactNode }) {
  const { events, connectionState, lastEventId } = useSSE('/api/v1/events/stream')

  const subscribersRef = useRef<SubscriberMap>({
    task_update: new Set(),
    worker_update: new Set(),
    security_event: new Set(),
    brownie_update: new Set(),
    system_metrics: new Set(),
  })

  // Dispatch each new event to registered subscribers
  const processedCountRef = useRef(0)
  useEffect(() => {
    const unprocessed = events.slice(processedCountRef.current)
    processedCountRef.current = events.length

    for (const event of unprocessed) {
      const handlers = subscribersRef.current[event.type] as Set<
        (e: SSEEvent) => void
      >
      handlers.forEach((h) => h(event))
    }
  }, [events])

  const subscribe = useCallback(
    <K extends keyof EventTypeMap>(
      type: K,
      handler: (event: EventTypeMap[K]) => void,
    ) => {
      const set = subscribersRef.current[type] as Set<
        (event: EventTypeMap[K]) => void
      >
      set.add(handler)
      return () => set.delete(handler)
    },
    [],
  )

  const [latestEvents, setLatestEvents] = useState<SSEEvent[]>([])
  useEffect(() => {
    setLatestEvents(events.slice(-50))
  }, [events])

  return (
    <SSEContext.Provider
      value={{ connectionState, lastEventId, subscribe, latestEvents }}
    >
      {children}
    </SSEContext.Provider>
  )
}

// ---------------------------------------------------------------------------
// Consumers
// ---------------------------------------------------------------------------

export function useSSEContext(): SSEContextValue {
  const ctx = useContext(SSEContext)
  if (!ctx) throw new Error('useSSEContext must be used within SSEProvider')
  return ctx
}

export function useSSESubscription<K extends keyof EventTypeMap>(
  type: K,
  handler: (event: EventTypeMap[K]) => void,
) {
  const { subscribe } = useSSEContext()
  const handlerRef = useRef(handler)
  handlerRef.current = handler

  useEffect(() => {
    return subscribe(type, (e) => handlerRef.current(e))
  }, [subscribe, type])
}
