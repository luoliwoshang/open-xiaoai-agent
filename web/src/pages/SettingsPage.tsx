import { BellRing, MessageSquareHeart, Settings2, SmartphoneCharging } from 'lucide-react'
import { useEffect, useMemo, useState, type Dispatch, type SetStateAction } from 'react'
import { IMDebugSendPanel } from '../components/settings/IMDebugSendPanel'
import { IMDeliveryPanel } from '../components/settings/IMDeliveryPanel'
import { IMTargetsPanel } from '../components/settings/IMTargetsPanel'
import { SessionSettingsPanel } from '../components/settings/SessionSettingsPanel'
import { WeChatAccountsPanel } from '../components/settings/WeChatAccountsPanel'
import { WeChatLoginPanel } from '../components/settings/WeChatLoginPanel'
import { EmptyState } from '../components/ui/EmptyState'
import { PillTabs } from '../components/ui/PillTabs'
import { SectionCard } from '../components/ui/SectionCard'
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
type IMViewKey = 'delivery' | 'debug' | 'accounts' | 'targets'

type SettingsSection = {
  key: SettingsSectionKey
  eyebrow: string
  title: string
  description: string
  Icon: typeof Settings2
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
    eyebrow: 'SYSTEM CARE',
    title: '系统设置',
    description: '把会话节奏和全局行为调成更适合你的日常陪伴方式。',
    Icon: Settings2,
  },
  {
    key: 'im',
    eyebrow: 'IM GATEWAY',
    title: '触达配置',
    description: '管理微信账号、默认触达、调试发送，以及后续要发到哪里。',
    Icon: SmartphoneCharging,
  },
]

const imViews: Array<{ key: IMViewKey; label: string; caption: string }> = [
  { key: 'debug', label: '手动调试', caption: '文本 / 图片 / 文件' },
  { key: 'delivery', label: '镜像规则', caption: '自动回复怎么转发' },
  { key: 'accounts', label: '微信账号', caption: '扫码与账号管理' },
  { key: 'targets', label: '触达对象', caption: '谁会收到消息' },
]

type Props = {
  data: DashboardState
  error: string | null
  setData: Dispatch<SetStateAction<DashboardState>>
  refresh: () => Promise<void>
}

export function SettingsPage({ data, error, setData, refresh }: Props) {
  const [activeSection, setActiveSection] = useState<SettingsSectionKey>('system')
  const [activeIMView, setActiveIMView] = useState<IMViewKey>('debug')
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
      setSettingsFeedback('新的会话窗口已经生效。')
    } catch (err) {
      setSettingsError(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSettingsSaving(false)
    }
  }

  async function saveDeliverySettings() {
    if (deliveryEnabled && (!deliveryAccountID || !deliveryTargetID)) {
      setDeliveryError('先选好一个账号和一个触达对象，再打开自动镜像。')
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
      setDeliveryFeedback('默认触达规则已经保存。')
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
        message: '请用微信扫描这张二维码。',
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
      setDeliveryFeedback('微信账号已经加入，可以继续配置默认触达。')
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
      setTargetFeedback('触达对象已经保存。')
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
      setTargetFeedback('默认触达对象已经更新。')
      setTargetError(null)
    } catch (err) {
      setTargetError(err instanceof Error ? err.message : '更新默认目标失败')
    }
  }

  async function deleteTarget(targetID: string) {
    if (!window.confirm('确定删除这个触达对象吗？')) return
    try {
      await postJSON('/api/im/targets/delete', {
        target_id: targetID,
      })
      await refresh()
      setTargetFeedback('触达对象已删除。')
      setTargetError(null)
    } catch (err) {
      setTargetError(err instanceof Error ? err.message : '删除触达目标失败')
    }
  }

  async function deleteAccount(accountID: string) {
    if (!window.confirm('确定删除这个微信账号吗？这会同时删除它下面的触达对象。')) return
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
        setDebugTextFeedback(`测试消息已发到 ${payload.receipt.target.name}。`)
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
        setDebugImageFeedback(`测试图片 ${label} 已经发出。`)
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
        setDebugFileFeedback(`测试文件 ${label} 已经发出。`)
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
    <main className="page-shell settings-shell">
      <header className="page-hero">
        <div className="page-hero-copy">
          <p className="section-eyebrow">CARE & DELIVERY</p>
          <h2>把小爱的设置收成舒服、安静、不会乱跑的几个区域</h2>
          <p>设置页会固定住整体结构。左边选配置域，右边只做当前这件事，不再把所有表单铺成一条长走廊。</p>
        </div>

        <div className="hero-metrics">
          <article className="hero-metric hero-metric-peach">
            <span><Settings2 size={16} /> 配置域</span>
            <strong>{settingsSections.length}</strong>
            <small>系统与触达分开</small>
          </article>
          <article className="hero-metric hero-metric-mint">
            <span><BellRing size={16} /> 自动镜像</span>
            <strong>{data.settings.im_delivery_enabled ? '已开启' : '未开启'}</strong>
            <small>只影响默认触达</small>
          </article>
          <article className="hero-metric hero-metric-sky">
            <span><MessageSquareHeart size={16} /> 微信账号</span>
            <strong>{data.im.accounts.length}</strong>
            <small>已经接入的账号数</small>
          </article>
        </div>
      </header>

      {error ? <div className="banner-error">接口暂时有点卡住了：{error}</div> : null}

      <section className="settings-layout">
        <aside className="settings-sidebar">
          <SectionCard
            className="settings-menu-card"
            description="选一个区域，再在右边专心把这件事做好。"
            eyebrow="SETTINGS MAP"
            title="配置导航"
          >
            <div className="settings-menu-list">
              {settingsSections.map((section) => {
                const active = section.key === activeSection
                const Icon = section.Icon
                return (
                  <button
                    key={section.key}
                    className={`settings-menu-item ${active ? 'settings-menu-item-active' : ''}`}
                    onClick={() => setActiveSection(section.key)}
                    type="button"
                  >
                    <span className="settings-menu-item-icon"><Icon size={18} /></span>
                    <span className="settings-menu-item-copy">
                      <strong>{section.title}</strong>
                      <small>{section.description}</small>
                    </span>
                  </button>
                )
              })}
            </div>
          </SectionCard>
        </aside>

        <div className="settings-stage">
          <SectionCard
            className="settings-stage-card"
            description={currentSection.description}
            eyebrow={currentSection.eyebrow}
            title={currentSection.title}
          />

          {activeSection === 'system' ? (
            <div className="settings-content-single">
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

              <SectionCard
                className="helper-card"
                description="会话窗口越长，小爱越容易延续刚刚那段对话；越短，越像重新开始一轮新聊天。"
                eyebrow="GENTLE TIP"
                title="怎么理解这个设置"
              >
                <div className="helper-points">
                  <p>日常陪伴型使用可以保留默认值 300 秒。</p>
                  <p>如果你希望切话题更干脆，可以调短一些。</p>
                  <p>这里现在只保留滑动窗口策略，不再暴露多余开关。</p>
                </div>
              </SectionCard>
            </div>
          ) : (
            <div className="settings-content-stack">
              <PillTabs className="settings-pill-tabs" tabs={imViews} value={activeIMView} onChange={setActiveIMView} />

              {activeIMView === 'delivery' ? (
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
              ) : null}

              {activeIMView === 'debug' ? (
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
              ) : null}

              {activeIMView === 'accounts' ? (
                <WeChatAccountsPanel
                  accounts={data.im.accounts}
                  loginBusy={loginPanel.open}
                  onDeleteAccount={(accountID) => void deleteAccount(accountID)}
                  onStartLogin={() => void startWeChatLogin()}
                />
              ) : null}

              {activeIMView === 'targets' ? (
                data.im.accounts.length > 0 ? (
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
                ) : (
                  <SectionCard className="helper-card" eyebrow="TARGETS" title="先加一个微信账号">
                    <EmptyState
                      title="还没有可配置的账号"
                      description="先去“微信账号”里扫码接入一个账号，再回来管理默认触达对象。"
                    />
                  </SectionCard>
                )
              ) : null}
            </div>
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
