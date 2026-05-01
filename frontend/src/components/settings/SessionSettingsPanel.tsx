import { useState } from 'react'
import { Save } from 'lucide-react'

interface SessionSettingsPanelProps {
  windowSeconds: number
  onSave: (seconds: number) => Promise<void>
}

export function SessionSettingsPanel({ windowSeconds, onSave }: SessionSettingsPanelProps) {
  const [value, setValue] = useState(String(windowSeconds))
  const [saving, setSaving] = useState(false)

  const handleSave = async () => {
    const num = parseInt(value, 10)
    if (isNaN(num) || num < 0) return
    setSaving(true)
    try {
      await onSave(num)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="settings-section">
      <div className="settings-section-header">
        <div className="settings-section-title">会话窗口</div>
        <div className="settings-section-desc">设置会话上下文的滑动窗口秒数</div>
      </div>
      <div className="settings-section-body">
        <div className="form-group">
          <label className="form-label">窗口秒数</label>
          <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
            <input
              type="number"
              className="form-input"
              style={{ maxWidth: 140 }}
              value={value}
              onChange={(e) => setValue(e.target.value)}
              min={0}
            />
            <button className="btn btn-primary" onClick={handleSave} disabled={saving}>
              <Save />
              {saving ? '保存中...' : '保存'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
