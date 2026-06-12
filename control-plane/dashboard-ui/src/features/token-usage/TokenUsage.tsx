import { memo, useMemo } from 'react'
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  Legend,
  ResponsiveContainer,
  CartesianGrid,
} from 'recharts'
import { Cpu } from 'lucide-react'
import type { TokenUsage as TokenUsageType } from '../../types'
import { StatCard } from '../../components/StatCard'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function fmtTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`
  return String(n)
}

const AGENT_COLORS = [
  '#58a6ff', '#3fb950', '#d29922', '#f85149',
  '#bc8cff', '#ff7b72', '#79c0ff', '#56d364',
]

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface TokenUsageProps {
  data: TokenUsageType
}

export const TokenUsage = memo(function TokenUsage({ data }: TokenUsageProps) {
  const agents = useMemo(() => {
    const set = new Set<string>()
    data.daily.forEach((d) => set.add(d.agent))
    return Array.from(set)
  }, [data.daily])

  // Group daily data by date, one entry per date with per-agent totals
  const chartData = useMemo(() => {
    const byDate = new Map<string, Record<string, number>>()
    for (const row of data.daily) {
      const entry = byDate.get(row.date) ?? {}
      entry[row.agent] = (entry[row.agent] ?? 0) + row.total_tokens
      byDate.set(row.date, entry)
    }
    return Array.from(byDate.entries())
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([date, vals]) => ({ date: date.slice(5), ...vals }))
  }, [data.daily])

  return (
    <div className="flex flex-col gap-4">
      {/* Monthly totals */}
      <div className="grid grid-cols-3 gap-3">
        <StatCard
          title="Input Tokens"
          value={fmtTokens(data.month_input)}
          subtitle="This month"
          colorClass="text-blue-400"
        />
        <StatCard
          title="Output Tokens"
          value={fmtTokens(data.month_output)}
          subtitle="This month"
          colorClass="text-green-400"
        />
        <StatCard
          title="Total Tokens"
          value={fmtTokens(data.month_total)}
          subtitle="This month"
          icon={<Cpu size={16} />}
          colorClass="text-grid-accent"
        />
      </div>

      {/* Bar chart */}
      <div className="bg-grid-surface border border-grid-border rounded-lg p-4">
        <h3 className="text-sm font-semibold text-white mb-4">
          Daily Token Consumption by Agent
        </h3>
        {chartData.length === 0 ? (
          <p className="text-grid-muted text-sm text-center py-8">
            No token data available
          </p>
        ) : (
          <ResponsiveContainer width="100%" height={240}>
            <BarChart data={chartData} margin={{ top: 0, right: 8, left: 0, bottom: 0 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#30363d" />
              <XAxis
                dataKey="date"
                tick={{ fill: '#8b949e', fontSize: 11 }}
                axisLine={{ stroke: '#30363d' }}
                tickLine={false}
              />
              <YAxis
                tick={{ fill: '#8b949e', fontSize: 11 }}
                axisLine={{ stroke: '#30363d' }}
                tickLine={false}
                tickFormatter={fmtTokens}
                width={52}
              />
              <Tooltip
                contentStyle={{
                  background: '#161b22',
                  border: '1px solid #30363d',
                  borderRadius: 6,
                  fontSize: 12,
                }}
                labelStyle={{ color: '#e6edf3' }}
                formatter={(value: number) => fmtTokens(value)}
              />
              <Legend
                wrapperStyle={{ fontSize: 11, color: '#8b949e' }}
                iconType="square"
              />
              {agents.map((agent, i) => (
                <Bar
                  key={agent}
                  dataKey={agent}
                  stackId="tokens"
                  fill={AGENT_COLORS[i % AGENT_COLORS.length]}
                />
              ))}
            </BarChart>
          </ResponsiveContainer>
        )}
      </div>

      {/* Per-org breakdown */}
      <div className="bg-grid-surface border border-grid-border rounded-lg overflow-hidden">
        <div className="px-4 py-3 border-b border-grid-border">
          <h3 className="text-sm font-semibold text-white">Per-Org Breakdown</h3>
        </div>
        <table className="w-full text-sm">
          <thead>
            <tr className="text-xs text-grid-muted uppercase tracking-wider border-b border-grid-border">
              <th className="px-4 py-2 text-left">Org</th>
              <th className="px-4 py-2 text-right">Input</th>
              <th className="px-4 py-2 text-right">Output</th>
              <th className="px-4 py-2 text-right">Total</th>
            </tr>
          </thead>
          <tbody>
            {data.by_org.length === 0 ? (
              <tr>
                <td colSpan={4} className="px-4 py-6 text-center text-grid-muted">
                  No data
                </td>
              </tr>
            ) : (
              data.by_org.map((org) => (
                <tr
                  key={org.org_id}
                  className="border-b border-grid-border/40 hover:bg-white/5 transition-colors"
                >
                  <td className="px-4 py-2 text-gray-300">{org.org_name}</td>
                  <td className="px-4 py-2 text-right font-mono text-blue-400">
                    {fmtTokens(org.input_tokens)}
                  </td>
                  <td className="px-4 py-2 text-right font-mono text-green-400">
                    {fmtTokens(org.output_tokens)}
                  </td>
                  <td className="px-4 py-2 text-right font-mono text-grid-accent font-semibold">
                    {fmtTokens(org.total_tokens)}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
})
