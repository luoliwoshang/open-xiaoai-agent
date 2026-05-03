import { LayoutDashboard, FileText, Settings } from 'lucide-react'
import type { AssistantRuntimeStatus, IMSnapshot, SettingsSnapshot, XiaoAIConnectionStatus } from '../types'

interface SidebarProps {
  page: string
  assistant: AssistantRuntimeStatus
  xiaoai: XiaoAIConnectionStatus
  settings: SettingsSnapshot
  im: IMSnapshot
}

type SidebarStatusTone = 'green' | 'red' | 'yellow' | 'blue' | 'gray' | 'purple'

interface IMChannelStatusSummary {
  tone: SidebarStatusTone
  label: string
  detail: string
}

function platformLabel(platform: string): string {
  switch (platform.trim()) {
    case 'weixin':
      return '微信'
    default:
      return platform.trim() || 'IM'
  }
}

function compactText(value: string, max: number): string {
  const text = value.trim()
  if (!text) return ''
  if (text.length <= max) return text
  return `${text.slice(0, max - 1).trimEnd()}…`
}

function buildIMChannelStatus(settings: SettingsSnapshot, im: IMSnapshot): IMChannelStatusSummary {
  const selectedAccountID = settings.im_selected_account_id.trim()
  const selectedTargetID = settings.im_selected_target_id.trim()
  const selectedAccount = im.accounts.find((account) => account.id === selectedAccountID)
  const selectedTarget = im.targets.find((target) => target.id === selectedTargetID)

  if (selectedAccount && selectedTarget) {
    const targetLabel = compactText(selectedTarget.name || selectedTarget.target_user_id || selectedTarget.id, 24)
    return {
      tone: settings.im_delivery_enabled ? 'green' : 'purple',
      label: settings.im_delivery_enabled ? 'IM 默认渠道已启用' : 'IM 默认渠道已配置',
      detail: `${platformLabel(selectedAccount.platform)} · ${targetLabel}`,
    }
  }

  if (selectedAccountID || selectedTargetID) {
    return {
      tone: 'red',
      label: 'IM 默认渠道配置失效',
      detail: '请到调试设置重新选择账号与触达对象',
    }
  }

  if (im.accounts.length > 0 || im.targets.length > 0) {
    return {
      tone: 'yellow',
      label: 'IM 尚未选定默认渠道',
      detail: `${im.accounts.length} 个账号 / ${im.targets.length} 个目标`,
    }
  }

  return {
    tone: 'gray',
    label: 'IM 未配置渠道',
    detail: '尚未添加账号或默认触达对象',
  }
}

export function Sidebar({ page, assistant, xiaoai, settings, im }: SidebarProps) {
  const navItems = [
    { id: 'dashboard', label: '调试台', icon: LayoutDashboard },
    { id: 'logs', label: '调试日志', icon: FileText },
    { id: 'settings', label: '调试设置', icon: Settings },
  ]
  const imStatus = buildIMChannelStatus(settings, im)

  return (
    <aside className="sidebar">
      <div className="sidebar-brand">
        <h1>XiaoAi Agent</h1>
        <div className="sidebar-brand-sub">debug console</div>
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
        <div className="status-row">
          <span className={`status-dot ${imStatus.tone}`} />
          <span>{imStatus.label}</span>
        </div>
        <div className="status-row status-row-detail" title={imStatus.detail}>
          <span className="status-dot gray" />
          <span className="status-detail-text">{imStatus.detail}</span>
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
