import { Bot, Download, FileHeart, FolderCog, ScrollText } from 'lucide-react'
import { formatBytes, formatTime } from '../../lib/dashboard'
import type { ClaudeRecord, Task, TaskArtifact, TaskEvent } from '../../types'
import { EmptyState } from '../ui/EmptyState'
import { PillTabs } from '../ui/PillTabs'
import { SectionCard } from '../ui/SectionCard'
import { StatusBadge } from '../ui/StatusBadge'

export type TaskDetailTab = 'overview' | 'artifacts' | 'events' | 'claude'

type Props = {
  task: Task | null
  artifacts: TaskArtifact[]
  events: TaskEvent[]
  claudeRecord: ClaudeRecord | null
  tab: TaskDetailTab
  onTabChange: (tab: TaskDetailTab) => void
}

const tabs: Array<{ key: TaskDetailTab; label: string; caption: string }> = [
  { key: 'overview', label: '概览', caption: '摘要与结果' },
  { key: 'artifacts', label: '产物', caption: '文件与下载' },
  { key: 'events', label: '轨迹', caption: '过程记录' },
  { key: 'claude', label: '执行器', caption: 'Claude 上下文' },
]

export function TaskDetailPane({ task, artifacts, events, claudeRecord, tab, onTabChange }: Props) {
  if (!task) {
    return (
      <SectionCard
        className="dashboard-main-card"
        description="选中一条任务之后，可以在这里继续看它的过程、结果和交付文件。"
        eyebrow="TASK ROOM"
        title="任务工作台"
      >
        <EmptyState title="先选一条任务" description="左边点开任意一条任务，这里就会展开更完整的细节。" />
      </SectionCard>
    )
  }

  return (
    <SectionCard
      actions={<StatusBadge state={task.state} />}
      className="dashboard-main-card"
      description={task.input || '这条任务没有额外输入描述。'}
      eyebrow="TASK ROOM"
      title={task.title}
    >
      <PillTabs tabs={tabs} value={tab} onChange={onTabChange} />

      {tab === 'overview' ? (
        <div className="detail-grid">
          <article className="soft-panel">
            <div className="soft-panel-head">
              <ScrollText size={18} />
              <strong>阶段摘要</strong>
            </div>
            <p>{task.summary || '当前还没有阶段摘要。'}</p>
          </article>

          <article className="soft-panel">
            <div className="soft-panel-head">
              <Bot size={18} />
              <strong>最终结果</strong>
            </div>
            <p>{task.result || '任务还没有给出最终结果。'}</p>
          </article>

          <article className="soft-panel">
            <div className="soft-panel-head">
              <FolderCog size={18} />
              <strong>基础信息</strong>
            </div>
            <dl className="fact-list">
              <div>
                <dt>任务 ID</dt>
                <dd>{task.id}</dd>
              </div>
              <div>
                <dt>类型</dt>
                <dd>{task.kind}</dd>
              </div>
              <div>
                <dt>创建时间</dt>
                <dd>{formatTime(task.created_at)}</dd>
              </div>
              <div>
                <dt>最近更新</dt>
                <dd>{formatTime(task.updated_at)}</dd>
              </div>
            </dl>
          </article>
        </div>
      ) : null}

      {tab === 'artifacts' ? (
        artifacts.length > 0 ? (
          <div className="artifact-list">
            {artifacts.map((artifact) => (
              <article className="artifact-card" key={artifact.id}>
                <div className="artifact-card-head">
                  <div className="artifact-card-copy">
                    <strong>{artifact.file_name}</strong>
                    <p>{artifact.mime_type || 'application/octet-stream'}</p>
                  </div>
                  {artifact.deliver ? <span className="soft-chip soft-chip-peach">已标记交付</span> : null}
                </div>
                <div className="artifact-card-meta">
                  <span>
                    <FileHeart size={14} />
                    {artifact.kind}
                  </span>
                  <span>{formatBytes(artifact.size_bytes)}</span>
                  <span>{formatTime(artifact.created_at)}</span>
                </div>
                <a
                  className="action-link"
                  href={`/api/tasks/${encodeURIComponent(artifact.task_id)}/artifacts/${encodeURIComponent(artifact.id)}/download`}
                >
                  <Download size={16} />
                  下载这个产物
                </a>
              </article>
            ))}
          </div>
        ) : (
          <EmptyState title="还没有交付文件" description="如果这条任务产出了网页、文档或别的文件，它们会在这里出现。" />
        )
      ) : null}

      {tab === 'events' ? (
        events.length > 0 ? (
          <div className="timeline-stack">
            {events.map((event) => (
              <article className="timeline-bubble" key={event.id}>
                <div className="timeline-bubble-head">
                  <strong>{event.type}</strong>
                  <span>{formatTime(event.created_at)}</span>
                </div>
                <p>{event.message}</p>
              </article>
            ))}
          </div>
        ) : (
          <EmptyState title="过程记录还很安静" description="这条任务目前还没有额外事件，稍后会在这里看到更多过程信息。" />
        )
      ) : null}

      {tab === 'claude' ? (
        claudeRecord ? (
          <div className="detail-grid">
            <article className="soft-panel">
              <div className="soft-panel-head">
                <Bot size={18} />
                <strong>会话与状态</strong>
              </div>
              <dl className="fact-list">
                <div>
                  <dt>Claude 会话</dt>
                  <dd>{claudeRecord.session_id || '还未建立'}</dd>
                </div>
                <div>
                  <dt>执行状态</dt>
                  <dd>{claudeRecord.status}</dd>
                </div>
                <div>
                  <dt>工作目录</dt>
                  <dd>{claudeRecord.working_directory || '—'}</dd>
                </div>
              </dl>
            </article>

            <article className="soft-panel">
              <div className="soft-panel-head">
                <ScrollText size={18} />
                <strong>最近进度</strong>
              </div>
              <p>{claudeRecord.last_summary || '还没有可展示的执行摘要。'}</p>
              {claudeRecord.error ? <p className="inline-error">{claudeRecord.error}</p> : null}
            </article>
          </div>
        ) : (
          <EmptyState title="这条任务没有 Claude 私有记录" description="如果它是通过 Claude Code 执行的，这里会展示会话和工作目录信息。" />
        )
      ) : null}
    </SectionCard>
  )
}
