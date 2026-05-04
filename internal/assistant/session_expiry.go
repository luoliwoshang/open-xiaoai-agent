package assistant

import (
	"context"
	"log"
	"strings"
	"time"
)

const (
	sessionExpirySweepInterval = 2 * time.Second
	sessionSummaryTimeout      = 90 * time.Second
)

// runSessionExpiryLoop 持续扫描“已经自然结束的会话”。
//
// assistant 主流程里的每次稳定问答仍然先写入短期会话窗口；
// 当某段 session 因为超时而关闭后，这个后台循环会把那段完整 history
// 交给长期记忆实现做一次低频总结。
func (s *Service) runSessionExpiryLoop(ctx context.Context) {
	if s == nil || s.history == nil {
		return
	}

	ticker := time.NewTicker(sessionExpirySweepInterval)
	defer ticker.Stop()

	var pending []ConversationSnapshot
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			expired := s.history.PopExpiredSessions(now)
			if len(expired) > 0 {
				pending = append(pending, expired...)
			}
			if len(pending) == 0 {
				continue
			}
			pending = s.processExpiredSessions(ctx, pending)
		}
	}
}

// processExpiredSessions 顺序处理一批刚结束的会话。
//
// 失败的会话不会直接丢弃，而是放回 pending，等待下一轮扫描时重试。
func (s *Service) processExpiredSessions(parent context.Context, snapshots []ConversationSnapshot) []ConversationSnapshot {
	if len(snapshots) == 0 {
		return nil
	}

	nextPending := make([]ConversationSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if err := s.updateMemoryFromExpiredSession(parent, snapshot); err != nil {
			log.Printf(
				"session memory update failed: key=%s messages=%d started_at=%s last_active=%s err=%v",
				strings.TrimSpace(snapshot.ID),
				len(snapshot.Messages),
				snapshot.StartedAt.Format(time.RFC3339),
				snapshot.LastActive.Format(time.RFC3339),
				err,
			)
			nextPending = append(nextPending, snapshot)
		}
	}
	return nextPending
}

// updateMemoryFromExpiredSession 把一段已经结束的完整 session history
// 交给长期记忆实现做一次总结更新。
//
// 注意这里传给 memory 的不是全量历史，而只是这次刚结束会话自己的消息列表。
func (s *Service) updateMemoryFromExpiredSession(parent context.Context, snapshot ConversationSnapshot) error {
	if s == nil || s.memory == nil {
		return nil
	}

	historyKey := strings.TrimSpace(snapshot.ID)
	if historyKey == "" || len(snapshot.Messages) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(parent, sessionSummaryTimeout)
	defer cancel()

	log.Printf(
		"session expired: key=%s messages=%d started_at=%s last_active=%s",
		historyKey,
		len(snapshot.Messages),
		snapshot.StartedAt.Format(time.RFC3339),
		snapshot.LastActive.Format(time.RFC3339),
	)
	return s.memory.UpdateFromSession(ctx, historyKey, snapshot.Messages)
}
