import { memo } from 'react'
import clsx from 'clsx'
import type { ConnectionState } from '../types'
import { useSSEContext } from '../context/SSEContext'

const labels: Record<ConnectionState, string> = {
  connected:    'Live',
  reconnecting: 'Reconnecting…',
  disconnected: 'Disconnected',
}

const dotClass: Record<ConnectionState, string> = {
  connected:    'bg-grid-success animate-pulse-dot',
  reconnecting: 'bg-grid-warning animate-pulse',
  disconnected: 'bg-grid-danger',
}

const textClass: Record<ConnectionState, string> = {
  connected:    'text-grid-success',
  reconnecting: 'text-grid-warning',
  disconnected: 'text-grid-danger',
}

interface SSEIndicatorProps {
  className?: string
}

export const SSEIndicator = memo(function SSEIndicator({
  className,
}: SSEIndicatorProps) {
  const { connectionState } = useSSEContext()

  return (
    <div
      className={clsx(
        'flex items-center gap-1.5 text-xs font-mono select-none',
        className,
      )}
      title={`SSE stream: ${connectionState}`}
    >
      <span
        className={clsx('h-2 w-2 rounded-full flex-shrink-0', dotClass[connectionState])}
      />
      <span className={clsx('hidden sm:inline', textClass[connectionState])}>
        {labels[connectionState]}
      </span>
    </div>
  )
})
