import { useEffect, useMemo, useState, type Dispatch, type SetStateAction } from 'react'
import { IMDebugSendPanel } from '../components/settings/IMDebugSendPanel'
import { IMDeliveryPanel } from '../components/settings/IMDeliveryPanel'
import { IMTargetsPanel } from '../components/settings/IMTargetsPanel'
import { SessionSettingsPanel } from '../components/settings/SessionSettingsPanel'
import { WeChatAccountsPanel } from '../components/settings/WeChatAccountsPanel'
import { WeChatLoginPanel } from '../components/settings/WeChatLoginPanel'
import { postFormData, postJSON } from '../lib/api'
import { normalizeSettings, selectBestTarget } from '../lib/dashboard'
import type {
  DashboardState,
  IMDeliveryReceipt,
  SessionSettings,
  SettingsSnapshot,
  WeChatLoginCandidate,
  WeChatLoginStart,
  WeChatLoginStatus,
} from '../types'

type SettingsSectionKey = 'system' | 'im'

type SettingsSection = {
  key: SettingsSectionKey
  eyebrow: string
  title: string
  description: string
}

type LoginPanelState = {
  open: boolean
  loading: boolean
  polling: boolean
  confirming: boolean
  sessionKey: string | null
  qrDataUrl: string | null
  qrRawText: string | null
  expiresAt: string | null
  status: WeChatLoginStatus['status'] | null
  message: string | null
  error: string | null
  candidate: WeChatLoginCandidate | null
}

const emptyLoginState: LoginPanelState = {
  open: false,
  loading: false,
  polling: false,
  confirming: false,
  sessionKey: null,
  qrDataUrl: null,
  qrRawText: null,
  expiresAt: null,
  status: null,
  message: null,
  error: null,
  candidate: null,
}

const settingsSections: SettingsSection[] = [
  {
    key: 'system',
    eyebrow: 'SYSTEM',
    title: '系统设置',
    description: '管理会话窗口和全局运行期行为，适合放置 Agent 的基础配置。',
  },
  {
    key: 'im',
    eyebrow: 'IM GATEWAY',
    title: 'IM 配置',
    description: '单独管理微信登录、账号、触达目标和回复镜像，不再与系统设置混排。',
  },
]

type Props = {
  data: DashboardState
  error: string | null
  setData: Dispatch<SetStateAction<DashboardState>>
  refresh: () => Promise<void>
}

export function SettingsPage({ data, error, setData, refresh }: Props) {
  const [activeSection, setActiveSection] = useState<SettingsSectionKey>('system')
  const [windowInput, setWindowInput] = useState('300')
  const [windowDirty, setWindowDirty] = useState(false)
  const [settingsSaving, setSettingsSaving] = useState(false)
  const [settingsFeedback, setSettingsFeedback] = useState<string | null>(null)
  const [settingsError, setSettingsError] = useState<string | null>(null)

  const [deliveryEnabled, setDeliveryEnabled] = useState(false)
  const [deliveryAccountID, setDeliveryAccountID] = useState('')
  const [deliveryTargetID, setDeliveryTargetID] = useState('')
  const [deliveryDirty, setDeliveryDirty] = useState(false)
  const [deliverySaving, setDeliverySaving] = useState(false)
  const [deliveryFeedback, setDeliveryFeedback] = useState<string | null>(null)
  const [deliveryError, setDeliveryError] = useState<string | null>(null)
  const [debugText, setDebugText] = useState('')
  const [debugTextSending, setDebugTextSending] = useState(false)
  const [debugTextFeedback, setDebugTextFeedback] = useState<string | null>(null)
  const [debugTextError, setDebugTextError] = useState<string | null>(null)
  const [debugImageFile, setDebugImageFile] = useState<File | null>(null)
  const [debugImageCaption, setDebugImageCaption] = useState('')
  const [debugImageInputKey, setDebugImageInputKey] = useState(0)
  const [debugImageSending, setDebugImageSending] = useState(false)
  const [debugImageFeedback, setDebugImageFeedback] = useState<string | null>(null)
  const [debugImageError, setDebugImageError] = useState<string | null>(null)
  const [debugFileFile, setDebugFileFile] = useState<File | null>(null)
  const [debugFileCaption, setDebugFileCaption] = useState('')
  const [debugFileInputKey, setDebugFileInputKey] = useState(0)
  const [debugFileSending, setDebugFileSending] = useState(false)
  const [debugFileFeedback, setDebugFileFeedback] = useState<string | null>(null)
  const [debugFileError, setDebugFileError] = useState<string | null>(null)

  const [loginPanel, setLoginPanel] = useState<LoginPanelState>(emptyLoginState)

  const [targetAccountID, setTargetAccountID] = useState('')
  const [targetName, setTargetName] = useState('')
  const [targetUserID, setTargetUserID] = useState('')
  const [targetDefault, setTargetDefault] = useState(true)
  const [targetSaving, setTargetSaving] = useState(false)
  const [targetFeedback, setTargetFeedback] = useState<string | null>(null)
  const [targetError, setTargetError] = useState<string | null>(null)

  const deliveryAccounts = useMemo(() => {
    const accountIDsWithTargets = new Set(data.im.targets.map((target) => target.account_id))
    return data.im.accounts.filter((account) => accountIDsWithTargets.has(account.id))
  }, [data.im.accounts, data.im.targets])

  const deliveryTargets = useMemo(() => {
    return data.im.targets.filter((target) => target.account_id === deliveryAccountID)
  }, [data.im.targets, deliveryAccountID])

  const savedDebugAccount = useMemo(() => {
    return data.im.accounts.find((account) => account.id === data.settings.im_selected_account_id) ?? null
  }, [data.im.accounts, data.settings.im_selected_account_id])

  const savedDebugTarget = useMemo(() => {
    const target = data.im.targets.find((item) => item.id === data.settings.im_selected_target_id) ?? null
    if (!target || !savedDebugAccount || target.account_id !== savedDebugAccount.id) {
      return null
    }
    return target
  }, [data.im.targets, data.settings.im_selected_target_id, savedDebugAccount])

  const targetFormTargets = useMemo(() => {
    return data.im.targets.filter((target) => target.account_id === targetAccountID)
  }, [data.im.targets, targetAccountID])

  const currentSection = useMemo(() => {
    return settingsSections.find((section) => section.key === activeSection) ?? settingsSections[0]
  }, [activeSection])

  useEffect(() => {
    if (windowDirty || settingsSaving) return
    setWindowInput(String(data.settings.session_window_seconds))
  }, [data.settings.session_window_seconds, settingsSaving, windowDirty])

  useEffect(() => {
    if (deliveryDirty || deliverySaving) return
    setDeliveryEnabled(data.settings.im_delivery_enabled)
    setDeliveryAccountID(data.settings.im_selected_account_id)
    setDeliveryTargetID(data.settings.im_selected_target_id)
  }, [
    data.settings.im_delivery_enabled,
    data.settings.im_selected_account_id,
    data.settings.im_selected_target_id,
    deliveryDirty,
    deliverySaving,
  ])

  useEffect(() => {
    const accountExists = data.im.accounts.some((account) => account.id === targetAccountID)
    if (accountExists) return
    setTargetAccountID(data.im.accounts[0]?.id ?? '')
  }, [data.im.accounts, targetAccountID])

  useEffect(() => {
    if (!loginPanel.open || !loginPanel.polling || !loginPanel.sessionKey) return

    let active = true
    const timer = window.setInterval(async () => {
      try {
        const response = await fetch(`/api/im/wechat/login/status?session_key=${encodeURIComponent(loginPanel.sessionKey ?? '')}`, {
          cache: 'no-store',
        })
        if (!response.ok) {
          throw new Error(await response.text())
        }
        const payload = (await response.json()) as { status?: WeChatLoginStatus }
        if (!active || !payload.status) return

        const nextStatus = payload.status.status
        setLoginPanel((current) => ({
          ...current,
          status: payload.status?.status ?? current.status,
          message: payload.status?.message ?? current.message,
          candidate: payload.status?.candidate ?? current.candidate,
          error: null,
          polling: nextStatus === 'pending' || nextStatus === 'scanned',
          sessionKey: nextStatus === 'expired' || nextStatus === 'failed' ? null : current.sessionKey,
        }))
      } catch (err) {
        if (!active) return
        setLoginPanel((current) => ({
          ...current,
          polling: false,
          error: err instanceof Error ? err.message : '登录状态查询失败',
        }))
      }
    }, 2000)

    return () => {
      active = false
      window.clearInterval(timer)
    }
  }, [loginPanel.open, loginPanel.polling, loginPanel.sessionKey])

  async function saveSessionWindowSettings() {
    const nextValue = Number(windowInput)
    if (!Number.isInteger(nextValue)) {
      setSettingsError('请输入整数秒数。')
      setSettingsFeedback(null)
      return
    }

    setSettingsSaving(true)
    setSettingsError(null)
    setSettingsFeedback(null)

    try {
      const payload = await postJSON<{ session?: SessionSettings }>('/api/settings/session', {
        window_seconds: nextValue,
      })
      const nextSettings = normalizeSettings(payload.session)
      setData((current) => ({
        ...current,
        settings: {
          ...current.settings,
          ...nextSettings,
        },
      }))
      setWindowInput(String(nextSettings.session_window_seconds))
      setWindowDirty(false)
      setSettingsFeedback('已保存，后续请求会立即按新的滑动窗口秒数生效。')
    } catch (err) {
      setSettingsError(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSettingsSaving(false)
    }
  }

  async function saveDeliverySettings() {
    if (deliveryEnabled && (!deliveryAccountID || !deliveryTargetID)) {
      setDeliveryError('开启镜像前，请先选择一个已经配置好的账号和触达目标。')
      setDeliveryFeedback(null)
      return
    }

    setDeliverySaving(true)
    setDeliveryError(null)
    setDeliveryFeedback(null)

    try {
      const payload = await postJSON<{ settings?: SettingsSnapshot }>('/api/settings/im-delivery', {
        enabled: deliveryEnabled,
        selected_account_id: deliveryAccountID,
        selected_target_id: deliveryTargetID,
      })
      const nextSettings = normalizeSettings(payload.settings)
      setData((current) => ({
        ...current,
        settings: nextSettings,
      }))
      setDeliveryDirty(false)
      setDeliveryFeedback('IM 文本触达设置已保存。')
    } catch (err) {
      setDeliveryError(err instanceof Error ? err.message : '保存失败')
    } finally {
      setDeliverySaving(false)
    }
  }

  async function startWeChatLogin() {
    setLoginPanel({
      ...emptyLoginState,
      open: true,
      loading: true,
      message: '正在准备微信登录二维码。',
    })
    try {
      const payload = await postJSON<{ login?: WeChatLoginStart }>('/api/im/wechat/login/start', {})
      if (!payload.login) {
        throw new Error('登录二维码返回为空')
      }
      setLoginPanel({
        open: true,
        loading: false,
        polling: true,
        confirming: false,
        sessionKey: payload.login.session_key,
        qrDataUrl: payload.login.qr_code_data_url,
        qrRawText: payload.login.qr_raw_text,
        expiresAt: payload.login.expires_at,
        status: 'pending',
        message: '请使用微信扫描下方二维码。',
        error: null,
        candidate: null,
      })
    } catch (err) {
      setLoginPanel({
        ...emptyLoginState,
        open: true,
        error: err instanceof Error ? err.message : '启动微信登录失败',
      })
    }
  }

  function closeWeChatLogin() {
    setLoginPanel(emptyLoginState)
  }

  async function confirmWeChatLogin() {
    if (!loginPanel.sessionKey) {
      setLoginPanel((current) => ({
        ...current,
        error: '当前登录会话已经失效，请重新扫码。',
      }))
      return
    }

    setLoginPanel((current) => ({
      ...current,
      confirming: true,
      error: null,
    }))
    try {
      await postJSON('/api/im/wechat/login/confirm', {
        session_key: loginPanel.sessionKey,
      })
      await refresh()
      setLoginPanel(emptyLoginState)
      setDeliveryFeedback('微信账号已添加，你现在可以继续配置触达目标和镜像规则。')
      setDeliveryError(null)
    } catch (err) {
      setLoginPanel((current) => ({
        ...current,
        confirming: false,
        error: err instanceof Error ? err.message : '确认添加微信账号失败',
      }))
    }
  }

  async function createTarget() {
    setTargetSaving(true)
    setTargetError(null)
    setTargetFeedback(null)
    try {
      await postJSON('/api/im/targets', {
        account_id: targetAccountID,
        name: targetName,
        target_user_id: targetUserID,
        set_default: targetDefault,
      })
      await refresh()
      setTargetName('')
      setTargetUserID('')
      setTargetDefault(true)
      setTargetFeedback('触达目标已保存。')
      if (!deliveryAccountID) {
        setDeliveryAccountID(targetAccountID)
      }
    } catch (err) {
      setTargetError(err instanceof Error ? err.message : '保存目标失败')
    } finally {
      setTargetSaving(false)
    }
  }

  async function setDefaultTarget(accountID: string, targetID: string) {
    try {
      await postJSON('/api/im/targets/default', {
        account_id: accountID,
        target_id: targetID,
      })
      await refresh()
      setTargetFeedback('默认触达目标已更新。')
      setTargetError(null)
    } catch (err) {
      setTargetError(err instanceof Error ? err.message : '更新默认目标失败')
    }
  }

  async function deleteTarget(targetID: string) {
    if (!window.confirm('确定删除这个触达目标吗？')) return
    try {
      await postJSON('/api/im/targets/delete', {
        target_id: targetID,
      })
      await refresh()
      setTargetFeedback('触达目标已删除。')
      setTargetError(null)
    } catch (err) {
      setTargetError(err instanceof Error ? err.message : '删除触达目标失败')
    }
  }

  async function deleteAccount(accountID: string) {
    if (!window.confirm('确定删除这个微信账号吗？这会同时删除它下面的触达目标。')) return
    try {
      await postJSON('/api/im/accounts/delete', {
        account_id: accountID,
      })
      await refresh()
      setDeliveryFeedback('微信账号已删除。')
      setDeliveryError(null)
    } catch (err) {
      setDeliveryError(err instanceof Error ? err.message : '删除微信账号失败')
    }
  }

  async function sendDebugText() {
    if (!debugText.trim()) {
      setDebugTextError('请输入要发送的测试文本。')
      setDebugTextFeedback(null)
      return
    }

    setDebugTextSending(true)
    setDebugTextError(null)
    setDebugTextFeedback(null)
    try {
      const payload = await postJSON<{ receipt?: IMDeliveryReceipt }>('/api/im/debug/send-default', {
        text: debugText,
      })
      await refresh()
      setDebugText('')
      if (payload.receipt) {
        setDebugTextFeedback(`测试消息已发送到 ${payload.receipt.account.display_name || payload.receipt.account.remote_account_id} / ${payload.receipt.target.name}。`)
      } else {
        setDebugTextFeedback('测试消息发送成功。')
      }
    } catch (err) {
      setDebugTextError(err instanceof Error ? err.message : '发送测试消息失败')
    } finally {
      setDebugTextSending(false)
    }
  }

  async function sendDebugImage() {
    if (!debugImageFile) {
      setDebugImageError('请先选择一张图片。')
      setDebugImageFeedback(null)
      return
    }

    const payload = new FormData()
    payload.set('file', debugImageFile)
    payload.set('caption', debugImageCaption)

    setDebugImageSending(true)
    setDebugImageError(null)
    setDebugImageFeedback(null)
    try {
      const result = await postFormData<{ receipt?: IMDeliveryReceipt }>('/api/im/debug/send-image-default', payload)
      await refresh()
      setDebugImageFile(null)
      setDebugImageCaption('')
      setDebugImageInputKey((current) => current + 1)
      if (result.receipt) {
        const label = result.receipt.media_file_name || debugImageFile.name
        setDebugImageFeedback(`测试图片 ${label} 已发送到 ${result.receipt.account.display_name || result.receipt.account.remote_account_id} / ${result.receipt.target.name}。`)
      } else {
        setDebugImageFeedback('测试图片发送成功。')
      }
    } catch (err) {
      setDebugImageError(err instanceof Error ? err.message : '发送测试图片失败')
    } finally {
      setDebugImageSending(false)
    }
  }

  async function sendDebugFile() {
    if (!debugFileFile) {
      setDebugFileError('请先选择一个文件。')
      setDebugFileFeedback(null)
      return
    }

    const payload = new FormData()
    payload.set('file', debugFileFile)
    payload.set('caption', debugFileCaption)

    setDebugFileSending(true)
    setDebugFileError(null)
    setDebugFileFeedback(null)
    try {
      const result = await postFormData<{ receipt?: IMDeliveryReceipt }>('/api/im/debug/send-file-default', payload)
      await refresh()
      setDebugFileFile(null)
      setDebugFileCaption('')
      setDebugFileInputKey((current) => current + 1)
      if (result.receipt) {
        const label = result.receipt.media_file_name || debugFileFile.name
        setDebugFileFeedback(`测试文件 ${label} 已发送到 ${result.receipt.account.display_name || result.receipt.account.remote_account_id} / ${result.receipt.target.name}。`)
      } else {
        setDebugFileFeedback('测试文件发送成功。')
      }
    } catch (err) {
      setDebugFileError(err instanceof Error ? err.message : '发送测试文件失败')
    } finally {
      setDebugFileSending(false)
    }
  }

  return (
    <main className="settings-page">
      <section className="settings-hero-card">
        <div>
          <p className="eyebrow">SETTINGS CENTER</p>
          <h2>把设置拆成清晰的配置域</h2>
          <p className="hero-text">
            设置页不再把所有配置堆成一个长滚动页面，而是按配置域分开管理。
            左侧菜单负责切换，右侧只展示当前配置域的内容，让系统设置和 IM 配置各自独立。
          </p>
        </div>
        <div className="settings-hero-stats">
          <div className="metric-card metric-mint">
            <span>配置域</span>
            <strong>{settingsSections.length}</strong>
          </div>
          <div className="metric-card metric-cyan">
            <span>微信账号</span>
            <strong>{data.im.accounts.length}</strong>
          </div>
          <div className="metric-card metric-amber">
            <span>镜像状态</span>
            <strong>{data.settings.im_delivery_enabled ? '已开启' : '未开启'}</strong>
          </div>
        </div>
      </section>

      {error ? <div className="error-banner">接口异常：{error}</div> : null}

      <section className="settings-workspace">
        <aside className="panel settings-nav-card">
          <div className="panel-head compact">
            <div>
              <p className="eyebrow">SETTINGS MAP</p>
              <h3>配置导航</h3>
            </div>
          </div>

          <div className="settings-nav-list">
            {settingsSections.map((section) => (
              <button
                key={section.key}
                className={`settings-nav-item ${section.key === activeSection ? 'settings-nav-item-active' : ''}`}
                onClick={() => setActiveSection(section.key)}
                type="button"
              >
                <span>{section.eyebrow}</span>
                <strong>{section.title}</strong>
                <p>{section.description}</p>
              </button>
            ))}
          </div>
        </aside>

        <div className="settings-stage">
          <section className="panel settings-stage-card">
            <div className="panel-head compact">
              <div>
                <p className="eyebrow">{currentSection.eyebrow}</p>
                <h3>{currentSection.title}</h3>
              </div>
            </div>
            <p className="settings-stage-copy">{currentSection.description}</p>
          </section>

          {activeSection === 'system' ? (
            <section className="settings-grid-page settings-grid-single">
              <SessionSettingsPanel
                settingsError={settingsError}
                settingsFeedback={settingsFeedback}
                settingsSaving={settingsSaving}
                windowDirty={windowDirty}
                windowInput={windowInput}
                onSave={() => void saveSessionWindowSettings()}
                onWindowInputChange={(value) => {
                  setWindowInput(value)
                  setWindowDirty(true)
                  setSettingsFeedback(null)
                  setSettingsError(null)
                }}
              />
            </section>
          ) : (
            <section className="settings-grid-page">
              <IMDeliveryPanel
                accountID={deliveryAccountID}
                accounts={deliveryAccounts}
                dirty={deliveryDirty}
                enabled={deliveryEnabled}
                error={deliveryError}
                feedback={deliveryFeedback}
                saving={deliverySaving}
                targetID={deliveryTargetID}
                targets={deliveryTargets}
                onAccountChange={(value) => {
                  setDeliveryAccountID(value)
                  setDeliveryTargetID(selectBestTarget(data.im.targets, value))
                  setDeliveryDirty(true)
                  setDeliveryFeedback(null)
                  setDeliveryError(null)
                }}
                onEnabledChange={(value) => {
                  setDeliveryEnabled(value)
                  setDeliveryDirty(true)
                  setDeliveryFeedback(null)
                  setDeliveryError(null)
                }}
                onSave={() => void saveDeliverySettings()}
                onTargetChange={(value) => {
                  setDeliveryTargetID(value)
                  setDeliveryDirty(true)
                  setDeliveryFeedback(null)
                  setDeliveryError(null)
                }}
              />

              <IMDebugSendPanel
                account={savedDebugAccount}
                configDirty={deliveryDirty}
                imageCaption={debugImageCaption}
                imageError={debugImageError}
                imageFeedback={debugImageFeedback}
                imageFileName={debugImageFile?.name ?? null}
                imageInputKey={debugImageInputKey}
                imageSending={debugImageSending}
                fileCaption={debugFileCaption}
                fileError={debugFileError}
                fileFeedback={debugFileFeedback}
                fileFileName={debugFileFile?.name ?? null}
                fileInputKey={debugFileInputKey}
                fileSending={debugFileSending}
                target={savedDebugTarget}
                text={debugText}
                textError={debugTextError}
                textFeedback={debugTextFeedback}
                textSending={debugTextSending}
                onFileCaptionChange={(value) => {
                  setDebugFileCaption(value)
                  setDebugFileFeedback(null)
                  setDebugFileError(null)
                }}
                onFileFileChange={(file) => {
                  setDebugFileFile(file)
                  setDebugFileFeedback(null)
                  setDebugFileError(null)
                }}
                onSendFile={() => void sendDebugFile()}
                onImageCaptionChange={(value) => {
                  setDebugImageCaption(value)
                  setDebugImageFeedback(null)
                  setDebugImageError(null)
                }}
                onImageFileChange={(file) => {
                  setDebugImageFile(file)
                  setDebugImageFeedback(null)
                  setDebugImageError(null)
                }}
                onSendImage={() => void sendDebugImage()}
                onSendText={() => void sendDebugText()}
                onTextChange={(value) => {
                  setDebugText(value)
                  setDebugTextFeedback(null)
                  setDebugTextError(null)
                }}
              />

              <WeChatAccountsPanel
                accounts={data.im.accounts}
                loginBusy={loginPanel.open}
                onDeleteAccount={(accountID) => void deleteAccount(accountID)}
                onStartLogin={() => void startWeChatLogin()}
              />

              <IMTargetsPanel
                accountID={targetAccountID}
                accountTargets={targetFormTargets}
                accounts={data.im.accounts}
                error={targetError}
                feedback={targetFeedback}
                name={targetName}
                saving={targetSaving}
                setDefault={targetDefault}
                targetUserID={targetUserID}
                onAccountChange={(value) => {
                  setTargetAccountID(value)
                  setTargetFeedback(null)
                  setTargetError(null)
                }}
                onDeleteTarget={(targetID) => void deleteTarget(targetID)}
                onNameChange={setTargetName}
                onSave={() => void createTarget()}
                onSetDefaultChange={setTargetDefault}
                onSetDefaultTarget={(accountID, targetID) => void setDefaultTarget(accountID, targetID)}
                onTargetUserIDChange={setTargetUserID}
              />
            </section>
          )}
        </div>
      </section>

      <WeChatLoginPanel
        candidate={loginPanel.candidate}
        confirming={loginPanel.confirming}
        error={loginPanel.error}
        expiresAt={loginPanel.expiresAt}
        loading={loginPanel.loading}
        message={loginPanel.message}
        open={loginPanel.open}
        polling={loginPanel.polling}
        qrDataUrl={loginPanel.qrDataUrl}
        qrRawText={loginPanel.qrRawText}
        status={loginPanel.status}
        onClose={() => closeWeChatLogin()}
        onConfirm={() => void confirmWeChatLogin()}
      />
    </main>
  )
}
