import type { IMAccount, IMTarget } from '../../types'

type Props = {
  accounts: IMAccount[]
  accountID: string
  name: string
  targetUserID: string
  setDefault: boolean
  saving: boolean
  feedback: string | null
  error: string | null
  accountTargets: IMTarget[]
  onAccountChange: (value: string) => void
  onNameChange: (value: string) => void
  onTargetUserIDChange: (value: string) => void
  onSetDefaultChange: (value: boolean) => void
  onSave: () => void
  onSetDefaultTarget: (accountID: string, targetID: string) => void
  onDeleteTarget: (targetID: string) => void
}

export function IMTargetsPanel({
  accounts,
  accountID,
  name,
  targetUserID,
  setDefault,
  saving,
  feedback,
  error,
  accountTargets,
  onAccountChange,
  onNameChange,
  onTargetUserIDChange,
  onSetDefaultChange,
  onSave,
  onSetDefaultTarget,
  onDeleteTarget,
}: Props) {
  return (
    <article className="panel settings-panel panel-wide">
      <div className="panel-head compact">
        <div>
          <p className="eyebrow">TARGETS</p>
          <h3>触达目标管理</h3>
        </div>
      </div>

      <div className="target-editor">
        <div className="settings-form">
          <label className="settings-field">
            <span>所属微信账号</span>
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
            <span>目标备注名</span>
            <input
              className="settings-input"
              placeholder="例如：我的微信"
              type="text"
              value={name}
              onChange={(event) => onNameChange(event.target.value)}
            />
          </label>

          <label className="settings-field">
            <span>微信用户 ID</span>
            <input
              className="settings-input"
              placeholder="例如：xxx@im.wechat"
              type="text"
              value={targetUserID}
              onChange={(event) => onTargetUserIDChange(event.target.value)}
            />
          </label>

          <label className="checkbox-row">
            <input checked={setDefault} onChange={(event) => onSetDefaultChange(event.target.checked)} type="checkbox" />
            <span>保存后设为该账号的默认触达目标</span>
          </label>

          <div className="settings-actions">
            <button
              className="settings-button"
              disabled={saving || !accountID || !targetUserID}
              onClick={() => onSave()}
              type="button"
            >
              {saving ? '保存中...' : '保存目标'}
            </button>
            <span className="settings-note">如果扫码返回了用户 ID，系统通常会自动补一个“扫码用户”目标。</span>
          </div>

          {feedback ? <div className="settings-feedback">{feedback}</div> : null}
          {error ? <div className="error-banner settings-error">{error}</div> : null}
        </div>

        <div className="target-list">
          {accountTargets.length === 0 ? (
            <div className="empty-card">当前账号还没有触达目标。</div>
          ) : (
            accountTargets.map((target) => (
              <article className="target-card" key={target.id}>
                <div>
                  <p className="task-title">{target.name}</p>
                  <p className="task-input">{target.target_user_id}</p>
                </div>
                <div className="target-actions">
                  {target.is_default ? <span className="badge badge-running">默认</span> : null}
                  <button className="ghost-button" onClick={() => onSetDefaultTarget(target.account_id, target.id)} type="button">
                    设为默认
                  </button>
                  <button className="ghost-button ghost-danger" onClick={() => onDeleteTarget(target.id)} type="button">
                    删除
                  </button>
                </div>
              </article>
            ))
          )}
        </div>
      </div>
    </article>
  )
}

