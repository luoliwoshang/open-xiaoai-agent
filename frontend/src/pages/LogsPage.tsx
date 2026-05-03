import { useCallback, useEffect, useRef, useState } from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import type { LogEntry, LogPage } from '../types'
import { fetchLogs } from '../lib/api'

const PAGE_SIZES = [20, 50, 100]

export function LogsPage() {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [data, setData] = useState<LogPage | null>(null)
  const [error, setError] = useState<string | null>(null)
  const timer = useRef<ReturnType<typeof setInterval> | null>(null)

  const load = useCallback(async () => {
    try {
      const result = await fetchLogs(page, pageSize)
      setData(result)
      setError(null)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : '加载失败')
    }
  }, [page, pageSize])

  useEffect(() => {
    load()
    if (page === 1) {
      timer.current = setInterval(load, 2000)
    }
    return () => {
      if (timer.current) clearInterval(timer.current)
    }
  }, [load, page])

  const totalPages = data ? Math.ceil(data.total / pageSize) : 1

  const levelColor: Record<string, string> = {
    debug: 'log-level debug',
    info: 'log-level info',
    warn: 'log-level warn',
    error: 'log-level error',
    fatal: 'log-level fatal',
  }

  return (
    <div>
      <div className="page-header">
        <h2>调试日志</h2>
        <div className="page-header-sub">
          {page === 1 ? '实时刷新中，用于运行排障' : '自动刷新已暂停'}
        </div>
      </div>

      <div className="logs-container">
        <div className="logs-controls">
          <div className="logs-stats">
            {data && (
              <>
                <div className="logs-stat">总计 <strong>{data.total}</strong></div>
                <div className="logs-stat">第 <strong>{data.page}</strong> / {totalPages} 页</div>
                <div className="logs-stat">本页 <strong>{data.items.length}</strong> 条</div>
              </>
            )}
          </div>
          <div className="page-size-tabs">
            {PAGE_SIZES.map((size) => (
              <button
                key={size}
                className={`page-size-tab ${pageSize === size ? 'active' : ''}`}
                onClick={() => { setPageSize(size); setPage(1) }}
              >
                {size}
              </button>
            ))}
          </div>
        </div>

        {error && (
          <div style={{ padding: 16, color: 'var(--red)', fontFamily: 'var(--font-mono)', fontSize: 12 }}>
            错误: {error}
          </div>
        )}

        <div className="logs-list">
          {data?.items.map((log: LogEntry) => (
            <div key={log.id} className="log-entry">
              <span className={levelColor[log.level] || 'log-level debug'}>{log.level}</span>
              <span className="log-time">{new Date(log.created_at).toLocaleTimeString('zh-CN')}</span>
              <span className="log-source">{log.source}</span>
              <span className="log-message">{log.message || log.raw}</span>
            </div>
          ))}
        </div>

        {data && totalPages > 1 && (
          <div className="logs-pagination">
            <button
              className="pagination-btn"
              disabled={page <= 1}
              onClick={() => setPage((p) => p - 1)}
            >
              <ChevronLeft style={{ width: 14, height: 14 }} />
              上一页
            </button>
            <span className="pagination-info">第 {page} / {totalPages} 页</span>
            <button
              className="pagination-btn"
              disabled={!data.has_more}
              onClick={() => setPage((p) => p + 1)}
            >
              下一页
              <ChevronRight style={{ width: 14, height: 14 }} />
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
