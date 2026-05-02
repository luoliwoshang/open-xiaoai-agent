import type { WeChatLoginStatus } from '../../types'

interface WeChatLoginPanelProps {
  qrCodeDataUrl: string
  qrRawText?: string
  loginStatus: WeChatLoginStatus | null
  onConfirm: () => void
  onCancel: () => void
}

export function WeChatLoginPanel({
  qrCodeDataUrl,
  qrRawText,
  loginStatus,
  onConfirm,
  onCancel,
}: WeChatLoginPanelProps) {
  const statusText: Record<string, string> = {
    pending: '等待扫码...',
    scanned: '已扫码，请确认登录',
    confirmed: '微信侧已确认，请确认添加账号',
    expired: '二维码已过期',
    failed: '登录失败',
  }
  const canConfirm =
    !!loginStatus?.candidate &&
    loginStatus.status !== 'expired' &&
    loginStatus.status !== 'failed'

  return (
    <div className="modal-overlay" onClick={onCancel}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-title">微信扫码登录</div>
        <div className="qr-code-container">
          {qrCodeDataUrl ? (
            <img src={qrCodeDataUrl} alt="WeChat QR Code" className="qr-code-img" />
          ) : (
            <div style={{ textAlign: 'center', fontSize: 12, color: 'var(--text-secondary)', lineHeight: 1.6 }}>
              暂时没有拿到二维码图片，请稍后重试。
            </div>
          )}
          <div className="qr-status">
            {loginStatus ? statusText[loginStatus.status] || loginStatus.message : '加载中...'}
          </div>
          {qrRawText && (
            <div style={{ textAlign: 'center', fontSize: 12, color: 'var(--text-ghost)', wordBreak: 'break-all' }}>
              二维码链接：{qrRawText}
            </div>
          )}
          {loginStatus?.candidate && (
            <div style={{ textAlign: 'center', fontSize: 12, color: 'var(--text-secondary)' }}>
              <div>检测到: {loginStatus.candidate.display_name}</div>
              <div style={{ color: 'var(--text-ghost)', fontFamily: 'var(--font-mono)' }}>
                {loginStatus.candidate.remote_account_id}
              </div>
            </div>
          )}
        </div>
        <div className="modal-actions">
          {canConfirm && (
            <button className="btn btn-primary" onClick={onConfirm}>确认添加账号</button>
          )}
          <button className="btn" onClick={onCancel}>取消</button>
        </div>
      </div>
    </div>
  )
}
