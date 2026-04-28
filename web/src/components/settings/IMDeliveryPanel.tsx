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
  const hasAccounts = accounts.length > 0
  const hasTargets = targets.length > 0

  return (
    <article className="panel settings-panel">
      <div className="panel-head compact">
        <div>
          <p className="eyebrow">IM DELIVERY</p>
          <h3>默认触达</h3>
        </div>
      </div>

      <div className="settings-form">
        <label className="checkbox-row">
          <input
            checked={enabled}
            onChange={(event) => onEnabledChange(event.target.checked)}
            type="checkbox"
          />
          <span>开启后，小爱的正常回复会悄悄再发一份到默认微信对象。</span>
        </label>

        <label className="settings-field">
          <span>激活微信账号</span>
          <select
            className="settings-select"
            disabled={!hasAccounts}
            value={accountID}
            onChange={(event) => onAccountChange(event.target.value)}
          >
            <option value="">{hasAccounts ? '请选择账号' : '请先配置带有触达目标的账号'}</option>
            {accounts.map((account) => (
              <option key={account.id} value={account.id}>
                {account.display_name || account.remote_account_id}
              </option>
            ))}
          </select>
          <span className="settings-note">这里只显示已经配置过触达目标的账号。</span>
        </label>

        <label className="settings-field">
          <span>默认触达对象</span>
          <select
            className="settings-select"
            disabled={!accountID || !hasTargets}
            value={targetID}
            onChange={(event) => onTargetChange(event.target.value)}
          >
            <option value="">{accountID ? (hasTargets ? '请选择目标' : '当前账号还没有已配置目标') : '请先选择账号'}</option>
            {targets.map((target) => (
              <option key={target.id} value={target.id}>
                {target.name} · {target.target_user_id}
              </option>
            ))}
          </select>
          <span className="settings-note">这里只能从已经配置好的对象里选，避免把消息发到未知目标。</span>
        </label>

        <div className="settings-actions">
          <button
            className="settings-button"
            disabled={saving || !dirty || (enabled && (!accountID || !targetID))}
            onClick={() => onSave()}
            type="button"
          >
            {saving ? '保存中...' : '保存镜像设置'}
          </button>
          <span className="settings-note">这条转发不会打断小爱当前的播报节奏。</span>
        </div>

        {feedback ? <div className="settings-feedback">{feedback}</div> : null}
        {error ? <div className="error-banner settings-error">{error}</div> : null}
      </div>
    </article>
  )
}
