import { memo, useState, useEffect } from 'react'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  CartesianGrid,
} from 'recharts'
import clsx from 'clsx'
import { Database, Layers, Activity, GitCommit, CheckCircle, XCircle } from 'lucide-react'
import type { SystemMetrics } from '../../types'
import { useSSESubscription } from '../../context/SSEContext'
import { format } from 'date-fns'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function HealthPill({
  label,
  healthy,
  value,
}: {
  label: string
  healthy: boolean
  value?: string
}) {
  return (
    <div className="flex items-center justify-between px-3 py-2 bg-grid-bg rounded border border-grid-border">
      <div className="flex items-center gap-2">
        {healthy ? (
          <CheckCircle size={14} className="text-grid-success" />
        ) : (
          <XCircle size={14} className="text-grid-danger" />
        )}
        <span className="text-xs text-gray-300">{label}</span>
      </div>
      {value && (
        <span className="text-xs font-mono text-grid-muted">{value}</span>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface SystemStatusProps {
  initial: SystemMetrics
}

interface ResponsePoint {
  t: string
  p50: number
  p95: number
  p99: number
}

export const SystemStatus = memo(function SystemStatus({
  initial,
}: SystemStatusProps) {
  const [metrics, setMetrics] = useState<SystemMetrics>(initial)
  const [history, setHistory] = useState<ResponsePoint[]>([
    {
      t: format(new Date(initial.ts), 'HH:mm'),
      p50: initial.response_time_p50_ms,
      p95: initial.response_time_p95_ms,
      p99: initial.response_time_p99_ms,
    },
  ])

  useSSESubscription('system_metrics', (event) => {
    const m = event.payload
    setMetrics(m)
    setHistory((prev) => {
      const point: ResponsePoint = {
        t: format(new Date(m.ts), 'HH:mm'),
        p50: m.response_time_p50_ms,
        p95: m.response_time_p95_ms,
        p99: m.response_time_p99_ms,
      }
      // Keep 24h of data at 1-min intervals ≈ 1440 points; cap at 288 (~12h)
      const next = [...prev, point]
      return next.length > 288 ? next.slice(next.length - 288) : next
    })
  })

  return (
    <div className="flex flex-col gap-4">
      {/* Health indicators */}
      <div className="bg-grid-surface border border-grid-border rounded-lg p-4">
        <div className="flex items-center gap-2 mb-3">
          <Layers size={15} className="text-grid-muted" />
          <h3 className="text-sm font-semibold text-white">Control Plane Health</h3>
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
          <HealthPill
            label="PostgreSQL"
            healthy={metrics.db_latency_ms < 100}
            value={`${metrics.db_latency_ms}ms`}
          />
          <HealthPill
            label="Redis"
            healthy={metrics.redis_latency_ms < 20}
            value={`${metrics.redis_latency_ms}ms`}
          />
          <HealthPill
            label="Scheduler"
            healthy={metrics.scheduler_running}
          />
          <HealthPill
            label="Queue Depth"
            healthy={metrics.queue_depth < 500}
            value={String(metrics.queue_depth)}
          />
        </div>

        {/* Version */}
        <div className="mt-3 flex items-center gap-3 text-xs text-grid-muted">
          <GitCommit size={12} />
          <span>{metrics.version}</span>
          <span className="font-mono">{metrics.build_sha.slice(0, 8)}</span>
        </div>
      </div>

      {/* Response time graph */}
      <div className="bg-grid-surface border border-grid-border rounded-lg p-4">
        <div className="flex items-center gap-2 mb-4">
          <Activity size={15} className="text-grid-muted" />
          <h3 className="text-sm font-semibold text-white">Response Time (last 24h)</h3>
          <span className="ml-auto text-xs text-grid-muted">ms</span>
        </div>
        <ResponsiveContainer width="100%" height={180}>
          <LineChart data={history} margin={{ top: 0, right: 8, left: 0, bottom: 0 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="#30363d" />
            <XAxis
              dataKey="t"
              tick={{ fill: '#8b949e', fontSize: 10 }}
              axisLine={{ stroke: '#30363d' }}
              tickLine={false}
              interval="preserveStartEnd"
            />
            <YAxis
              tick={{ fill: '#8b949e', fontSize: 10 }}
              axisLine={{ stroke: '#30363d' }}
              tickLine={false}
              width={36}
            />
            <Tooltip
              contentStyle={{
                background: '#161b22',
                border: '1px solid #30363d',
                borderRadius: 6,
                fontSize: 11,
              }}
              labelStyle={{ color: '#e6edf3' }}
              formatter={(v: number) => `${v}ms`}
            />
            <Line
              type="monotone"
              dataKey="p50"
              name="p50"
              stroke="#3fb950"
              dot={false}
              strokeWidth={1.5}
            />
            <Line
              type="monotone"
              dataKey="p95"
              name="p95"
              stroke="#d29922"
              dot={false}
              strokeWidth={1.5}
            />
            <Line
              type="monotone"
              dataKey="p99"
              name="p99"
              stroke="#f85149"
              dot={false}
              strokeWidth={1.5}
            />
          </LineChart>
        </ResponsiveContainer>
        <div className="flex gap-4 mt-2 text-xs text-grid-muted justify-end">
          <span className="flex items-center gap-1">
            <span className="h-0.5 w-4 bg-green-400 inline-block" />p50
          </span>
          <span className="flex items-center gap-1">
            <span className="h-0.5 w-4 bg-yellow-400 inline-block" />p95
          </span>
          <span className="flex items-center gap-1">
            <span className="h-0.5 w-4 bg-red-400 inline-block" />p99
          </span>
        </div>
      </div>
    </div>
  )
})
