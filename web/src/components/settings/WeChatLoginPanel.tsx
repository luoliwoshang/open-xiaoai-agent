import { formatTime } from '../../lib/dashboard'
import type { WeChatLoginCandidate, WeChatLoginStatus } from '../../types'

type Props = {
  open: boolean
  loading: boolean
  polling: boolean
  confirming: boolean
  status: WeChatLoginStatus['status'] | null
  qrDataUrl: string | null
  qrRawText: string | null
  expiresAt: string | null
  message: string | null
  error: string | null
  candidate: WeChatLoginCandidate | null
  onClose: () => void
  onConfirm: () => void
}

export function WeChatLoginPanel({
  open,
  loading,
  polling,
  confirming,
  status,
  qrDataUrl,
  qrRawText,
  expiresAt,
  message,
  error,
  candidate,
  onClose,
  onConfirm,
}: Props) {
  if (!open) return null

  const confirmed = status === 'confirmed' && candidate

  return (
    <div className="settings-modal-backdrop" role="presentation">
      <div aria-modal="true" className="settings-modal-card" role="dialog">
        <div className="panel-head compact">
          <div>
            <p className="eyebrow">WECHAT LOGIN</p>
            <h3>微信扫码登录</h3>
          </div>
          <button className="ghost-button" onClick={() => onClose()} type="button">
            关闭
          </button>
        </div>

        {confirmed ? (
          <div className="settings-login-confirm">
            <p className="settings-note">
              扫码已经完成。只有你确认之后，这个账号才会真正加入系统，并自动补上一个“扫码用户”默认触达对象。
            </p>

            {message ? <div className="settings-feedback">{message}</div> : null}
            {error ? <div className="error-banner settings-error">{error}</div> : null}

            <div className="focus-grid">
              <div className="task-meta">
                <span>账号标识</span>
                <p>{candidate.display_name || candidate.remote_account_id}</p>
              </div>
              <div className="task-meta">
                <span>微信扫码用户</span>
                <p>{candidate.owner_user_id || '—'}</p>
              </div>
              <div className="task-meta task-meta-wide">
                <span>Base URL</span>
                <p>{candidate.base_url || '—'}</p>
              </div>
            </div>

            <div className="settings-actions">
              <button className="settings-button" disabled={confirming} onClick={() => onConfirm()} type="button">
                {confirming ? '添加中...' : '确认添加账号'}
              </button>
              <button className="ghost-button" disabled={confirming} onClick={() => onClose()} type="button">
                暂不添加
              </button>
            </div>
          </div>
        ) : (
          <div className="settings-login-grid">
            <div className="qr-card">
              {qrDataUrl ? (
                <img alt="微信登录二维码" className="qr-image" src={qrDataUrl} />
              ) : (
                <div className="empty-card qr-empty">
                  {loading ? '正在准备二维码...' : '点击新增微信账号后，这里会出现登录二维码。'}
                </div>
              )}
            </div>

            <div className="login-copy">
              <p className="settings-note">
                扫码确认后，界面会先展示待添加的账号信息。只有你点确认，登录态才会真正保存下来。
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

              <div className="settings-actions">
                <span className="settings-note">
                  {loading ? '正在准备扫码流程。' : polling ? '正在等待扫码与确认，不会自动把账号写进系统。' : '关闭弹窗后可以重新发起一次扫码。'}
                </span>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
