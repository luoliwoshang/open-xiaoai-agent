import { BellRing, LayoutDashboard, Settings2, Sparkles, TextSearch } from 'lucide-react'
import type { DashboardState } from '../types'
import type { Page } from '../lib/dashboard'

type Props = {
  page: Page
  data: DashboardState
}

const navItems: Array<{ key: Page; href: string; label: string; caption: string; Icon: typeof LayoutDashboard }> = [
  { key: 'dashboard', href: '#/', label: '任务看板', caption: '对话、任务、交付', Icon: LayoutDashboard },
  { key: 'settings', href: '#/settings', label: '系统设置', caption: '镜像、账号、调试', Icon: Settings2 },
  { key: 'logs', href: '#/logs', label: '后端日志', caption: '排查与追踪', Icon: TextSearch },
]

export function AppTopbar({ page, data }: Props) {
  const pendingCount = data.tasks.filter((task) => task.report_pending).length
  const runningCount = data.tasks.filter((task) => task.state === 'running').length

  return (
    <aside className="app-sidebar">
      <div className="brand-card">
        <div className="brand-orb brand-orb-peach" />
        <div className="brand-orb brand-orb-mint" />
        <p className="brand-kicker">XIAOAIAGENT</p>
        <h1>更可爱一点的小爱工作台</h1>
        <p>
          把聊天、复杂任务、交付文件和默认触达都收在一个安静、清爽又更顺手的界面里。
        </p>
      </div>

      <nav className="sidebar-nav">
        {navItems.map(({ key, href, label, caption, Icon }) => {
          const active = key === page
          return (
            <a className={`sidebar-link ${active ? 'sidebar-link-active' : ''}`} href={href} key={key}>
              <span className="sidebar-link-icon">
                <Icon size={18} />
              </span>
              <span className="sidebar-link-copy">
                <strong>{label}</strong>
                <small>{caption}</small>
              </span>
            </a>
          )
        })}
      </nav>

      <div className="sidebar-stats">
        <article className="mini-stat mini-stat-peach">
          <span>
            <Sparkles size={14} />
            待补报
          </span>
          <strong>{pendingCount}</strong>
        </article>
        <article className="mini-stat mini-stat-mint">
          <span>
            <BellRing size={14} />
            执行中
          </span>
          <strong>{runningCount}</strong>
        </article>
      </div>
    </aside>
  )
}
