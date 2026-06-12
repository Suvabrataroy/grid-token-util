import { memo, type ReactNode } from 'react'
import clsx from 'clsx'

interface StatCardProps {
  title: string
  value: string | number
  subtitle?: string
  icon?: ReactNode
  colorClass?: string        // e.g. 'text-grid-success'
  className?: string
}

export const StatCard = memo(function StatCard({
  title,
  value,
  subtitle,
  icon,
  colorClass = 'text-white',
  className,
}: StatCardProps) {
  return (
    <div
      className={clsx(
        'bg-grid-surface border border-grid-border rounded-lg p-4 flex flex-col gap-1',
        className,
      )}
    >
      <div className="flex items-center justify-between">
        <span className="text-xs text-grid-muted uppercase tracking-widest">
          {title}
        </span>
        {icon && (
          <span className={clsx('opacity-70', colorClass)}>{icon}</span>
        )}
      </div>
      <span className={clsx('text-2xl font-bold tabular-nums', colorClass)}>
        {value}
      </span>
      {subtitle && (
        <span className="text-xs text-grid-muted">{subtitle}</span>
      )}
    </div>
  )
})
