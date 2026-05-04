import { useEffect, useState } from 'react'
import { FolderCog, Save } from 'lucide-react'

interface MemorySettingsPanelProps {
  storageDir: string
  onSave: (dir: string) => Promise<void>
}

export function MemorySettingsPanel({ storageDir, onSave }: MemorySettingsPanelProps) {
  const [value, setValue] = useState(storageDir)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    setValue(storageDir)
  }, [storageDir])

  const handleSave = async () => {
    const next = value.trim()
    if (!next) return
    setSaving(true)
    try {
      await onSave(next)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="settings-section">
      <div className="settings-section-header">
        <div className="settings-section-title">记忆目录</div>
        <div className="settings-section-desc">长期记忆文件会按 memory key 落到这个目录，供主流程 reply 与复杂任务复用。</div>
      </div>
      <div className="settings-section-body">
        <div className="form-group">
          <label className="form-label">存储目录</label>
          <div className="inline-form-row">
            <div className="inline-form-input">
              <FolderCog />
              <input
                type="text"
                className="form-input"
                value={value}
                onChange={(e) => setValue(e.target.value)}
                placeholder=".open-xiaoai-agent/memory"
              />
            </div>
            <button className="btn btn-primary" onClick={handleSave} disabled={saving}>
              <Save />
              {saving ? '保存中...' : '保存目录'}
            </button>
          </div>
          <div className="form-help-text">默认主流程会使用 `main-voice` 这份记忆文件。手动编辑内容与查看更新日志请到“长期记忆”页面。</div>
        </div>
      </div>
    </div>
  )
}
