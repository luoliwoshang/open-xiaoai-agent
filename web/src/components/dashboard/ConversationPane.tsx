import { MessageCircleHeart } from 'lucide-react'
import { formatTime } from '../../lib/dashboard'
import type { ConversationSnapshot } from '../../types'
import { EmptyState } from '../ui/EmptyState'
import { SectionCard } from '../ui/SectionCard'

type Props = {
  conversation: ConversationSnapshot | null
}

export function ConversationPane({ conversation }: Props) {
  return (
    <SectionCard
      className="conversation-card"
      description="这里保留最近一次正在延续的对话片段，方便确认小爱刚刚记住了什么。"
      eyebrow="CHAT MEMORY"
      title="最近会话"
    >
      {!conversation ? (
        <EmptyState title="还没有活跃会话" description="等你和小爱多说几句，这里就会开始积累最近上下文。" />
      ) : (
        <div className="conversation-stack">
          <div className="conversation-summary">
            <span>
              <MessageCircleHeart size={14} />
              {conversation.messages.length} 条消息
            </span>
            <span>{formatTime(conversation.last_active)}</span>
          </div>

          <div className="conversation-bubbles">
            {conversation.messages.map((message, index) => (
              <article
                className={`chat-bubble ${message.role === 'user' ? 'chat-bubble-user' : 'chat-bubble-assistant'}`}
                key={`${conversation.id}-${index}`}
              >
                <strong>{message.role === 'user' ? '你' : '小爱'}</strong>
                <p>{message.content}</p>
              </article>
            ))}
          </div>
        </div>
      )}
    </SectionCard>
  )
}
