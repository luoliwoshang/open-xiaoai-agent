type Props = {
  windowInput: string
  settingsSaving: boolean
  settingsFeedback: string | null
  settingsError: string | null
  windowDirty: boolean
  onWindowInputChange: (value: string) => void
  onSave: () => void
}

export function SessionSettingsPanel({
  windowInput,
  settingsSaving,
  settingsFeedback,
  settingsError,
  windowDirty,
  onWindowInputChange,
  onSave,
}: Props) {
  return (
    <article className="panel settings-panel">
      <div className="panel-head compact">
        <div>
          <p className="eyebrow">SESSION</p>
          <h3>会话窗口</h3>
        </div>
      </div>

      <div className="settings-form">
        <label className="settings-field">
          <span>会话窗口秒数</span>
          <input
            className="settings-input"
            inputMode="numeric"
            min={30}
            max={3600}
            step={1}
            type="number"
            value={windowInput}
            onChange={(event) => onWindowInputChange(event.target.value)}
          />
        </label>

        <div className="settings-actions">
          <button
            className="settings-button"
            disabled={settingsSaving || !windowDirty}
            onClick={() => onSave()}
            type="button"
          >
            {settingsSaving ? '保存中...' : '保存设置'}
          </button>
          <span className="settings-note">默认值 300 秒，只保留滑动窗口策略。</span>
        </div>

        {settingsFeedback ? <div className="settings-feedback">{settingsFeedback}</div> : null}
        {settingsError ? <div className="error-banner settings-error">{settingsError}</div> : null}
      </div>
    </article>
  )
}

