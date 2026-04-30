import { Bot, ChevronRight, LayoutDashboard, Settings2, TextSearch } from 'lucide-react'
import type { DashboardState } from '../types'
import type { Page } from '../lib/dashboard'

type Props = {
  page: Page
  data: DashboardState
}

const navItems: Array<{ key: Page; href: string; label: string; Icon: typeof LayoutDashboard }> = [
  { key: 'dashboard', href: '#/', label: 'Dashboard', Icon: LayoutDashboard },
  { key: 'settings', href: '#/settings', label: 'Settings', Icon: Settings2 },
  { key: 'logs', href: '#/logs', label: 'Logs', Icon: TextSearch },
]

export function AppTopbar({ page, data: _data }: Props) {
  return (
    <aside className="app-sidebar">
      <div className="sidebar-brand">
        <div className="sidebar-brand-mark">
          <div className="sidebar-logo-shell">
            <div className="sidebar-logo-face">
              <Bot size={18} />
            </div>
            <span className="sidebar-logo-dot sidebar-logo-dot-top" />
            <span className="sidebar-logo-dot sidebar-logo-dot-bottom" />
          </div>
        </div>
        <div className="sidebar-brand-copy">
          <strong>XiaoAiAgent</strong>
          <small>Frontend Console</small>
        </div>
      </div>

      <nav className="sidebar-nav">
        {navItems.map(({ key, href, label, Icon }) => {
          const active = key === page
          return (
            <a className={`sidebar-link ${active ? 'sidebar-link-active' : ''}`} href={href} key={key}>
              <span className="sidebar-link-icon">
                <Icon size={18} />
              </span>
              <span className="sidebar-link-copy">
                <strong>{label}</strong>
              </span>
            </a>
          )
        })}
      </nav>

      <div className="sidebar-mascot-card">
        <div className="sidebar-mascot-stars">
          <span />
          <span />
          <span />
        </div>
        <div className="sidebar-mascot-floor" />
        <div className="sidebar-mascot-bot">
          <div className="sidebar-mascot-antenna" />
          <div className="sidebar-mascot-head">
            <span className="sidebar-mascot-eye" />
            <span className="sidebar-mascot-eye" />
          </div>
          <div className="sidebar-mascot-body" />
        </div>
      </div>

      <div className="sidebar-user-card">
        <div className="sidebar-user-avatar">管</div>
        <div className="sidebar-user-copy">
          <strong>管理员</strong>
          <small>admin@xiaoaiagent.local</small>
        </div>
        <ChevronRight size={16} />
      </div>
    </aside>
  )
}
