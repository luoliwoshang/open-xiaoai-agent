import { formatTime } from '../../lib/dashboard'
import type { IMAccount } from '../../types'

type Props = {
  accounts: IMAccount[]
  loginBusy: boolean
  onStartLogin: () => void
  onDeleteAccount: (accountID: string) => void
}

export function WeChatAccountsPanel({ accounts, loginBusy, onStartLogin, onDeleteAccount }: Props) {
  return (
    <article className="panel settings-panel panel-wide">
      <div className="panel-head compact">
        <div>
          <p className="eyebrow">WECHAT ACCOUNTS</p>
          <h3>微信账号</h3>
        </div>
        <button className="settings-button" disabled={loginBusy} onClick={() => onStartLogin()} type="button">
          {loginBusy ? '登录流程进行中...' : '新增微信账号'}
        </button>
      </div>

      <div className="account-grid">
        {accounts.length === 0 ? (
          <div className="empty-card">还没有微信账号。先扫码登录一个账号。</div>
        ) : (
          accounts.map((account) => (
            <article className="account-card" key={account.id}>
              <div className="account-card-head">
                <div>
                  <h4>{account.display_name || account.remote_account_id}</h4>
                  <p>{account.remote_account_id}</p>
                </div>
                <button className="ghost-button" onClick={() => onDeleteAccount(account.id)} type="button">
                  删除
                </button>
              </div>

              <div className="focus-grid">
                <div className="task-meta">
                  <span>平台</span>
                  <p>{account.platform}</p>
                </div>
                <div className="task-meta">
                  <span>扫码用户</span>
                  <p>{account.owner_user_id || '—'}</p>
                </div>
                <div className="task-meta task-meta-wide">
                  <span>Base URL</span>
                  <p>{account.base_url}</p>
                </div>
                <div className="task-meta">
                  <span>最近发送</span>
                  <p>{formatTime(account.last_sent_at)}</p>
                </div>
                <div className="task-meta">
                  <span>最近错误</span>
                  <p>{account.last_error || '—'}</p>
                </div>
              </div>
            </article>
          ))
        )}
      </div>
    </article>
  )
}
