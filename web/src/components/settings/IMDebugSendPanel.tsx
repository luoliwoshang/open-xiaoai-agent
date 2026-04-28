import type { IMAccount, IMTarget } from '../../types'

type Props = {
  account: IMAccount | null
  target: IMTarget | null
  configDirty: boolean
  text: string
  sending: boolean
  feedback: string | null
  error: string | null
  onTextChange: (value: string) => void
  onSend: () => void
}

export function IMDebugSendPanel({
  account,
  target,
  configDirty,
  text,
  sending,
  feedback,
  error,
  onTextChange,
  onSend,
}: Props) {
  const ready = Boolean(account && target)

  return (
    <article className="panel settings-panel panel-wide">
      <div className="panel-head compact">
        <div>
          <p className="eyebrow">IM DEBUG</p>
          <h3>默认渠道调试</h3>
        </div>
      </div>

      <div className="settings-form">
        <p className="settings-note">
          这里始终命中当前已经保存的默认渠道，不依赖自动镜像开关。适合单独验证账号、目标和通道是否正常。
        </p>

        {ready ? (
          <div className="focus-grid">
            <div className="task-meta">
              <span>当前渠道</span>
              <p>{account?.platform || '—'}</p>
            </div>
            <div className="task-meta">
              <span>当前账号</span>
              <p>{account?.display_name || account?.remote_account_id || '—'}</p>
            </div>
            <div className="task-meta task-meta-wide">
              <span>当前目标</span>
              <p>{target?.name} · {target?.target_user_id}</p>
            </div>
          </div>
        ) : (
          <div className="empty-card">请先在上方保存一个默认微信账号和触达目标，然后再发送测试消息。</div>
        )}

        {configDirty ? (
          <div className="settings-feedback">
            上方镜像配置还有未保存的修改；调试发送仍然会命中当前已保存的默认渠道。
          </div>
        ) : null}

        <label className="settings-field">
          <span>测试文本</span>
          <textarea
            className="settings-textarea"
            disabled={!ready || sending}
            placeholder="例如：这是一条默认渠道调试消息。"
            rows={4}
            value={text}
            onChange={(event) => onTextChange(event.target.value)}
          />
        </label>

        <div className="settings-actions">
          <button
            className="settings-button"
            disabled={!ready || sending || !text.trim()}
            onClick={() => onSend()}
            type="button"
          >
            {sending ? '发送中...' : '发送测试消息'}
          </button>
          <span className="settings-note">当前阶段只支持文本消息。</span>
        </div>

        {feedback ? <div className="settings-feedback">{feedback}</div> : null}
        {error ? <div className="error-banner settings-error">{error}</div> : null}
      </div>
    </article>
  )
}
