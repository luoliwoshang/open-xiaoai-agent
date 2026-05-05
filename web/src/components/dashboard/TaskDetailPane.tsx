import { Bot, CalendarClock, FileStack, FolderTree, Sparkles, TimerReset } from 'lucide-react'
import { formatTime } from '../../lib/dashboard'
import type { ClaudeRecord, Task, TaskArtifact, TaskEvent } from '../../types'
import { EmptyState } from '../ui/EmptyState'
import { SectionCard } from '../ui/SectionCard'
import { StatusBadge } from '../ui/StatusBadge'

type Props = {
  task: Task | null
  events: TaskEvent[]
  artifacts: TaskArtifact[]
  claudeRecord: ClaudeRecord | null
}

type ProgressModel = {
  percent: number
  step: number
  steps: Array<{ label: string; note: string; done: boolean; active: boolean }>
}

function buildProgressModel(task: Task, events: TaskEvent[], artifacts: TaskArtifact[]): ProgressModel {
  const hasArtifacts = artifacts.length > 0
  const eventCount = events.length

  if (task.state === 'completed') {
    return {
      percent: 100,
      step: 5,
      steps: [
        { label: '已接收', note: formatTime(task.created_at), done: true, active: false },
        { label: '开始执行', note: eventCount > 0 ? formatTime(events[0]?.created_at ?? task.created_at) : '已开始', done: true, active: false },
        { label: '处理中', note: task.summary ? '阶段摘要已生成' : '处理中', done: true, active: false },
        { label: '生成产物', note: hasArtifacts ? `${artifacts.length} 个产物` : '无需产物', done: true, active: false },
        { label: '完成', note: formatTime(task.updated_at), done: true, active: true },
      ],
    }
  }

  if (task.state === 'superseded') {
    return {
      percent: hasArtifacts ? 82 : eventCount >= 2 ? 64 : 40,
      step: hasArtifacts ? 4 : eventCount >= 2 ? 3 : 2,
      steps: [
        { label: '已接收', note: formatTime(task.created_at), done: true, active: false },
        { label: '开始执行', note: eventCount > 0 ? formatTime(events[0]?.created_at ?? task.created_at) : '已开始', done: true, active: false },
        { label: '处理中', note: '旧执行已被新的补充要求接管', done: true, active: false },
        { label: '生成产物', note: hasArtifacts ? `${artifacts.length} 个产物` : '交给新的续做任务继续产出', done: hasArtifacts, active: !hasArtifacts },
        { label: '完成', note: '本任务不再继续推进', done: false, active: false },
      ],
    }
  }

  if (task.state === 'failed' || task.state === 'canceled') {
    return {
      percent: hasArtifacts ? 76 : eventCount >= 2 ? 54 : 28,
      step: hasArtifacts ? 4 : eventCount >= 2 ? 3 : 2,
      steps: [
        { label: '已接收', note: formatTime(task.created_at), done: true, active: false },
        { label: '开始执行', note: eventCount > 0 ? formatTime(events[0]?.created_at ?? task.created_at) : '已开始', done: true, active: false },
        { label: '处理中', note: task.state === 'failed' ? '任务异常结束' : '任务被取消', done: eventCount >= 2, active: eventCount < 2 },
        { label: '生成产物', note: hasArtifacts ? `${artifacts.length} 个产物` : '未生成', done: hasArtifacts, active: eventCount >= 2 && !hasArtifacts },
        { label: '完成', note: '未达成', done: false, active: false },
      ],
    }
  }

  if (task.state === 'running') {
    const step = hasArtifacts ? 4 : eventCount >= 2 ? 3 : 2
    const percent = hasArtifacts ? 78 : eventCount >= 2 ? 62 : 36
    return {
      percent,
      step,
      steps: [
        { label: '已接收', note: formatTime(task.created_at), done: true, active: false },
        { label: '开始执行', note: eventCount > 0 ? formatTime(events[0]?.created_at ?? task.created_at) : '已开始', done: true, active: false },
        { label: '处理中', note: task.summary || '正在推进当前任务', done: step > 3, active: step === 3 },
        { label: '生成产物', note: hasArtifacts ? `${artifacts.length} 个产物已写入` : '待产物输出', done: false, active: step === 4 },
        { label: '完成', note: '等待结束', done: false, active: false },
      ],
    }
  }

  return {
    percent: 18,
    step: 1,
    steps: [
      { label: '已接收', note: formatTime(task.created_at), done: true, active: true },
      { label: '开始执行', note: '等待调度', done: false, active: false },
      { label: '处理中', note: '等待推进', done: false, active: false },
      { label: '生成产物', note: '待开始', done: false, active: false },
      { label: '完成', note: '待开始', done: false, active: false },
    ],
  }
}

export function TaskDetailPane({ task, events, artifacts, claudeRecord }: Props) {
  if (!task) {
    return (
      <SectionCard
        className="dashboard-main-card"
        description="选中一条任务之后，这里会展示它的状态、摘要、执行节奏和当前上下文。"
        eyebrow="TASK DETAIL"
        title="任务详情"
      >
        <EmptyState title="先选一条任务" description="左侧点开任意任务，这里就会展开完整细节。" />
      </SectionCard>
    )
  }

  const progress = buildProgressModel(task, events, artifacts)

  return (
    <SectionCard
      actions={<StatusBadge state={task.state} />}
      className="dashboard-main-card"
      description={task.input || '这条任务没有额外输入描述。'}
      eyebrow="TASK DETAIL"
      title={task.title}
    >
      <div className="task-detail-hero">
        <div className="task-detail-meta-grid">
          <div className="task-detail-meta">
            <span>任务 ID</span>
            <p>{task.id}</p>
          </div>
          <div className="task-detail-meta">
            <span>父任务 ID</span>
            <p>{task.parent_task_id || '—'}</p>
          </div>
          <div className="task-detail-meta">
            <span>Claude 会话</span>
            <p>{claudeRecord?.session_id || '尚未建立'}</p>
          </div>
          <div className="task-detail-meta">
            <span>任务类型</span>
            <p>{task.kind}</p>
          </div>
          <div className="task-detail-meta">
            <span>创建时间</span>
            <p>{formatTime(task.created_at)}</p>
          </div>
          <div className="task-detail-meta">
            <span>最近更新</span>
            <p>{formatTime(task.updated_at)}</p>
          </div>
        </div>

        <div className="task-detail-mascot">
          <div className="task-detail-mascot-ring" />
          <div className="task-detail-mascot-core">
            <Bot size={34} />
          </div>
          <span className="task-detail-star task-detail-star-a">
            <Sparkles size={14} />
          </span>
          <span className="task-detail-star task-detail-star-b">
            <Sparkles size={12} />
          </span>
        </div>
      </div>

      <article className="task-progress-card">
        <div className="task-progress-head">
          <div>
            <strong>进度概览</strong>
            <p>根据当前任务状态、事件和产物数量推导出当前推进阶段。</p>
          </div>
          <div className="task-progress-value">
            <span>总体进度</span>
            <strong>{progress.percent}%</strong>
          </div>
        </div>

        <div className="task-progress-bar">
          <span style={{ width: `${progress.percent}%` }} />
        </div>

        <div className="task-progress-steps">
          {progress.steps.map((step, index) => (
            <div className="task-progress-step" key={step.label}>
              <div className={`task-progress-node ${step.done ? 'task-progress-node-done' : ''} ${step.active ? 'task-progress-node-active' : ''}`}>
                {index + 1}
              </div>
              <div className="task-progress-copy">
                <strong>{step.label}</strong>
                <span>{step.note}</span>
              </div>
            </div>
          ))}
        </div>
      </article>

      <div className="task-detail-bottom-grid">
        <article className="task-detail-panel">
          <div className="task-detail-panel-head">
            <FolderTree size={16} />
            <strong>任务说明</strong>
          </div>
          <p>{task.input || '这条任务没有额外输入描述。'}</p>
        </article>

        <article className="task-detail-panel">
          <div className="task-detail-panel-head">
            <FileStack size={16} />
            <strong>当前摘要</strong>
          </div>
          <p>{task.summary || '当前还没有阶段摘要。'}</p>
        </article>

        <article className="task-detail-panel task-detail-panel-wide">
          <div className="task-detail-panel-head">
            <CalendarClock size={16} />
            <strong>最近结果</strong>
          </div>
          <p>{task.result || task.summary || '任务还没有给出最终结果。'}</p>
        </article>

        <article className="task-detail-panel task-detail-panel-wide task-detail-panel-muted">
          <div className="task-detail-panel-head">
            <TimerReset size={16} />
            <strong>当前状态说明</strong>
          </div>
          <p>
            {task.state === 'completed'
              ? '这条任务已经结束，后续如果继续补充要求，会新建一条 continuation task，并保留当前任务记录。'
              : task.state === 'running'
                ? '任务正在后台推进中。你可以继续正常聊天，也可以直接追问这条任务的当前进展。'
                : task.state === 'accepted'
                  ? '任务已经被系统接收，正在等待调度执行。'
                  : task.state === 'failed'
                    ? '任务已失败，建议查看右侧事件流和 Claude 记录定位问题。'
                    : '任务已取消，当前不会继续推进。'}
          </p>
        </article>
      </div>
    </SectionCard>
  )
}
