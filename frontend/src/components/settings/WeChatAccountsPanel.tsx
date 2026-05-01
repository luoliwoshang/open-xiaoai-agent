import { Plus, Trash2 } from 'lucide-react'
import type { IMAccount } from '../../types'
import { formatTime } from '../../lib/dashboard'

interface WeChatAccountsPanelProps {
  accounts: IMAccount[]
  onDelete: (accountId: string) => Promise<void>
  onLogin: () => void
}

export function WeChatAccountsPanel({ accounts, onDelete, onLogin }: WeChatAccountsPanelProps) {
  return (
    <div className="settings-section">
      <div className="settings-section-header">
        <div className="settings-section-title">微信账号</div>
        <div className="settings-section-desc">管理已绑定的微信账号</div>
      </div>
      <div className="settings-section-body">
        <button className="btn" onClick={onLogin} style={{ marginBottom: 16 }}>
          <Plus /> 添加账号
        </button>
        <div className="im-accounts-list">
          {accounts.map((acc) => (
            <div key={acc.id} className="im-account-item">
              <div className="im-account-info">
                <div className="im-account-name">{acc.display_name || acc.remote_account_id}</div>
                <div className="im-account-meta">
                  {acc.platform} · 最后发送: {formatTime(acc.last_sent_at)}
                </div>
              </div>
              <button className="btn btn-sm btn-danger" onClick={() => onDelete(acc.id)}>
                <Trash2 style={{ width: 12, height: 12 }} />
              </button>
            </div>
          ))}
          {accounts.length === 0 && (
            <div className="empty-state" style={{ padding: 20 }}>
              <div className="empty-state-title">暂无绑定账号</div>
              <div className="empty-state-desc">点击上方按钮扫码登录</div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
