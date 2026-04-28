import { BellDot, FolderKanban, MessageCircleMore, Send } from 'lucide-react'
import { useEffect, useMemo, useState, type Dispatch, type SetStateAction } from 'react'
import { countByState, latest } from '../lib/dashboard'
import type { ClaudeRecord, ConversationSnapshot, DashboardState, TaskEvent } from '../types'
import { ConversationPane } from '../components/dashboard/ConversationPane'
import { TaskDetailPane, type TaskDetailTab } from '../components/dashboard/TaskDetailPane'
import { TaskListPane } from '../components/dashboard/TaskListPane'

type Props = {
  data: DashboardState
  loading: boolean
  error: string | null
  setData: Dispatch<SetStateAction<DashboardState>>
}

export function DashboardPage({ data, loading, error, setData: _setData }: Props) {
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null)
  const [detailTab, setDetailTab] = useState<TaskDetailTab>('overview')

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

  const overviewCards = useMemo(() => {
    return [
      {
        label: '排队中的事情',
        value: data.tasks.length,
        note: '今天小爱接到的任务',
        Icon: FolderKanban,
        tone: 'peach',
      },
      {
        label: '正在处理',
        value: countByState(data.tasks, 'running'),
        note: '后台持续推进',
        Icon: Send,
        tone: 'mint',
      },
      {
        label: '待补报',
        value: data.tasks.filter((task) => task.report_pending).length,
        note: '下次聊天会顺带告诉你',
        Icon: BellDot,
        tone: 'sky',
      },
      {
        label: '最近会话',
        value: activeConversation?.messages.length ?? 0,
        note: '上下文还在记着',
        Icon: MessageCircleMore,
        tone: 'butter',
      },
    ]
  }, [activeConversation?.messages.length, data.tasks])

  return (
    <main className="page-shell dashboard-shell">
      <header className="page-hero">
        <div className="page-hero-copy">
          <p className="section-eyebrow">TODAY WITH XIAOAI</p>
          <h2>{selectedTask ? `这会儿正围着「${selectedTask.title}」忙活` : '先和小爱说一句话，让今天开始转起来'}</h2>
          <p>
            这里不再混入系统设置和技术说明。你只会看到现在最有用的三类信息：任务进展、交付文件、最近对话。
          </p>
        </div>

        <div className="hero-metrics">
          {overviewCards.map(({ Icon, label, note, tone, value }) => (
            <article className={`hero-metric hero-metric-${tone}`} key={label}>
              <span>
                <Icon size={16} />
                {label}
              </span>
              <strong>{value}</strong>
              <small>{note}</small>
            </article>
          ))}
        </div>
      </header>

      {error ? <div className="banner-error">接口暂时有点卡住了：{error}</div> : null}
      {loading ? <div className="banner-hint">正在更新最新状态…</div> : null}

      <section className="dashboard-layout">
        <div className="dashboard-rail">
          <TaskListPane tasks={data.tasks} selectedTaskID={selectedTask?.id ?? null} onSelect={(taskID) => {
            setSelectedTaskId(taskID)
            setDetailTab('overview')
          }} />
        </div>

        <div className="dashboard-stage">
          <TaskDetailPane
            artifacts={selectedArtifacts}
            claudeRecord={claudeRecord}
            events={selectedEvents}
            onTabChange={setDetailTab}
            tab={detailTab}
            task={selectedTask}
          />

          <ConversationPane conversation={activeConversation} />
        </div>
      </section>
    </main>
  )
}
