import { LayoutDashboard, FileText, Settings } from 'lucide-react'
import type { AssistantRuntimeStatus, XiaoAIConnectionStatus } from '../types'

interface SidebarProps {
  page: string
  assistant: AssistantRuntimeStatus
  xiaoai: XiaoAIConnectionStatus
}

export function Sidebar({ page, assistant, xiaoai }: SidebarProps) {
  const navItems = [
    { id: 'dashboard', label: '仪表盘', icon: LayoutDashboard },
    { id: 'logs', label: '日志', icon: FileText },
    { id: 'settings', label: '设置', icon: Settings },
  ]

  return (
    <aside className="sidebar">
      <div className="sidebar-brand">
        <h1>XiaoAi Agent</h1>
        <div className="sidebar-brand-sub">dashboard</div>
      </div>
      <nav className="sidebar-nav">
        {navItems.map((item) => (
          <a
            key={item.id}
            href={`#/${item.id === 'dashboard' ? '' : item.id}`}
            className={`nav-item ${page === item.id ? 'active' : ''}`}
          >
            <item.icon />
            {item.label}
          </a>
        ))}
      </nav>
      <div className="sidebar-status">
        <div className="status-row">
          <span className={`status-dot ${xiaoai.connected ? 'green' : 'red'}`} />
          <span>{xiaoai.connected ? '小爱已连接' : '小爱未连接'}</span>
        </div>
        <div className="status-row">
          <span className={`status-dot ${assistant.has_voice_channel ? 'blue' : 'gray'}`} />
          <span>{assistant.has_voice_channel ? '语音通道就绪' : '等待语音通道'}</span>
        </div>
        <div className="status-row">
          <span className={`status-dot ${assistant.busy ? 'yellow' : 'green'}`} />
          <span>{assistant.busy ? '执行中' : '空闲'}</span>
        </div>
        {assistant.result_report_ready && (
          <div className="status-row">
            <span className="status-dot blue" />
            <span>结果待汇报</span>
          </div>
        )}
        {(xiaoai.connected || xiaoai.last_remote_addr) && (
          <div className="status-row">
            <span className="status-dot gray" />
            <span>{xiaoai.last_remote_addr || `活动连接 ${xiaoai.active_sessions}`}</span>
          </div>
        )}
      </div>
    </aside>
  )
}
