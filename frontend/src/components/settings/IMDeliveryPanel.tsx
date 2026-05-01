import { useState } from 'react'
import { Save } from 'lucide-react'
import type { IMAccount, IMTarget } from '../../types'

interface IMDeliveryPanelProps {
  enabled: boolean
  selectedAccountId: string
  selectedTargetId: string
  accounts: IMAccount[]
  targets: IMTarget[]
  onSave: (settings: { im_delivery_enabled: boolean; im_selected_account_id: string; im_selected_target_id: string }) => Promise<void>
}

export function IMDeliveryPanel({
  enabled,
  selectedAccountId,
  selectedTargetId,
  accounts,
  targets,
  onSave,
}: IMDeliveryPanelProps) {
  const [isEnabled, setIsEnabled] = useState(enabled)
  const [accountId, setAccountId] = useState(selectedAccountId)
  const [targetId, setTargetId] = useState(selectedTargetId)
  const [saving, setSaving] = useState(false)

  const filteredTargets = targets.filter((t) => t.account_id === accountId)

  const handleSave = async () => {
    setSaving(true)
    try {
      await onSave({
        im_delivery_enabled: isEnabled,
        im_selected_account_id: accountId,
        im_selected_target_id: targetId,
      })
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="settings-section">
      <div className="settings-section-header">
        <div className="settings-section-title">消息投递</div>
        <div className="settings-section-desc">配置自动将小爱回复镜像到微信</div>
      </div>
      <div className="settings-section-body">
        <div className="form-group">
          <div className="form-checkbox-group">
            <input
              type="checkbox"
              className="form-checkbox"
              checked={isEnabled}
              onChange={(e) => setIsEnabled(e.target.checked)}
            />
            <label className="form-label" style={{ marginBottom: 0 }}>启用自动投递</label>
          </div>
        </div>
        {isEnabled && (
          <>
            <div className="form-group">
              <label className="form-label">微信账号</label>
              <select
                className="form-select"
                value={accountId}
                onChange={(e) => {
                  setAccountId(e.target.value)
                  setTargetId('')
                }}
              >
                <option value="">选择账号</option>
                {accounts.map((acc) => (
                  <option key={acc.id} value={acc.id}>
                    {acc.display_name || acc.remote_account_id}
                  </option>
                ))}
              </select>
            </div>
            <div className="form-group">
              <label className="form-label">投递目标</label>
              <select
                className="form-select"
                value={targetId}
                onChange={(e) => setTargetId(e.target.value)}
              >
                <option value="">选择目标</option>
                {filteredTargets.map((t) => (
                  <option key={t.id} value={t.id}>
                    {t.name} {t.is_default ? '(默认)' : ''}
                  </option>
                ))}
              </select>
            </div>
          </>
        )}
        <button className="btn btn-primary" onClick={handleSave} disabled={saving}>
          <Save />
          {saving ? '保存中...' : '保存'}
        </button>
      </div>
    </div>
  )
}
