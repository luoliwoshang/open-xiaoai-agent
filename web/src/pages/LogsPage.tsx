import { useEffect, useMemo, useState } from 'react'
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

  const latestLog = data.items[0] ?? null

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
    <main className="logs-page">
      <section className="settings-hero-card logs-hero-card">
        <div>
          <p className="eyebrow">BACKEND LOGS</p>
          <h2>后端日志中心</h2>
          <p className="hero-text">
            这里只看 Go server 的运行日志。日志会带上时间、源文件和原始消息，适合直接把当前页交给 AI 辅助排查。
          </p>
        </div>
        <div className="settings-hero-stats">
          <div className="metric-card metric-cyan">
            <span>总日志数</span>
            <strong>{data.total}</strong>
          </div>
          <div className="metric-card metric-amber">
            <span>当前分页</span>
            <strong>
              {data.page} / {pageCount}
            </strong>
          </div>
          <div className="metric-card metric-mint">
            <span>最新来源</span>
            <strong>{latestLog?.source || '—'}</strong>
          </div>
        </div>
      </section>

      <section className="panel logs-toolbar">
        <div className="panel-head compact">
          <div>
            <p className="eyebrow">LOG CONTROLS</p>
            <h3>分页浏览</h3>
          </div>
        </div>

        <div className="settings-actions">
          <label className="settings-field logs-size-field">
            <span>每页条数</span>
            <select
              className="settings-select"
              value={pageSize}
              onChange={(event) => {
                setPageSize(Number(event.target.value))
                setPage(1)
              }}
            >
              <option value={20}>20</option>
              <option value={50}>50</option>
              <option value={100}>100</option>
            </select>
          </label>

          <button className="ghost-button" disabled={loading} onClick={() => void refreshNow()} type="button">
            {loading ? '加载中...' : '刷新当前页'}
          </button>

          <span className="settings-note">
            第 1 页会自动刷新，其它页保持静态，避免你翻页排障时被新日志打断。
          </span>
        </div>

        <div className="settings-actions">
          <button className="ghost-button" disabled={page <= 1 || loading} onClick={() => setPage((current) => current - 1)} type="button">
            上一页
          </button>
          <button
            className="ghost-button"
            disabled={!data.has_more || loading}
            onClick={() => setPage((current) => current + 1)}
            type="button"
          >
            下一页
          </button>
          <span className="panel-meta">
            {loading ? '正在加载日志...' : `本页 ${data.items.length} 条`}
          </span>
        </div>
      </section>

      {error ? <div className="error-banner logs-error-banner">日志接口异常：{error}</div> : null}

      <section className="panel logs-stream">
        <div className="panel-head compact">
          <div>
            <p className="eyebrow">LOG STREAM</p>
            <h3>紧凑日志流</h3>
          </div>
          <span className="panel-meta">按时间倒序展示，适合连续排查。</span>
        </div>

        {data.items.length === 0 ? (
          <div className="empty-card">当前还没有后端日志。</div>
        ) : (
          <div className="logs-stream-list">
            {data.items.map((item) => (
              <article className="log-row" key={item.id}>
                <div className="log-row-primary">
                  <span className={`badge badge-log badge-log-${item.level}`}>{levelLabels[item.level]}</span>
                  <span className="panel-meta">{formatTime(item.created_at)}</span>
                  <code className="log-source">{item.source || 'unknown'}</code>
                </div>
                <p className="log-row-message">{item.message || '—'}</p>
                <code className="log-row-raw">{item.raw}</code>
              </article>
            ))}
          </div>
        )}
      </section>
    </main>
  )
}
