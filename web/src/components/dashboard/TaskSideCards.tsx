import { Bot, Download, FileBox, MessageCircleMore, ScrollText } from 'lucide-react'
import { formatBytes, formatTime } from '../../lib/dashboard'
import type { ClaudeRecord, TaskArtifact, TaskEvent } from '../../types'
import { EmptyState } from '../ui/EmptyState'
import { SectionCard } from '../ui/SectionCard'
import { StatusBadge } from '../ui/StatusBadge'

export function TaskEventsCard({ events }: { events: TaskEvent[] }) {
  return (
    <SectionCard
      className="dashboard-side-card"
      actions={<span className="section-link">查看全部</span>}
      eyebrow="TIMELINE"
      title="任务事件流"
    >
      {events.length === 0 ? (
        <EmptyState title="还没有事件" description="等任务开始推进，这里会按时间顺序记录每一步动作。" />
      ) : (
        <div className="timeline-list">
          {events.slice(0, 6).map((event, index) => (
            <article className="timeline-row" key={event.id}>
              <div className={`timeline-dot timeline-dot-${index % 5}`} />
              <span className="timeline-time">{formatTime(event.created_at)}</span>
              <strong>{event.type}</strong>
              <p>{event.message}</p>
            </article>
          ))}
        </div>
      )}
    </SectionCard>
  )
}

export function TaskArtifactsCard({ artifacts }: { artifacts: TaskArtifact[] }) {
  return (
    <SectionCard
      className="dashboard-side-card"
      actions={<span className="section-link">查看全部</span>}
      eyebrow="FILES"
      title="Artifacts"
    >
      {artifacts.length === 0 ? (
        <EmptyState title="还没有产物" description="当任务输出文件、网页、图片或文档时，会出现在这里。" />
      ) : (
        <div className="artifact-table">
          <div className="artifact-table-head">
            <span>文件名</span>
            <span>类型</span>
            <span>大小</span>
            <span>操作</span>
          </div>
          {artifacts.slice(0, 4).map((artifact) => (
            <div className="artifact-table-row" key={artifact.id}>
              <div className="artifact-file-cell">
                <FileBox size={15} />
                <span>{artifact.file_name}</span>
              </div>
              <span>{artifact.kind}</span>
              <span>{formatBytes(artifact.size_bytes)}</span>
              <a
                className="artifact-download-button"
                href={`/api/tasks/${encodeURIComponent(artifact.task_id)}/artifacts/${encodeURIComponent(artifact.id)}/download`}
              >
                <Download size={14} />
              </a>
            </div>
          ))}
        </div>
      )}
    </SectionCard>
  )
}

export function ClaudeRecordCard({ claudeRecord }: { claudeRecord: ClaudeRecord | null }) {
  return (
    <SectionCard className="dashboard-side-card" eyebrow="EXECUTOR" title="Claude 记录">
      {!claudeRecord ? (
        <EmptyState title="没有 Claude 记录" description="如果当前任务走的是 Claude Code，这里会展示会话和最近状态。" />
      ) : (
        <div className="record-facts">
          <div className="record-facts-row">
            <span>执行器</span>
            <strong>Claude Code</strong>
          </div>
          <div className="record-facts-row">
            <span>会话状态</span>
            <StatusBadge state={claudeRecord.status} />
          </div>
          <div className="record-facts-row">
            <span>Claude 会话</span>
            <code>{claudeRecord.session_id || '未建立'}</code>
          </div>
          <div className="record-facts-row record-facts-row-top">
            <span>最近动作</span>
            <p>{claudeRecord.last_summary || claudeRecord.last_assistant_text || '暂无可展示的执行摘要。'}</p>
          </div>
          <div className="record-facts-row">
            <span>工作目录</span>
            <code>{claudeRecord.working_directory || '—'}</code>
          </div>
        </div>
      )}
    </SectionCard>
  )
}

export function RecentConversationCard({
  lastActive,
  messages,
}: {
  lastActive: string
  messages: Array<{ role: string; content: string }>
}) {
  const items = messages.slice(-3)

  return (
    <SectionCard
      className="dashboard-side-card"
      actions={<span className="section-link">查看全部</span>}
      eyebrow="MEMORY"
      title="最近会话"
    >
      {items.length === 0 ? (
        <EmptyState title="还没有会话" description="等你和小爱多说几句，这里就会开始保留最近上下文。" />
      ) : (
        <div className="recent-conversation-list">
          {items.map((message, index) => (
            <article className="recent-conversation-row" key={`${message.role}-${index}`}>
              <span className={`recent-conversation-avatar recent-conversation-avatar-${message.role === 'user' ? 'user' : 'assistant'}`}>
                {message.role === 'user' ? '你' : '助'}
              </span>
              <div className="recent-conversation-copy">
                <strong>{message.role === 'user' ? '用户' : '助手'}</strong>
                <p>{message.content}</p>
              </div>
              <span className="recent-conversation-time">{formatTime(lastActive)}</span>
            </article>
          ))}
        </div>
      )}
    </SectionCard>
  )
}

export function EmptyClaudeCard() {
  return (
    <SectionCard className="dashboard-side-card" eyebrow="EXECUTOR" title="Claude 记录">
      <div className="empty-inline-card">
        <Bot size={18} />
        <div>
          <strong>当前任务没有执行器记录</strong>
          <p>如果它是通过 Claude Code 跑起来的，这里会补上会话与最近动作。</p>
        </div>
      </div>
    </SectionCard>
  )
}

export function EmptyConversationCard() {
  return (
    <SectionCard className="dashboard-side-card" eyebrow="MEMORY" title="最近会话">
      <div className="empty-inline-card">
        <MessageCircleMore size={18} />
        <div>
          <strong>还没有会话片段</strong>
          <p>和小爱先说上几句，这里就会开始保留最近对话。</p>
        </div>
      </div>
    </SectionCard>
  )
}

export function EmptyTimelineCard() {
  return (
    <SectionCard className="dashboard-side-card" eyebrow="TIMELINE" title="任务事件流">
      <div className="empty-inline-card">
        <ScrollText size={18} />
        <div>
          <strong>还没有任务事件</strong>
          <p>一旦任务开始推进，这里就会按顺序展示受理、执行、产物写入等事件。</p>
        </div>
      </div>
    </SectionCard>
  )
}
