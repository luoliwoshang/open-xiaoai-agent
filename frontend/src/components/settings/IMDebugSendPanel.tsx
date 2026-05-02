import { useEffect, useRef, useState } from 'react'
import { Send, Image, FileUp, X } from 'lucide-react'

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
  const [selectedImage, setSelectedImage] = useState<File | null>(null)
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [imagePreviewUrl, setImagePreviewUrl] = useState('')
  const imageRef = useRef<HTMLInputElement>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!selectedImage) {
      setImagePreviewUrl('')
      return
    }

    const nextUrl = URL.createObjectURL(selectedImage)
    setImagePreviewUrl(nextUrl)
    return () => URL.revokeObjectURL(nextUrl)
  }, [selectedImage])

  const clearImage = () => {
    setSelectedImage(null)
    if (imageRef.current) imageRef.current.value = ''
  }

  const clearFile = () => {
    setSelectedFile(null)
    if (fileRef.current) fileRef.current.value = ''
  }

  const handleSend = async () => {
    setSending(true)
    try {
      if (tab === 'text') {
        await onSendText(text)
        setText('')
      } else if (tab === 'image') {
        const file = selectedImage
        if (file) {
          await onSendImage(file, caption || undefined)
          clearImage()
          setCaption('')
        }
      } else {
        const file = selectedFile
        if (file) {
          await onSendFile(file, caption || undefined)
          clearFile()
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
              <input
                ref={imageRef}
                type="file"
                accept="image/*"
                style={{ display: 'none' }}
                onChange={(event) => setSelectedImage(event.target.files?.[0] ?? null)}
              />
              <div className="file-input-wrapper">
                <button className="btn btn-sm" type="button" onClick={() => imageRef.current?.click()}>
                  <Image /> 选择图片
                </button>
                <span className="debug-send-helper">
                  {selectedImage ? '已选择 1 张图片' : '支持常见图片格式'}
                </span>
              </div>
            </div>
            {selectedImage && (
              <div className="debug-preview-card">
                <div className="debug-preview-image-shell">
                  {imagePreviewUrl && (
                    <img
                      src={imagePreviewUrl}
                      alt={selectedImage.name}
                      className="debug-preview-image"
                    />
                  )}
                </div>
                <div className="debug-preview-meta">
                  <div className="debug-preview-title">{selectedImage.name}</div>
                  <div className="debug-preview-subtitle">
                    图片已选中 · {formatFileSize(selectedImage.size)}
                  </div>
                </div>
                <button className="debug-preview-clear" type="button" onClick={clearImage} aria-label="清除图片">
                  <X />
                </button>
              </div>
            )}
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
              <input
                ref={fileRef}
                type="file"
                style={{ display: 'none' }}
                onChange={(event) => setSelectedFile(event.target.files?.[0] ?? null)}
              />
              <div className="file-input-wrapper">
                <button className="btn btn-sm" type="button" onClick={() => fileRef.current?.click()}>
                  <FileUp /> 选择文件
                </button>
                <span className="debug-send-helper">
                  {selectedFile ? '已选择 1 个文件' : '选择后会显示文件信息'}
                </span>
              </div>
            </div>
            {selectedFile && (
              <div className="debug-preview-card is-file">
                <div className="debug-preview-file-icon">
                  <FileUp />
                </div>
                <div className="debug-preview-meta">
                  <div className="debug-preview-title">{selectedFile.name}</div>
                  <div className="debug-preview-subtitle">
                    文件已选中 · {formatFileSize(selectedFile.size)}
                  </div>
                </div>
                <button className="debug-preview-clear" type="button" onClick={clearFile} aria-label="清除文件">
                  <X />
                </button>
              </div>
            )}
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

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}
