import { useCallback, useEffect, useMemo, useState } from 'react'
import { BrainCircuit, FilePenLine, History, RefreshCw, Save } from 'lucide-react'
import { fetchMemoryFile, fetchMemoryLogs, saveMemoryFile } from '../lib/api'
import { formatTime } from '../lib/dashboard'
import type { MemoryManagedFile, MemoryUpdateLog, SettingsSnapshot } from '../types'

interface MemoryPageProps {
  settings: SettingsSnapshot
}

const MEMORY_KEY = 'main-voice'

type DiffRow =
  | { type: 'same'; value: string }
  | { type: 'before'; value: string }
  | { type: 'after'; value: string }

function buildDiffRows(before: string, after: string): DiffRow[] {
  const normalizedBefore = before.replace(/\r\n/g, '\n')
  const normalizedAfter = after.replace(/\r\n/g, '\n')
  if (normalizedBefore === normalizedAfter) return []

  const left = normalizedBefore.split('\n')
  const right = normalizedAfter.split('\n')
  const rows: DiffRow[] = []
  const total = Math.max(left.length, right.length)

  for (let i = 0; i < total; i += 1) {
    const beforeLine = left[i] ?? ''
    const afterLine = right[i] ?? ''
    if (beforeLine === afterLine) {
      rows.push({ type: 'same', value: beforeLine })
      continue
    }
    if (beforeLine !== '') rows.push({ type: 'before', value: beforeLine })
    if (afterLine !== '') rows.push({ type: 'after', value: afterLine })
  }

  return rows
}

function logPreview(log: MemoryUpdateLog): string {
  if (Array.isArray(log.messages) && log.messages.length > 0) {
    const first = log.messages[0]
    return `${first.role}：${first.content}`
  }
  const after = log.after.trim()
  if (!after) return '手动编辑了记忆文件'
  return after.split('\n')[0]
}

export function MemoryPage({ settings }: MemoryPageProps) {
  const [file, setFile] = useState<MemoryManagedFile | null>(null)
  const [draft, setDraft] = useState('')
  const [logs, setLogs] = useState<MemoryUpdateLog[]>([])
  const [selectedLogID, setSelectedLogID] = useState<string | null>(null)
  const [page, setPage] = useState(1)
  const [pageSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [hasMore, setHasMore] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [toast, setToast] = useState<string | null>(null)

  const loadFile = useCallback(async () => {
    const nextFile = await fetchMemoryFile(MEMORY_KEY)
    setFile(nextFile)
    setDraft(nextFile.content)
    return nextFile
  }, [])

  const loadLogs = useCallback(async (nextPage: number) => {
    const result = await fetchMemoryLogs(nextPage, pageSize, MEMORY_KEY)
    const items = Array.isArray(result.items) ? result.items : []
    setLogs(items)
    setTotal(result.total)
    setHasMore(result.has_more)
    setSelectedLogID((current) => current && items.some((item) => item.id === current) ? current : (items[0]?.id ?? null))
  }, [pageSize])

  const loadAll = useCallback(async (nextPage: number) => {
    setLoading(true)
    setError(null)
    try {
      await Promise.all([loadFile(), loadLogs(nextPage)])
    } catch (e) {
      setError(e instanceof Error ? e.message : '加载长期记忆失败')
    } finally {
      setLoading(false)
    }
  }, [loadFile, loadLogs])

  useEffect(() => {
    void loadAll(page)
  }, [loadAll, page, settings.memory_storage_dir])

  useEffect(() => {
    if (!toast) return
    const timer = window.setTimeout(() => setToast(null), 2500)
    return () => window.clearTimeout(timer)
  }, [toast])

  const selectedLog = useMemo(
    () => logs.find((item) => item.id === selectedLogID) ?? logs[0] ?? null,
    [logs, selectedLogID],
  )

  const diffRows = useMemo(
    () => buildDiffRows(selectedLog?.before ?? '', selectedLog?.after ?? ''),
    [selectedLog],
  )

  const handleSave = async () => {
    setSaving(true)
    setError(null)
    try {
      const nextFile = await saveMemoryFile(MEMORY_KEY, draft)
      setFile(nextFile)
      setDraft(nextFile.content)
      await loadLogs(1)
      setPage(1)
      setToast('长期记忆已保存')
    } catch (e) {
      setError(e instanceof Error ? e.message : '保存长期记忆失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div>
      <div className="page-header">
        <h2>长期记忆</h2>
        <div className="page-header-sub">
          主流程 reply 与 `complex_task` 会参考这份记忆；intent 不会读取它
        </div>
      </div>

      <div className="memory-page">
        <section className="memory-hero-card">
          <div>
            <div className="memory-eyebrow">MEMORY DEBUG</div>
            <h3>主语音长期记忆文件</h3>
            <p>这里维护的是 `main-voice` 这份长期记忆。你可以手动整理偏好、环境说明、常用服务信息；系统也会在一次主语音 session 自然结束后，整理那次完整会话并写入更新日志。</p>
          </div>
          <div className="memory-hero-meta">
            <div className="memory-meta-chip">
              <BrainCircuit />
              memory key：{MEMORY_KEY}
            </div>
            <div className="memory-meta-chip">
              <FilePenLine />
              目录：{settings.memory_storage_dir || '.open-xiaoai-agent/memory'}
            </div>
          </div>
        </section>

        {error && <div className="memory-error-banner">{error}</div>}

        <div className="memory-grid">
          <section className="memory-editor-card">
            <div className="memory-card-header">
              <div>
                <div className="memory-card-title">记忆正文</div>
                <div className="memory-card-sub">
                  {file?.path ? `文件：${file.path}` : '首次读取时会自动创建默认记忆文件'}
                </div>
              </div>
              <div className="memory-card-actions">
                <button className="btn btn-secondary" onClick={() => void loadAll(page)} disabled={loading || saving}>
                  <RefreshCw />
                  刷新
                </button>
                <button className="btn btn-primary" onClick={handleSave} disabled={loading || saving}>
                  <Save />
                  {saving ? '保存中...' : '保存记忆'}
                </button>
              </div>
            </div>
            <div className="memory-file-meta">
              <span>更新时间：{formatTime(file?.updated_at ?? '')}</span>
              <span>更新日志：{total} 条</span>
            </div>
            <textarea
              className="memory-editor-textarea"
              value={draft}
              disabled={loading}
              placeholder={loading ? '正在加载长期记忆...' : '在这里手动维护长期记忆内容'}
              onChange={(event) => setDraft(event.target.value)}
            />
          </section>

          <section className="memory-log-card">
            <div className="memory-card-header">
              <div>
                <div className="memory-card-title">更新日志</div>
                <div className="memory-card-sub">这里展示文件型记忆实现自己记录的变更轨迹，包括系统追加与手动编辑。</div>
              </div>
              <div className="memory-log-pager">
                <button className="btn btn-secondary" disabled={page <= 1 || loading} onClick={() => setPage((current) => Math.max(1, current - 1))}>
                  上一页
                </button>
                <span>{page}</span>
                <button className="btn btn-secondary" disabled={!hasMore || loading} onClick={() => setPage((current) => current + 1)}>
                  下一页
                </button>
              </div>
            </div>

            <div className="memory-log-list">
              {logs.length === 0 ? (
                <div className="memory-empty-state">还没有长期记忆更新记录。</div>
              ) : logs.map((item) => (
                <button
                  key={item.id}
                  className={`memory-log-item ${selectedLog?.id === item.id ? 'active' : ''}`}
                  onClick={() => setSelectedLogID(item.id)}
                >
                  <div className="memory-log-item-top">
                    <span className="memory-log-source">{item.source || 'memory'}</span>
                    <span>{formatTime(item.created_at)}</span>
                  </div>
                  <div className="memory-log-preview">{logPreview(item)}</div>
                </button>
              ))}
            </div>

            <div className="memory-diff-card">
              <div className="memory-diff-header">
                <div>
                  <div className="memory-card-title">变更 Diff</div>
                  <div className="memory-card-sub">
                    {selectedLog ? `${selectedLog.source || 'memory'} · ${formatTime(selectedLog.created_at)}` : '选择一条更新记录后查看'}
                  </div>
                </div>
                <History />
              </div>
              <div className="memory-diff-body">
                {!selectedLog ? (
                  <div className="memory-empty-state">当前页还没有可查看的更新记录。</div>
                ) : diffRows.length === 0 ? (
                  <div className="memory-empty-state">这条记录没有文本差异。</div>
                ) : (
                  diffRows.map((row, index) => (
                    <div key={`${row.type}-${index}`} className={`memory-diff-line ${row.type}`}>
                      <span className="memory-diff-prefix">
                        {row.type === 'before' ? '-' : row.type === 'after' ? '+' : '·'}
                      </span>
                      <code>{row.value || ' '}</code>
                    </div>
                  ))
                )}
              </div>
            </div>
          </section>
        </div>
      </div>

      {toast && <div className="toast success">{toast}</div>}
    </div>
  )
}
