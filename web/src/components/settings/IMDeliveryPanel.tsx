import type { IMAccount, IMTarget } from '../../types'

type Props = {
  accounts: IMAccount[]
  targets: IMTarget[]
  enabled: boolean
  accountID: string
  targetID: string
  dirty: boolean
  saving: boolean
  feedback: string | null
  error: string | null
  onEnabledChange: (value: boolean) => void
  onAccountChange: (value: string) => void
  onTargetChange: (value: string) => void
  onSave: () => void
}

export function IMDeliveryPanel({
  accounts,
  targets,
  enabled,
  accountID,
  targetID,
  dirty,
  saving,
  feedback,
  error,
  onEnabledChange,
  onAccountChange,
  onTargetChange,
  onSave,
}: Props) {
  return (
    <article className="panel settings-panel">
      <div className="panel-head compact">
        <div>
          <p className="eyebrow">IM DELIVERY</p>
          <h3>回复镜像到微信</h3>
        </div>
      </div>

      <div className="settings-form">
        <label className="checkbox-row">
          <input
            checked={enabled}
            onChange={(event) => onEnabledChange(event.target.checked)}
            type="checkbox"
          />
          <span>开启后，小爱的正常回复会异步再发一份到微信。</span>
        </label>

        <label className="settings-field">
          <span>激活微信账号</span>
          <select
            className="settings-select"
            value={accountID}
            onChange={(event) => onAccountChange(event.target.value)}
          >
            <option value="">请选择账号</option>
            {accounts.map((account) => (
              <option key={account.id} value={account.id}>
                {account.display_name || account.remote_account_id}
              </option>
            ))}
          </select>
        </label>

        <label className="settings-field">
          <span>默认触达对象</span>
          <select
            className="settings-select"
            value={targetID}
            onChange={(event) => onTargetChange(event.target.value)}
          >
            <option value="">请选择目标</option>
            {targets.map((target) => (
              <option key={target.id} value={target.id}>
                {target.name} · {target.target_user_id}
              </option>
            ))}
          </select>
        </label>

        <div className="settings-actions">
          <button
            className="settings-button"
            disabled={saving || !dirty}
            onClick={() => onSave()}
            type="button"
          >
            {saving ? '保存中...' : '保存镜像设置'}
          </button>
          <span className="settings-note">发送为异步副作用，不阻塞小爱当前播报。</span>
        </div>

        {feedback ? <div className="settings-feedback">{feedback}</div> : null}
        {error ? <div className="error-banner settings-error">{error}</div> : null}
      </div>
    </article>
  )
}

