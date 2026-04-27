import { formatTime } from '../../lib/dashboard'
import type { IMEvent } from '../../types'

type Props = {
  events: IMEvent[]
}

export function IMEventsPanel({ events }: Props) {
  return (
    <article className="panel settings-panel panel-wide">
      <div className="panel-head compact">
        <div>
          <p className="eyebrow">RECENT EVENTS</p>
          <h3>网关事件</h3>
        </div>
      </div>

      <div className="timeline">
        {events.length === 0 ? (
          <div className="empty-card">暂时还没有网关事件。</div>
        ) : (
          events.map((event) => (
            <article className="timeline-item" key={event.id}>
              <span className="timeline-dot" />
              <div>
                <div className="timeline-head">
                  <strong>{event.type}</strong>
                  <span>{formatTime(event.created_at)}</span>
                </div>
                <p>{event.message}</p>
              </div>
            </article>
          ))
        )}
      </div>
    </article>
  )
}

