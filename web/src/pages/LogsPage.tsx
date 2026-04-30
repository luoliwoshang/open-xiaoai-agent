import { AlertCircle, Clock3, RefreshCw, ScrollText } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '../components/ui/EmptyState'
import { PillTabs } from '../components/ui/PillTabs'
import { SectionCard } from '../components/ui/SectionCard'
import { fetchLogsPage } from '../lib/api'
import { formatTime } from '../lib/dashboard'
import type { LogEntry, LogPage } from '../types'

const emptyLogPage: LogPage = {
  items: [],
  page: 1,
  page_size: 50,
  total: 0,
  has_more: false,
}

const levelLabels: Record<LogEntry['level'], string> = {
  debug: '调试',
  info: '信息',
  warn: '警告',
  error: '错误',
  fatal: '致命',
}

const sizeTabs = [
  { key: '20', label: '20 条', caption: '更轻' },
  { key: '50', label: '50 条', caption: '默认' },
  { key: '100', label: '100 条', caption: '更密' },
] as const

export function LogsPage() {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(50)
  const [data, setData] = useState<LogPage>(emptyLogPage)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let active = true

    async function refresh() {
      try {
        const next = await fetchLogsPage(page, pageSize)
        if (!active) return
        setData(next)
        setError(null)
      } catch (err) {
        if (!active) return
        setError(err instanceof Error ? err.message : '日志加载失败')
      } finally {
        if (active) {
          setLoading(false)
        }
      }
    }

    void refresh()
    if (page !== 1) {
      return () => {
        active = false
      }
    }

    const timer = window.setInterval(() => {
      void refresh()
    }, 2000)
    return () => {
      active = false
      window.clearInterval(timer)
    }
  }, [page, pageSize])

  const pageCount = useMemo(() => {
    if (data.total === 0) return 1
    return Math.max(1, Math.ceil(data.total / data.page_size))
  }, [data.page_size, data.total])

  async function refreshNow() {
    try {
      setLoading(true)
      const next = await fetchLogsPage(page, pageSize)
      setData(next)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : '日志加载失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <main className="page-shell logs-shell">
      <header className="page-hero">
        <div className="page-hero-copy">
          <p className="section-eyebrow">QUIET TRACE</p>
          <h2>把后端的每一步脚印摊开看，但不要把页面弄得吵闹</h2>
          <p>日志页只做三件事：切分页、看当前页、在第一页时自动跟着刷新。排查时不再被过重的装饰打断。</p>
        </div>

        <div className="hero-metrics">
          <article className="hero-metric hero-metric-peach">
            <span><ScrollText size={16} /> 总日志</span>
            <strong>{data.total}</strong>
            <small>已经落库的记录数</small>
          </article>
          <article className="hero-metric hero-metric-mint">
            <span><Clock3 size={16} /> 当前页</span>
            <strong>{data.page} / {pageCount}</strong>
            <small>翻页时不会自动跳动</small>
          </article>
          <article className="hero-metric hero-metric-sky">
            <span><AlertCircle size={16} /> 当前页条数</span>
            <strong>{data.items.length}</strong>
            <small>倒序紧凑展示</small>
          </article>
        </div>
      </header>

      {error ? <div className="banner-error">日志接口现在有点不顺：{error}</div> : null}

      <section className="logs-layout">
        <SectionCard
          actions={(
            <button className="icon-button" disabled={loading} onClick={() => void refreshNow()} type="button">
              <RefreshCw size={16} />
              {loading ? '刷新中' : '刷新当前页'}
            </button>
          )}
          className="logs-toolbar-card"
          description="第一页会自动刷新；翻到别页时会停下来，让你安静看完。"
          eyebrow="LOG CONTROL"
          title="分页与刷新"
        >
          <div className="logs-toolbar-body">
            <PillTabs
              tabs={sizeTabs.map((tab) => ({ ...tab }))}
              value={String(pageSize) as (typeof sizeTabs)[number]['key']}
              onChange={(value) => {
                setPageSize(Number(value))
                setPage(1)
              }}
            />

            <div className="logs-pagination">
              <button className="secondary-button" disabled={page <= 1 || loading} onClick={() => setPage((current) => current - 1)} type="button">
                上一页
              </button>
              <span>第 {page} 页</span>
              <button className="secondary-button" disabled={!data.has_more || loading} onClick={() => setPage((current) => current + 1)} type="button">
                下一页
              </button>
            </div>
          </div>
        </SectionCard>

        <SectionCard
          className="logs-stream-card"
          description="按时间倒序紧凑排布，适合复制给人或 AI 继续排查。"
          eyebrow="LIVE STRIP"
          title="后端日志流"
        >
          {data.items.length === 0 ? (
            <EmptyState title="还没有后端日志" description="等后端开始跑起来，这里就会慢慢积累每一条记录。" />
          ) : (
            <div className="logs-stream-list">
              {data.items.map((item) => (
                <article className="log-line" key={item.id}>
                  <div className="log-line-head">
                    <span className={`log-level log-level-${item.level}`}>{levelLabels[item.level]}</span>
                    <span>{formatTime(item.created_at)}</span>
                    <code>{item.source || 'unknown'}</code>
                  </div>
                  <p className="log-line-message">{item.message || '—'}</p>
                  <code className="log-line-raw">{item.raw}</code>
                </article>
              ))}
            </div>
          )}
        </SectionCard>
      </section>
    </main>
  )
}
