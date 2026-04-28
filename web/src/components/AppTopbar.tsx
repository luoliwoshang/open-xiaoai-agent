import type { Page } from '../lib/dashboard'

type Props = {
  page: Page
}

export function AppTopbar({ page }: Props) {
  return (
    <header className="topbar">
      <div>
        <p className="eyebrow">XIAOAIAGENT</p>
        <h1 className="topbar-title">XiaoAiAgent 控制台</h1>
      </div>

      <nav className="topbar-nav">
        <a className={`topbar-link ${page === 'dashboard' ? 'topbar-link-active' : ''}`} href="#/">
          任务看板
        </a>
        <a className={`topbar-link ${page === 'settings' ? 'topbar-link-active' : ''}`} href="#/settings">
          系统设置
        </a>
        <a className={`topbar-link ${page === 'logs' ? 'topbar-link-active' : ''}`} href="#/logs">
          后端日志
        </a>
      </nav>
    </header>
  )
}
