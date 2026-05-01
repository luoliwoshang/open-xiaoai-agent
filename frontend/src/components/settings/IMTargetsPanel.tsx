import { useState } from 'react'
import { Plus, Trash2, Star } from 'lucide-react'
import type { IMAccount, IMTarget } from '../../types'

interface IMTargetsPanelProps {
  accounts: IMAccount[]
  targets: IMTarget[]
  onCreate: (data: { account_id: string; name: string; target_user_id: string; is_default: boolean }) => Promise<void>
  onSetDefault: (accountId: string, targetId: string) => Promise<void>
  onDelete: (targetId: string) => Promise<void>
}

export function IMTargetsPanel({ accounts, targets, onCreate, onSetDefault, onDelete }: IMTargetsPanelProps) {
  const [showForm, setShowForm] = useState(false)
  const [accountId, setAccountId] = useState('')
  const [name, setName] = useState('')
  const [userId, setUserId] = useState('')
  const [isDefault, setIsDefault] = useState(false)

  const handleCreate = async () => {
    if (!accountId || !name || !userId) return
    await onCreate({ account_id: accountId, name, target_user_id: userId, is_default: isDefault })
    setAccountId('')
    setName('')
    setUserId('')
    setIsDefault(false)
    setShowForm(false)
  }

  return (
    <div className="settings-section">
      <div className="settings-section-header">
        <div className="settings-section-title">投递目标</div>
        <div className="settings-section-desc">管理微信消息投递目标</div>
      </div>
      <div className="settings-section-body">
        {!showForm ? (
          <button className="btn" onClick={() => setShowForm(true)} style={{ marginBottom: 16 }}>
            <Plus /> 添加目标
          </button>
        ) : (
          <div style={{ marginBottom: 16, padding: 16, background: 'var(--bg-elevated)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border-subtle)' }}>
            <div className="form-group">
              <label className="form-label">所属账号</label>
              <select className="form-select" value={accountId} onChange={(e) => setAccountId(e.target.value)}>
                <option value="">选择账号</option>
                {accounts.map((acc) => (
                  <option key={acc.id} value={acc.id}>{acc.display_name || acc.remote_account_id}</option>
                ))}
              </select>
            </div>
            <div className="form-group">
              <label className="form-label">名称</label>
              <input className="form-input" value={name} onChange={(e) => setName(e.target.value)} placeholder="例如：文件传输助手" />
            </div>
            <div className="form-group">
              <label className="form-label">用户 ID</label>
              <input className="form-input" value={userId} onChange={(e) => setUserId(e.target.value)} placeholder="微信用户 ID" />
            </div>
            <div className="form-group">
              <div className="form-checkbox-group">
                <input type="checkbox" className="form-checkbox" checked={isDefault} onChange={(e) => setIsDefault(e.target.checked)} />
                <label className="form-label" style={{ marginBottom: 0 }}>设为默认</label>
              </div>
            </div>
            <div style={{ display: 'flex', gap: 8 }}>
              <button className="btn btn-primary" onClick={handleCreate}>创建</button>
              <button className="btn" onClick={() => setShowForm(false)}>取消</button>
            </div>
          </div>
        )}

        <div className="im-targets-list">
          {targets.map((t) => (
            <div key={t.id} className="im-target-item">
              <div className="im-target-info">
                <span style={{ fontSize: 13, color: 'var(--text-primary)' }}>{t.name}</span>
                {t.is_default && <span className="im-target-default">默认</span>}
                <span style={{ fontSize: 11, color: 'var(--text-ghost)', fontFamily: 'var(--font-mono)' }}>{t.target_user_id}</span>
              </div>
              <div className="im-target-actions">
                {!t.is_default && (
                  <button className="btn btn-sm" onClick={() => onSetDefault(t.account_id, t.id)}>
                    <Star style={{ width: 12, height: 12 }} />
                  </button>
                )}
                <button className="btn btn-sm btn-danger" onClick={() => onDelete(t.id)}>
                  <Trash2 style={{ width: 12, height: 12 }} />
                </button>
              </div>
            </div>
          ))}
          {targets.length === 0 && (
            <div className="empty-state" style={{ padding: 20 }}>
              <div className="empty-state-title">暂无目标</div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
