import { useState, useRef } from 'react'
import { Send, Image, FileUp } from 'lucide-react'

interface IMDebugSendPanelProps {
  onSendText: (text: string) => Promise<void>
  onSendImage: (file: File, caption?: string) => Promise<void>
  onSendFile: (file: File, caption?: string) => Promise<void>
}

type DebugTab = 'text' | 'image' | 'file'

export function IMDebugSendPanel({ onSendText, onSendImage, onSendFile }: IMDebugSendPanelProps) {
  const [tab, setTab] = useState<DebugTab>('text')
  const [text, setText] = useState('')
  const [caption, setCaption] = useState('')
  const [sending, setSending] = useState(false)
  const imageRef = useRef<HTMLInputElement>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const handleSend = async () => {
    setSending(true)
    try {
      if (tab === 'text') {
        await onSendText(text)
        setText('')
      } else if (tab === 'image') {
        const file = imageRef.current?.files?.[0]
        if (file) {
          await onSendImage(file, caption || undefined)
          if (imageRef.current) imageRef.current.value = ''
          setCaption('')
        }
      } else {
        const file = fileRef.current?.files?.[0]
        if (file) {
          await onSendFile(file, caption || undefined)
          if (fileRef.current) fileRef.current.value = ''
          setCaption('')
        }
      }
    } finally {
      setSending(false)
    }
  }

  return (
    <div className="settings-section">
      <div className="settings-section-header">
        <div className="settings-section-title">调试发送</div>
        <div className="settings-section-desc">向默认目标发送测试消息</div>
      </div>
      <div className="settings-section-body">
        <div className="debug-send-tabs">
          <button className={`debug-send-tab ${tab === 'text' ? 'active' : ''}`} onClick={() => setTab('text')}>
            <Send style={{ width: 12, height: 12, marginRight: 4 }} />
            文本
          </button>
          <button className={`debug-send-tab ${tab === 'image' ? 'active' : ''}`} onClick={() => setTab('image')}>
            <Image style={{ width: 12, height: 12, marginRight: 4 }} />
            图片
          </button>
          <button className={`debug-send-tab ${tab === 'file' ? 'active' : ''}`} onClick={() => setTab('file')}>
            <FileUp style={{ width: 12, height: 12, marginRight: 4 }} />
            文件
          </button>
        </div>

        {tab === 'text' && (
          <div className="form-group">
            <textarea
              className="form-textarea"
              placeholder="输入要发送的文本..."
              value={text}
              onChange={(e) => setText(e.target.value)}
            />
          </div>
        )}

        {tab === 'image' && (
          <>
            <div className="form-group">
              <input ref={imageRef} type="file" accept="image/*" style={{ display: 'none' }} />
              <div className="file-input-wrapper">
                <button className="btn btn-sm" onClick={() => imageRef.current?.click()}>
                  <Image /> 选择图片
                </button>
              </div>
            </div>
            <div className="form-group">
              <input
                className="form-input"
                placeholder="图片说明（可选）"
                value={caption}
                onChange={(e) => setCaption(e.target.value)}
              />
            </div>
          </>
        )}

        {tab === 'file' && (
          <>
            <div className="form-group">
              <input ref={fileRef} type="file" style={{ display: 'none' }} />
              <div className="file-input-wrapper">
                <button className="btn btn-sm" onClick={() => fileRef.current?.click()}>
                  <FileUp /> 选择文件
                </button>
              </div>
            </div>
            <div className="form-group">
              <input
                className="form-input"
                placeholder="文件说明（可选）"
                value={caption}
                onChange={(e) => setCaption(e.target.value)}
              />
            </div>
          </>
        )}

        <button className="btn btn-primary" onClick={handleSend} disabled={sending}>
          <Send />
          {sending ? '发送中...' : '发送'}
        </button>
      </div>
    </div>
  )
}
