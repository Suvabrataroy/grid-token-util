import {
  memo,
  useRef,
  useState,
  useCallback,
  type ReactNode,
  type CSSProperties,
} from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import clsx from 'clsx'

export interface ColumnDef<T> {
  key: string
  header: string
  width?: string | number   // CSS width value e.g. '120px' or '20%'
  minWidth?: number
  sortable?: boolean
  render: (row: T, index: number) => ReactNode
}

interface VirtualTableProps<T> {
  data: T[]
  columns: ColumnDef<T>[]
  rowHeight?: number
  containerHeight?: number   // px; defaults to 400
  getRowKey: (row: T, index: number) => string
  onRowClick?: (row: T) => void
  emptyMessage?: string
  className?: string
}

type SortDir = 'asc' | 'desc' | null

function VirtualTableInner<T>({
  data,
  columns,
  rowHeight = 40,
  containerHeight = 400,
  getRowKey,
  onRowClick,
  emptyMessage = 'No data',
  className,
}: VirtualTableProps<T>) {
  const parentRef = useRef<HTMLDivElement>(null)
  const [sortKey, setSortKey] = useState<string | null>(null)
  const [sortDir, setSortDir] = useState<SortDir>(null)

  const handleSort = useCallback(
    (key: string) => {
      if (sortKey === key) {
        setSortDir((d) => (d === 'asc' ? 'desc' : d === 'desc' ? null : 'asc'))
        if (sortDir === 'desc') setSortKey(null)
      } else {
        setSortKey(key)
        setSortDir('asc')
      }
    },
    [sortKey, sortDir],
  )

  const sorted = [...data]
  if (sortKey && sortDir) {
    sorted.sort((a, b) => {
      const col = columns.find((c) => c.key === sortKey)
      if (!col) return 0
      // Attempt string comparison on rendered value; callers can override with
      // a comparator if needed.
      const av = String((a as Record<string, unknown>)[sortKey] ?? '')
      const bv = String((b as Record<string, unknown>)[sortKey] ?? '')
      const cmp = av.localeCompare(bv, undefined, { numeric: true })
      return sortDir === 'asc' ? cmp : -cmp
    })
  }

  const rowVirtualizer = useVirtualizer({
    count: sorted.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => rowHeight,
    overscan: 8,
  })

  if (sorted.length === 0) {
    return (
      <div
        className={clsx(
          'flex items-center justify-center text-grid-muted text-sm py-12',
          className,
        )}
      >
        {emptyMessage}
      </div>
    )
  }

  return (
    <div className={clsx('flex flex-col', className)}>
      {/* Header */}
      <div className="flex border-b border-grid-border bg-grid-bg sticky top-0 z-10">
        {columns.map((col) => {
          const isSorted = sortKey === col.key
          return (
            <div
              key={col.key}
              style={colStyle(col)}
              className={clsx(
                'px-3 py-2 text-xs font-semibold text-grid-muted uppercase tracking-wider truncate',
                col.sortable && 'cursor-pointer hover:text-white select-none',
                isSorted && 'text-grid-accent',
              )}
              onClick={() => col.sortable && handleSort(col.key)}
            >
              {col.header}
              {isSorted && (
                <span className="ml-1">{sortDir === 'asc' ? '↑' : '↓'}</span>
              )}
            </div>
          )
        })}
      </div>

      {/* Virtualized rows */}
      <div
        ref={parentRef}
        style={{ height: containerHeight, overflowY: 'auto' }}
        className="relative"
      >
        <div
          style={{ height: rowVirtualizer.getTotalSize(), position: 'relative' }}
        >
          {rowVirtualizer.getVirtualItems().map((vRow) => {
            const row = sorted[vRow.index]
            return (
              <div
                key={getRowKey(row, vRow.index)}
                data-index={vRow.index}
                ref={rowVirtualizer.measureElement}
                style={{
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  right: 0,
                  transform: `translateY(${vRow.start}px)`,
                }}
                className={clsx(
                  'flex items-center border-b border-grid-border/50 hover:bg-white/5 transition-colors',
                  onRowClick && 'cursor-pointer',
                )}
                onClick={() => onRowClick?.(row)}
              >
                {columns.map((col) => (
                  <div
                    key={col.key}
                    style={colStyle(col)}
                    className="px-3 py-2 text-sm text-gray-300 truncate"
                  >
                    {col.render(row, vRow.index)}
                  </div>
                ))}
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}

function colStyle<T>(col: ColumnDef<T>): CSSProperties {
  return {
    width: col.width ?? 'auto',
    minWidth: col.minWidth,
    flexShrink: col.width ? 0 : 1,
    flexGrow: col.width ? 0 : 1,
    overflow: 'hidden',
  }
}

export const VirtualTable = memo(VirtualTableInner) as typeof VirtualTableInner
