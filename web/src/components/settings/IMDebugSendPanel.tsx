import type { IMAccount, IMTarget } from '../../types'

type Props = {
  account: IMAccount | null
  target: IMTarget | null
  configDirty: boolean
  text: string
  textSending: boolean
  textFeedback: string | null
  textError: string | null
  imageCaption: string
  imageFileName: string | null
  imageInputKey: number
  imageSending: boolean
  imageFeedback: string | null
  imageError: string | null
  fileCaption: string
  fileFileName: string | null
  fileInputKey: number
  fileSending: boolean
  fileFeedback: string | null
  fileError: string | null
  onTextChange: (value: string) => void
  onSendText: () => void
  onImageCaptionChange: (value: string) => void
  onImageFileChange: (file: File | null) => void
  onSendImage: () => void
  onFileCaptionChange: (value: string) => void
  onFileFileChange: (file: File | null) => void
  onSendFile: () => void
}

export function IMDebugSendPanel({
  account,
  target,
  configDirty,
  text,
  textSending,
  textFeedback,
  textError,
  imageCaption,
  imageFileName,
  imageInputKey,
  imageSending,
  imageFeedback,
  imageError,
  fileCaption,
  fileFileName,
  fileInputKey,
  fileSending,
  fileFeedback,
  fileError,
  onTextChange,
  onSendText,
  onImageCaptionChange,
  onImageFileChange,
  onSendImage,
  onFileCaptionChange,
  onFileFileChange,
  onSendFile,
}: Props) {
  const ready = Boolean(account && target)

  return (
    <article className="panel settings-panel panel-wide">
      <div className="panel-head compact">
        <div>
          <p className="eyebrow">IM DEBUG</p>
          <h3>手动调试发送</h3>
        </div>
      </div>

      <div className="settings-form">
        <p className="settings-note">
          这里始终命中当前已经保存的默认渠道，不依赖自动镜像开关。适合单独确认这条触达到底通不通。
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
            上方还有未保存的修改；这里仍然会命中“已经保存”的默认渠道。
          </div>
        ) : null}

        <label className="settings-field">
          <span>测试文本</span>
          <textarea
            className="settings-textarea"
            disabled={!ready || textSending}
            placeholder="例如：这是一条默认渠道调试消息。"
            rows={4}
            value={text}
            onChange={(event) => onTextChange(event.target.value)}
          />
        </label>

        <div className="settings-actions">
          <button
            className="settings-button"
            disabled={!ready || textSending || !text.trim()}
            onClick={() => onSendText()}
            type="button"
          >
            {textSending ? '发送中...' : '发送测试消息'}
          </button>
          <span className="settings-note">这条链路会同步验证默认渠道是否真的可用。</span>
        </div>

        {textFeedback ? <div className="settings-feedback">{textFeedback}</div> : null}
        {textError ? <div className="error-banner settings-error">{textError}</div> : null}

        <label className="settings-field">
          <span>测试图片</span>
          <input
            accept="image/*"
            className="settings-input"
            disabled={!ready || imageSending}
            key={imageInputKey}
            type="file"
            onChange={(event) => onImageFileChange(event.target.files?.[0] ?? null)}
          />
          <span className="settings-note">
            {imageFileName ? `已选择：${imageFileName}` : '只接受图片文件。'}
          </span>
        </label>

        <label className="settings-field">
          <span>图片说明</span>
          <textarea
            className="settings-textarea"
            disabled={!ready || imageSending}
            placeholder="例如：这是一张默认渠道调试图片。"
            rows={3}
            value={imageCaption}
            onChange={(event) => onImageCaptionChange(event.target.value)}
          />
        </label>

        <div className="settings-actions">
          <button
            className="settings-button"
            disabled={!ready || imageSending || !imageFileName}
            onClick={() => onSendImage()}
            type="button"
          >
            {imageSending ? '发送中...' : '发送测试图片'}
          </button>
          <span className="settings-note">如果填写了图片说明，微信侧会先收到一条文字，再收到图片。</span>
        </div>

        {imageFeedback ? <div className="settings-feedback">{imageFeedback}</div> : null}
        {imageError ? <div className="error-banner settings-error">{imageError}</div> : null}

        <label className="settings-field">
          <span>测试文件</span>
          <input
            className="settings-input"
            disabled={!ready || fileSending}
            key={fileInputKey}
            type="file"
            onChange={(event) => onFileFileChange(event.target.files?.[0] ?? null)}
          />
          <span className="settings-note">
            {fileFileName ? `已选择：${fileFileName}` : '可以发送任意文件。'}
          </span>
        </label>

        <label className="settings-field">
          <span>文件说明</span>
          <textarea
            className="settings-textarea"
            disabled={!ready || fileSending}
            placeholder="例如：这是本次调试要发送的文件。"
            rows={3}
            value={fileCaption}
            onChange={(event) => onFileCaptionChange(event.target.value)}
          />
        </label>

        <div className="settings-actions">
          <button
            className="settings-button"
            disabled={!ready || fileSending || !fileFileName}
            onClick={() => onSendFile()}
            type="button"
          >
            {fileSending ? '发送中...' : '发送测试文件'}
          </button>
          <span className="settings-note">如果填写了文件说明，微信侧会先收到一条文字，再收到文件。</span>
        </div>

        {fileFeedback ? <div className="settings-feedback">{fileFeedback}</div> : null}
        {fileError ? <div className="error-banner settings-error">{fileError}</div> : null}
      </div>
    </article>
  )
}
