import { formatTime } from '../../lib/dashboard'

type Props = {
  loading: boolean
  polling: boolean
  qrDataUrl: string | null
  qrRawText: string | null
  expiresAt: string | null
  message: string | null
  error: string | null
  onStart: () => void
}

export function WeChatLoginPanel({
  loading,
  polling,
  qrDataUrl,
  qrRawText,
  expiresAt,
  message,
  error,
  onStart,
}: Props) {
  return (
    <article className="panel settings-panel panel-wide">
      <div className="panel-head compact">
        <div>
          <p className="eyebrow">WECHAT LOGIN</p>
          <h3>微信扫码登录</h3>
        </div>
        <button className="settings-button" disabled={loading || polling} onClick={() => onStart()} type="button">
          {loading ? '启动中...' : polling ? '等待扫码中' : '新增微信账号'}
        </button>
      </div>

      <div className="settings-login-grid">
        <div className="qr-card">
          {qrDataUrl ? (
            <img alt="微信登录二维码" className="qr-image" src={qrDataUrl} />
          ) : (
            <div className="empty-card qr-empty">点击右上角按钮后，这里会出现微信登录二维码。</div>
          )}
        </div>

        <div className="login-copy">
          <p className="settings-note">
            当前阶段只做微信文本触达，不做 IM 入站会话。扫码成功后，会自动保存微信账号登录态，并尝试把扫码用户登记为默认触达对象。
          </p>
          {message ? <div className="settings-feedback">{message}</div> : null}
          {error ? <div className="error-banner settings-error">{error}</div> : null}
          {expiresAt ? (
            <div className="task-meta">
              <span>二维码过期时间</span>
              <p>{formatTime(expiresAt)}</p>
            </div>
          ) : null}
          {qrRawText ? (
            <div className="task-meta task-meta-wide">
              <span>二维码原始内容</span>
              <p><code>{qrRawText}</code></p>
            </div>
          ) : null}
        </div>
      </div>
    </article>
  )
}

