import { useEffect, useMemo, useState, type Dispatch, type SetStateAction } from 'react'
import { IMDeliveryPanel } from '../components/settings/IMDeliveryPanel'
import { IMEventsPanel } from '../components/settings/IMEventsPanel'
import { IMTargetsPanel } from '../components/settings/IMTargetsPanel'
import { SessionSettingsPanel } from '../components/settings/SessionSettingsPanel'
import { WeChatAccountsPanel } from '../components/settings/WeChatAccountsPanel'
import { WeChatLoginPanel } from '../components/settings/WeChatLoginPanel'
import { fetchState, postJSON } from '../lib/api'
import { normalizeSettings, selectBestTarget } from '../lib/dashboard'
import type {
  DashboardState,
  SessionSettings,
  SettingsSnapshot,
  WeChatLoginStart,
  WeChatLoginStatus,
} from '../types'

type LoginPanelState = {
  loading: boolean
  polling: boolean
  sessionKey: string | null
  qrDataUrl: string | null
  qrRawText: string | null
  expiresAt: string | null
  status: WeChatLoginStatus['status'] | null
  message: string | null
  error: string | null
}

const emptyLoginState: LoginPanelState = {
  loading: false,
  polling: false,
  sessionKey: null,
  qrDataUrl: null,
  qrRawText: null,
  expiresAt: null,
  status: null,
  message: null,
  error: null,
}

type Props = {
  data: DashboardState
  error: string | null
  setData: Dispatch<SetStateAction<DashboardState>>
  refresh: () => Promise<void>
}

export function SettingsPage({ data, error, setData, refresh }: Props) {
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

  const [loginPanel, setLoginPanel] = useState<LoginPanelState>(emptyLoginState)

  const [targetAccountID, setTargetAccountID] = useState('')
  const [targetName, setTargetName] = useState('')
  const [targetUserID, setTargetUserID] = useState('')
  const [targetDefault, setTargetDefault] = useState(true)
  const [targetSaving, setTargetSaving] = useState(false)
  const [targetFeedback, setTargetFeedback] = useState<string | null>(null)
  const [targetError, setTargetError] = useState<string | null>(null)

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
    if (targetAccountID) return
    const firstAccount = data.im.accounts[0]?.id ?? ''
    if (firstAccount) {
      setTargetAccountID(firstAccount)
    }
  }, [data.im.accounts, targetAccountID])

  useEffect(() => {
    if (!loginPanel.polling || !loginPanel.sessionKey) return

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
          error: null,
          polling: nextStatus === 'pending' || nextStatus === 'scanned',
          sessionKey:
            nextStatus === 'pending' || nextStatus === 'scanned'
              ? current.sessionKey
              : null,
        }))

        if (nextStatus === 'confirmed') {
          const next = await fetchState()
          if (!active) return
          setData(next)
          setDeliveryFeedback('微信账号已登录，现在可以选择它作为 IM 文本触达渠道。')
          setDeliveryError(null)
        }
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
  }, [loginPanel.polling, loginPanel.sessionKey, setData])

  const deliveryTargets = useMemo(() => {
    return data.im.targets.filter((target) => target.account_id === deliveryAccountID)
  }, [data.im.targets, deliveryAccountID])

  const targetFormTargets = useMemo(() => {
    return data.im.targets.filter((target) => target.account_id === targetAccountID)
  }, [data.im.targets, targetAccountID])

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
      loading: true,
    })
    try {
      const payload = await postJSON<{ login?: WeChatLoginStart }>('/api/im/wechat/login/start', {})
      if (!payload.login) {
        throw new Error('登录二维码返回为空')
      }
      setLoginPanel({
        loading: false,
        polling: true,
        sessionKey: payload.login.session_key,
        qrDataUrl: payload.login.qr_code_data_url,
        qrRawText: payload.login.qr_raw_text,
        expiresAt: payload.login.expires_at,
        status: 'pending',
        message: '请使用微信扫描下方二维码。',
        error: null,
      })
    } catch (err) {
      setLoginPanel({
        ...emptyLoginState,
        error: err instanceof Error ? err.message : '启动微信登录失败',
      })
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

  return (
    <main className="settings-page">
      <section className="settings-hero-card">
        <div>
          <p className="eyebrow">SYSTEM SETTINGS</p>
          <h2>IM Gateway 与系统设置</h2>
          <p className="hero-text">
            这里单独负责运行期设置和微信账号管理。第一期只做微信文本触达：
            小爱的回复在设备播报成功后，会异步再镜像到你选中的微信目标。
          </p>
        </div>
        <div className="settings-hero-stats">
          <div className="metric-card metric-mint">
            <span>微信账号</span>
            <strong>{data.im.accounts.length}</strong>
          </div>
          <div className="metric-card metric-cyan">
            <span>触达目标</span>
            <strong>{data.im.targets.length}</strong>
          </div>
          <div className="metric-card metric-amber">
            <span>镜像状态</span>
            <strong>{data.settings.im_delivery_enabled ? '已开启' : '未开启'}</strong>
          </div>
        </div>
      </section>

      {error ? <div className="error-banner">接口异常：{error}</div> : null}

      <section className="settings-grid-page">
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

        <IMDeliveryPanel
          accountID={deliveryAccountID}
          accounts={data.im.accounts}
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

        <WeChatLoginPanel
          error={loginPanel.error}
          expiresAt={loginPanel.expiresAt}
          loading={loginPanel.loading}
          message={loginPanel.message}
          polling={loginPanel.polling}
          qrDataUrl={loginPanel.qrDataUrl}
          qrRawText={loginPanel.qrRawText}
          onStart={() => void startWeChatLogin()}
        />

        <WeChatAccountsPanel
          accounts={data.im.accounts}
          onDeleteAccount={(accountID) => void deleteAccount(accountID)}
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

        <IMEventsPanel events={data.im.events} />
      </section>
    </main>
  )
}

