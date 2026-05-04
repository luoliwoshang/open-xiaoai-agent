import { useState, useCallback, useRef, useEffect } from 'react'
import { Cog, MessageSquare } from 'lucide-react'
import type { DashboardState, WeChatLoginStart, WeChatLoginStatus } from '../types'
import {
  saveSessionSettings,
  saveMemorySettings,
  saveIMDeliverySettings,
  startWeChatLogin,
  getWeChatLoginStatus,
  confirmWeChatLogin,
  createTarget,
  setDefaultTarget,
  deleteTarget,
  deleteAccount,
  sendDebugText,
  sendDebugImage,
  sendDebugFile,
} from '../lib/api'
import { selectBestTarget } from '../lib/dashboard'
import { SessionSettingsPanel } from '../components/settings/SessionSettingsPanel'
import { MemorySettingsPanel } from '../components/settings/MemorySettingsPanel'
import { IMDeliveryPanel } from '../components/settings/IMDeliveryPanel'
import { IMDebugSendPanel } from '../components/settings/IMDebugSendPanel'
import { IMTargetsPanel } from '../components/settings/IMTargetsPanel'
import { WeChatAccountsPanel } from '../components/settings/WeChatAccountsPanel'
import { WeChatLoginPanel } from '../components/settings/WeChatLoginPanel'

interface SettingsPageProps {
  state: DashboardState
  onReload: () => void
}

type SettingsSection = 'system' | 'im'
type IMSubTab = 'debug' | 'delivery' | 'accounts' | 'targets'

export function SettingsPage({ state, onReload }: SettingsPageProps) {
  const [section, setSection] = useState<SettingsSection>('system')
  const [imTab, setImTab] = useState<IMSubTab>('debug')
  const [toast, setToast] = useState<string | null>(null)
  const [loginData, setLoginData] = useState<WeChatLoginStart | null>(null)
  const [loginStatus, setLoginStatus] = useState<WeChatLoginStatus | null>(null)
  const loginPollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const accounts = Array.isArray(state.im?.accounts) ? state.im.accounts : []
  const targets = Array.isArray(state.im?.targets) ? state.im.targets : []

  const showToast = useCallback((msg: string, _isError = false) => {
    setToast(msg)
    setTimeout(() => setToast(null), 3000)
  }, [])

  const handleSaveSession = async (seconds: number) => {
    await saveSessionSettings({ session_window_seconds: seconds })
    showToast('会话设置已保存')
    onReload()
  }

  const handleSaveMemoryDir = async (dir: string) => {
    await saveMemorySettings(dir)
    showToast('记忆目录已保存')
    onReload()
  }

  const handleSaveDelivery = async (settings: Parameters<typeof saveIMDeliverySettings>[0]) => {
    await saveIMDeliverySettings(settings)
    showToast('投递设置已保存')
    onReload()
  }

  const handleStartLogin = async () => {
    try {
      const data = await startWeChatLogin()
      setLoginData(data)
      setLoginStatus({ status: 'pending', message: '等待扫码' })
      loginPollRef.current = setInterval(async () => {
        try {
          const status = await getWeChatLoginStatus(data.session_key)
          setLoginStatus(status)
          if (status.status === 'confirmed' || status.status === 'expired' || status.status === 'failed') {
            if (loginPollRef.current) clearInterval(loginPollRef.current)
          }
        } catch { /* ignore */ }
      }, 2000)
    } catch (e: unknown) {
      showToast(e instanceof Error ? e.message : '启动登录失败', true)
    }
  }

  const handleConfirmLogin = async () => {
    if (!loginData) return
    try {
      await confirmWeChatLogin(loginData.session_key)
      showToast('登录成功')
      setLoginData(null)
      setLoginStatus(null)
      if (loginPollRef.current) clearInterval(loginPollRef.current)
      onReload()
    } catch (e: unknown) {
      showToast(e instanceof Error ? e.message : '确认失败', true)
    }
  }

  const handleCancelLogin = () => {
    setLoginData(null)
    setLoginStatus(null)
    if (loginPollRef.current) clearInterval(loginPollRef.current)
  }

  useEffect(() => {
    return () => {
      if (loginPollRef.current) clearInterval(loginPollRef.current)
    }
  }, [])

  return (
    <div>
      <div className="page-header">
        <h2>调试设置</h2>
        <div className="page-header-sub">调试主流程、语音上下文与 IM 网关配置</div>
      </div>

      <div className="settings-container">
        <div className="settings-nav">
          <div
            className={`settings-nav-item ${section === 'system' ? 'active' : ''}`}
            onClick={() => setSection('system')}
          >
            <Cog /> 系统
          </div>
          <div
            className={`settings-nav-item ${section === 'im' ? 'active' : ''}`}
            onClick={() => setSection('im')}
          >
            <MessageSquare /> IM 网关
          </div>
        </div>

        <div className="settings-content">
          {section === 'system' && (
            <>
              <SessionSettingsPanel
                windowSeconds={state.settings.session_window_seconds}
                onSave={handleSaveSession}
              />
              <MemorySettingsPanel
                storageDir={state.settings.memory_storage_dir}
                onSave={handleSaveMemoryDir}
              />
            </>
          )}

          {section === 'im' && (
            <>
              <div className="debug-send-tabs" style={{ marginBottom: 16 }}>
                {(['debug', 'delivery', 'accounts', 'targets'] as IMSubTab[]).map((tab) => (
                  <button
                    key={tab}
                    className={`debug-send-tab ${imTab === tab ? 'active' : ''}`}
                    onClick={() => setImTab(tab)}
                  >
                    {{ debug: '调试', delivery: '投递', accounts: '账号', targets: '目标' }[tab]}
                  </button>
                ))}
              </div>

              {imTab === 'debug' && (
                <IMDebugSendPanel
                  onSendText={sendDebugText}
                  onSendImage={sendDebugImage}
                  onSendFile={sendDebugFile}
                />
              )}

              {imTab === 'delivery' && (
                <IMDeliveryPanel
                  enabled={state.settings.im_delivery_enabled}
                  selectedAccountId={state.settings.im_selected_account_id || accounts[0]?.id || ''}
                  selectedTargetId={state.settings.im_selected_target_id || selectBestTarget(targets)}
                  accounts={accounts}
                  targets={targets}
                  onSave={handleSaveDelivery}
                />
              )}

              {imTab === 'accounts' && (
                <WeChatAccountsPanel
                  accounts={accounts}
                  onDelete={async (id) => { await deleteAccount(id); onReload() }}
                  onLogin={handleStartLogin}
                />
              )}

              {imTab === 'targets' && (
                <IMTargetsPanel
                  accounts={accounts}
                  targets={targets}
                  onCreate={async (data) => { await createTarget(data); onReload() }}
                  onSetDefault={async (a, t) => { await setDefaultTarget(a, t); onReload() }}
                  onDelete={async (id) => { await deleteTarget(id); onReload() }}
                />
              )}
            </>
          )}
        </div>
      </div>

      {loginData && (
        <WeChatLoginPanel
          qrCodeDataUrl={loginData.qr_code_data_url}
          qrRawText={loginData.qr_raw_text}
          loginStatus={loginStatus}
          onConfirm={handleConfirmLogin}
          onCancel={handleCancelLogin}
        />
      )}

      {toast && (
        <div className={`toast ${toast.includes('失败') || toast.includes('错误') ? 'error' : 'success'}`}>
          {toast}
        </div>
      )}
    </div>
  )
}
