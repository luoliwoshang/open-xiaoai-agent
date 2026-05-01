import type { WeChatLoginStatus } from '../../types'

interface WeChatLoginPanelProps {
  qrCodeDataUrl: string
  loginStatus: WeChatLoginStatus | null
  onConfirm: () => void
  onCancel: () => void
}

export function WeChatLoginPanel({ qrCodeDataUrl, loginStatus, onConfirm, onCancel }: WeChatLoginPanelProps) {
  const statusText: Record<string, string> = {
    pending: '等待扫码...',
    scanned: '已扫码，请确认登录',
    confirmed: '已确认',
    expired: '二维码已过期',
    failed: '登录失败',
  }

  return (
    <div className="modal-overlay" onClick={onCancel}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-title">微信扫码登录</div>
        <div className="qr-code-container">
          <img src={qrCodeDataUrl} alt="WeChat QR Code" className="qr-code-img" />
          <div className="qr-status">
            {loginStatus ? statusText[loginStatus.status] || loginStatus.message : '加载中...'}
          </div>
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
          {loginStatus?.status === 'scanned' && (
            <button className="btn btn-primary" onClick={onConfirm}>确认登录</button>
          )}
          <button className="btn" onClick={onCancel}>取消</button>
        </div>
      </div>
    </div>
  )
}
