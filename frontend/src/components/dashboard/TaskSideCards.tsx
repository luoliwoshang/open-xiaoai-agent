import { Clock, FileDown, Bot, MessageCircle, Download } from 'lucide-react'
import type { Task, TaskEvent, TaskArtifact, ClaudeRecord, ConversationSnapshot } from '../../types'
import { formatTime, formatBytes } from '../../lib/dashboard'
import { EmptyState } from '../ui/EmptyState'

interface TaskSideCardsProps {
  task: Task | null
  events: TaskEvent[]
  artifacts: TaskArtifact[]
  claudeRecords: ClaudeRecord[]
  conversations: ConversationSnapshot[]
}

export function TaskSideCards({ task, events, artifacts, claudeRecords, conversations }: TaskSideCardsProps) {
  const taskId = task?.id
  const taskEvents = taskId ? events.filter((e) => e.task_id === taskId).slice(0, 6) : []
  const taskArtifacts = taskId ? artifacts.filter((a) => a.task_id === taskId).slice(0, 4) : []
  const taskClaude = taskId ? claudeRecords.find((c) => c.task_id === taskId) : null
  const activeConversation = conversations[0]

  return (
    <div className="side-cards">
      {/* Events Timeline */}
      <div className="section-card">
        <div className="section-card-header">
          <div className="section-card-title">
            <Clock />
            事件时间线
          </div>
        </div>
        <div className="section-card-body">
          {taskEvents.length === 0 ? (
            <EmptyState title="暂无事件" />
          ) : (
            <div className="timeline">
              {taskEvents.map((evt) => (
                <div key={evt.id} className="timeline-item">
                  <div className="timeline-dot" />
                  <div className="timeline-content">
                    <div className="timeline-text">{evt.message}</div>
                    <div className="timeline-time">{formatTime(evt.created_at)}</div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Artifacts */}
      <div className="section-card">
        <div className="section-card-header">
          <div className="section-card-title">
            <FileDown />
            产物文件
          </div>
        </div>
        <div className="section-card-body">
          {taskArtifacts.length === 0 ? (
            <EmptyState title="暂无产物" />
          ) : (
            <div className="artifact-table">
              {taskArtifacts.map((art) => (
                <div key={art.id} className="artifact-row">
                  <div className="artifact-icon">
                    <FileDown />
                  </div>
                  <div className="artifact-info">
                    <div className="artifact-name">{art.file_name}</div>
                    <div className="artifact-meta">{art.kind} · {formatBytes(art.size_bytes)}</div>
                  </div>
                  <a
                    href={`/api/tasks/${art.task_id}/artifacts/${art.id}/download`}
                    className="artifact-download"
                    download
                  >
                    <Download />
                  </a>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Claude Record */}
      <div className="section-card">
        <div className="section-card-header">
          <div className="section-card-title">
            <Bot />
            Claude 执行器
          </div>
        </div>
        <div className="section-card-body">
          {!taskClaude ? (
            <EmptyState title="无执行记录" />
          ) : (
            <div className="claude-record">
              <div className="claude-row">
                <span className="claude-label">状态</span>
                <span className="claude-value">{taskClaude.status}</span>
              </div>
              <div className="claude-row">
                <span className="claude-label">会话</span>
                <span className="claude-value">{taskClaude.session_id?.slice(0, 12) || '—'}</span>
              </div>
              {taskClaude.last_summary && (
                <div className="claude-row">
                  <span className="claude-label">进度</span>
                  <span className="claude-value">{taskClaude.last_summary}</span>
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Recent Conversation */}
      <div className="section-card">
        <div className="section-card-header">
          <div className="section-card-title">
            <MessageCircle />
            近期对话
          </div>
        </div>
        <div className="section-card-body">
          {!activeConversation || activeConversation.messages.length === 0 ? (
            <EmptyState title="暂无对话" />
          ) : (
            <div className="conversation-bubbles">
              {activeConversation.messages.slice(-3).map((msg, i) => (
                <div key={i} className={`bubble ${msg.role}`}>
                  <div className="bubble-role">{msg.role === 'user' ? '用户' : '助理'}</div>
                  {msg.content.length > 120 ? msg.content.slice(0, 120) + '...' : msg.content}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
