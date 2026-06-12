import { memo, Suspense, lazy, useState, type ReactNode } from 'react'
import { Routes, Route, NavLink, useLocation } from 'react-router-dom'
import clsx from 'clsx'
import {
  LayoutDashboard,
  Monitor,
  ClipboardList,
  ScrollText,
  Award,
  ShieldAlert,
  ChevronLeft,
  ChevronRight,
  Cpu,
  Loader2,
} from 'lucide-react'
import { SSEIndicator } from './components/SSEIndicator'

// ---------------------------------------------------------------------------
// Lazy page imports
// ---------------------------------------------------------------------------

const DashboardPage  = lazy(() => import('./pages/DashboardPage'))
const WorkersPage    = lazy(() => import('./pages/WorkersPage'))
const TasksPage      = lazy(() => import('./pages/TasksPage'))
const AuditPage      = lazy(() => import('./pages/AuditPage'))
const BrowniePage    = lazy(() => import('./pages/BrowniePage'))
const SecurityPage   = lazy(() => import('./pages/SecurityPage'))

// ---------------------------------------------------------------------------
// Sidebar nav config
// ---------------------------------------------------------------------------

interface NavItem {
  to: string
  label: string
  icon: ReactNode
  exact?: boolean
}

const NAV_ITEMS: NavItem[] = [
  { to: '/',         label: 'Dashboard', icon: <LayoutDashboard size={18} />, exact: true },
  { to: '/workers',  label: 'Workers',   icon: <Monitor size={18} /> },
  { to: '/tasks',    label: 'Tasks',     icon: <ClipboardList size={18} /> },
  { to: '/audit',    label: 'Audit Log', icon: <ScrollText size={18} /> },
  { to: '/brownie',  label: 'Brownie',   icon: <Award size={18} /> },
  { to: '/security', label: 'Security',  icon: <ShieldAlert size={18} /> },
]

// ---------------------------------------------------------------------------
// Sidebar
// ---------------------------------------------------------------------------

interface SidebarProps {
  collapsed: boolean
  onToggle: () => void
}

const Sidebar = memo(function Sidebar({ collapsed, onToggle }: SidebarProps) {
  return (
    <aside
      className={clsx(
        'flex flex-col bg-grid-surface border-r border-grid-border transition-all duration-200 flex-shrink-0',
        collapsed ? 'w-14' : 'w-52',
      )}
    >
      {/* Logo */}
      <div className="flex items-center gap-2.5 px-4 py-4 border-b border-grid-border h-14">
        <Cpu size={20} className="text-grid-accent flex-shrink-0" />
        {!collapsed && (
          <span className="font-bold text-sm text-white tracking-wide truncate">
            Grid Control
          </span>
        )}
      </div>

      {/* Nav links */}
      <nav className="flex-1 py-3 overflow-y-auto">
        {NAV_ITEMS.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.exact}
            className={({ isActive }) =>
              clsx(
                'flex items-center gap-3 px-4 py-2.5 mx-2 rounded-md text-sm transition-colors',
                isActive
                  ? 'bg-grid-accent/10 text-grid-accent'
                  : 'text-grid-muted hover:text-white hover:bg-white/5',
              )
            }
          >
            <span className="flex-shrink-0">{item.icon}</span>
            {!collapsed && <span className="truncate">{item.label}</span>}
          </NavLink>
        ))}
      </nav>

      {/* Collapse toggle */}
      <button
        onClick={onToggle}
        className="flex items-center justify-center py-3 border-t border-grid-border text-grid-muted hover:text-white transition-colors"
        title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
      >
        {collapsed ? <ChevronRight size={16} /> : <ChevronLeft size={16} />}
      </button>
    </aside>
  )
})

// ---------------------------------------------------------------------------
// Loading fallback
// ---------------------------------------------------------------------------

function PageLoader() {
  return (
    <div className="flex items-center justify-center h-64 text-grid-muted">
      <Loader2 size={24} className="animate-spin mr-2" />
      <span className="text-sm">Loading…</span>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Page title helper
// ---------------------------------------------------------------------------

const PAGE_TITLES: Record<string, string> = {
  '/':         'Dashboard',
  '/workers':  'Workers',
  '/tasks':    'Tasks',
  '/audit':    'Audit Log',
  '/brownie':  'Brownie Points',
  '/security': 'Security Events',
}

// ---------------------------------------------------------------------------
// App root
// ---------------------------------------------------------------------------

export default function App() {
  const [collapsed, setCollapsed] = useState(false)
  const { pathname } = useLocation()
  const title = PAGE_TITLES[pathname] ?? 'Grid Control'

  return (
    <div className="flex h-screen overflow-hidden bg-grid-bg dark">
      <Sidebar collapsed={collapsed} onToggle={() => setCollapsed((c) => !c)} />

      {/* Main area */}
      <div className="flex flex-col flex-1 overflow-hidden">
        {/* Top bar */}
        <header className="h-14 flex items-center px-6 border-b border-grid-border bg-grid-surface flex-shrink-0">
          <h1 className="text-sm font-semibold text-white">{title}</h1>
          <div className="ml-auto flex items-center gap-4">
            <SSEIndicator />
          </div>
        </header>

        {/* Page content */}
        <main className="flex-1 overflow-y-auto p-6">
          <Suspense fallback={<PageLoader />}>
            <Routes>
              <Route path="/"         element={<DashboardPage />} />
              <Route path="/workers"  element={<WorkersPage />} />
              <Route path="/tasks"    element={<TasksPage />} />
              <Route path="/audit"    element={<AuditPage />} />
              <Route path="/brownie"  element={<BrowniePage />} />
              <Route path="/security" element={<SecurityPage />} />
            </Routes>
          </Suspense>
        </main>
      </div>
    </div>
  )
}
