import { Ban, CircleCheckBig, CircleOff, FolderKanban, PlayCircle, RefreshCw } from 'lucide-react'
import { useEffect, useMemo, useState, type Dispatch, type SetStateAction } from 'react'
import { countByState, formatTime, latest } from '../lib/dashboard'
import type { ClaudeRecord, ConversationSnapshot, DashboardState, TaskEvent } from '../types'
import { TaskDetailPane } from '../components/dashboard/TaskDetailPane'
import { TaskListPane } from '../components/dashboard/TaskListPane'
import {
  ClaudeRecordCard,
  RecentConversationCard,
  TaskArtifactsCard,
  TaskEventsCard,
} from '../components/dashboard/TaskSideCards'

type Props = {
  data: DashboardState
  loading: boolean
  error: string | null
  setData: Dispatch<SetStateAction<DashboardState>>
}

export function DashboardPage({ data, loading, error, setData: _setData }: Props) {
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null)
  const [lastRefreshedAt, setLastRefreshedAt] = useState(() => new Date())

  useEffect(() => {
    if (data.tasks.length === 0) {
      setSelectedTaskId(null)
      return
    }
    if (selectedTaskId && data.tasks.some((task) => task.id === selectedTaskId)) {
      return
    }
    setSelectedTaskId(data.tasks[0].id)
  }, [data.tasks, selectedTaskId])

  useEffect(() => {
    if (loading) return
    setLastRefreshedAt(new Date())
  }, [data, loading])

  const heroTask = latest(data.tasks)
  const selectedTask = data.tasks.find((task) => task.id === selectedTaskId) ?? heroTask ?? null

  const eventsByTask = useMemo(() => {
    const grouped = new Map<string, TaskEvent[]>()
    for (const event of data.events) {
      const items = grouped.get(event.task_id)
      if (items) {
        items.push(event)
      } else {
        grouped.set(event.task_id, [event])
      }
    }
    return grouped
  }, [data.events])

  const selectedEvents = selectedTask ? eventsByTask.get(selectedTask.id) ?? [] : []
  const selectedArtifacts = useMemo(() => {
    if (!selectedTask) return []
    return data.artifacts.filter((artifact) => artifact.task_id === selectedTask.id)
  }, [data.artifacts, selectedTask])

  const claudeRecord = useMemo<ClaudeRecord | null>(() => {
    if (!selectedTask) return null
    return data.claude_records.find((record) => record.task_id === selectedTask.id) ?? null
  }, [data.claude_records, selectedTask])

  const activeConversation = useMemo<ConversationSnapshot | null>(() => {
    return data.conversations[0] ?? null
  }, [data.conversations])

  const stats = useMemo(() => {
    return [
      { label: '任务总数', value: data.tasks.length, Icon: FolderKanban, tone: 'neutral' },
      { label: '运行中', value: countByState(data.tasks, 'running'), Icon: PlayCircle, tone: 'running' },
      { label: '已完成', value: countByState(data.tasks, 'completed'), Icon: CircleCheckBig, tone: 'completed' },
      { label: '失败', value: countByState(data.tasks, 'failed'), Icon: CircleOff, tone: 'failed' },
      { label: '已取消', value: countByState(data.tasks, 'canceled'), Icon: Ban, tone: 'canceled' },
    ]
  }, [data.tasks])

  const assistantRuntimePill = useMemo(() => {
    if (data.assistant.busy) {
      return {
        label: '语音处理中',
        className: 'service-pill service-pill-busy',
      }
    }
    if (data.assistant.result_report_ready) {
      return {
        label: '结果待汇报',
        className: 'service-pill service-pill-pending',
      }
    }
    if (!data.assistant.has_session) {
      return {
        label: '等待连接',
        className: 'service-pill service-pill-neutral',
      }
    }
    return {
      label: '语音空闲',
      className: 'service-pill service-pill-neutral',
    }
  }, [data.assistant])

  return (
    <main className="page-shell dashboard-shell dashboard-page">
      <header className="dashboard-header">
        <div className="dashboard-header-title">
          <div className="dashboard-title-row">
            <h2>Dashboard 首页</h2>
            <div className="dashboard-title-pills">
              <span className={`service-pill ${error ? 'service-pill-danger' : ''}`}>{error ? '服务异常' : '服务正常'}</span>
              <span className={assistantRuntimePill.className}>{assistantRuntimePill.label}</span>
            </div>
          </div>
          <p>把任务、执行过程、交付文件和最近会话放回一个稳定、清爽、可追踪的工作台里。</p>
        </div>

        <div className="dashboard-header-stats">
          {stats.map(({ Icon, label, tone, value }) => (
            <article className={`dashboard-stat-card dashboard-stat-card-${tone}`} key={label}>
              <span>
                <Icon size={18} />
                {label}
              </span>
              <strong>{value}</strong>
            </article>
          ))}

          <article className="dashboard-stat-card dashboard-stat-card-refresh">
            <span>
              <RefreshCw size={18} />
              刷新
            </span>
            <strong>{loading ? '同步中' : '已同步'}</strong>
            <small>最近更新: {formatTime(lastRefreshedAt.toISOString())}</small>
          </article>
        </div>
      </header>

      {error ? <div className="banner-error">接口当前不可用：{error}</div> : null}

      <section className="dashboard-reference-grid">
        <div className="dashboard-left-column">
          <TaskListPane
            tasks={data.tasks}
            selectedTaskID={selectedTask?.id ?? null}
            onSelect={(taskID) => {
              setSelectedTaskId(taskID)
            }}
          />
        </div>

        <div className="dashboard-center-column">
          <TaskDetailPane
            artifacts={selectedArtifacts}
            claudeRecord={claudeRecord}
            events={selectedEvents}
            task={selectedTask}
          />
        </div>

        <div className="dashboard-right-column">
          <TaskEventsCard events={selectedEvents} />
          <TaskArtifactsCard artifacts={selectedArtifacts} />
          <ClaudeRecordCard claudeRecord={claudeRecord} />
          <RecentConversationCard
            lastActive={activeConversation?.last_active ?? ''}
            messages={activeConversation?.messages ?? []}
          />
        </div>
      </section>
    </main>
  )
}
