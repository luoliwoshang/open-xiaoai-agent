import { MessageCircle } from 'lucide-react'
import type { ConversationSnapshot } from '../../types'
import { formatTime } from '../../lib/dashboard'
import { EmptyState } from '../ui/EmptyState'

interface RecentConversationPaneProps {
  conversations: ConversationSnapshot[]
}

export function RecentConversationPane({ conversations }: RecentConversationPaneProps) {
  const activeConversation = conversations[0]

  return (
    <div className="section-card" style={{ maxHeight: 'calc(100vh - 200px)' }}>
      <div className="section-card-header">
        <div className="section-card-title">
          <MessageCircle />
          近期对话
        </div>
        {activeConversation && (
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-ghost)' }}>
            {formatTime(activeConversation.last_active)}
          </span>
        )}
      </div>
      <div className="section-card-body">
        {!activeConversation || activeConversation.messages.length === 0 ? (
          <EmptyState title="暂无对话" description="当主语音流程接到输入后，这里会展示最近上下文。" />
        ) : (
          <div className="conversation-bubbles">
            {activeConversation.messages.slice(-4).map((msg, index) => (
              <div key={`${msg.role}-${index}`} className={`bubble ${msg.role}`}>
                <div className="bubble-role">{msg.role === 'user' ? '用户' : '助理'}</div>
                {msg.content}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
