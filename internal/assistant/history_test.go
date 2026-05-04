package assistant

import (
	"testing"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/testmysql"
)

type staticSessionWindow struct {
	window time.Duration
}

func (s staticSessionWindow) SessionWindow() time.Duration {
	return s.window
}

type historyTestSession struct {
	id string
}

func (s historyTestSession) HistoryKey() string {
	return s.id
}

func TestHistoryStoreResetsAfterWindow(t *testing.T) {
	t.Parallel()

	store, err := newHistoryStore(staticSessionWindow{window: 5 * time.Minute}, "")
	if err != nil {
		t.Fatalf("newHistoryStore() error = %v", err)
	}
	session := historyTestSession{id: "session-a"}
	start := time.Date(2026, 4, 23, 18, 30, 0, 0, time.Local)

	store.AppendTurn(session, start, "第一句", "第一答")
	history := store.Snapshot(session, start.Add(4*time.Minute))
	if len(history) != 2 {
		t.Fatalf("len(history) = %d, want 2", len(history))
	}

	history = store.Snapshot(session, start.Add(6*time.Minute))
	if len(history) != 0 {
		t.Fatalf("len(history) = %d, want 0 after reset", len(history))
	}
}

func TestHistoryStoreSlidingWindowUsesLastActive(t *testing.T) {
	t.Parallel()

	store, err := newHistoryStore(staticSessionWindow{window: 5 * time.Minute}, "")
	if err != nil {
		t.Fatalf("newHistoryStore() error = %v", err)
	}
	session := historyTestSession{id: "session-a"}
	start := time.Date(2026, 4, 24, 16, 0, 0, 0, time.Local)

	store.AppendTurn(session, start, "A1", "A2")
	store.AppendTurn(session, start.Add(4*time.Minute), "A3", "A4")

	history := store.Snapshot(session, start.Add(8*time.Minute))
	if len(history) != 4 {
		t.Fatalf("len(history) = %d, want 4", len(history))
	}
	history = store.Snapshot(session, start.Add(10*time.Minute))
	if len(history) != 0 {
		t.Fatalf("len(history) = %d, want 0 after sliding expiration", len(history))
	}
}

func TestHistoryStorePopExpiredSessionsReturnsWholeConversation(t *testing.T) {
	t.Parallel()

	store, err := newHistoryStore(staticSessionWindow{window: 5 * time.Minute}, "")
	if err != nil {
		t.Fatalf("newHistoryStore() error = %v", err)
	}
	session := historyTestSession{id: "session-expired"}
	start := time.Date(2026, 5, 4, 12, 0, 0, 0, time.Local)

	store.AppendTurn(session, start, "第一句", "第一答")
	store.AppendTurn(session, start.Add(2*time.Minute), "第二句", "第二答")

	expired := store.PopExpiredSessions(start.Add(8 * time.Minute))
	if len(expired) != 1 {
		t.Fatalf("len(expired) = %d, want 1", len(expired))
	}
	if expired[0].ID != "session-expired" {
		t.Fatalf("expired[0].ID = %q, want session-expired", expired[0].ID)
	}
	if len(expired[0].Messages) != 4 {
		t.Fatalf("len(expired[0].Messages) = %d, want 4", len(expired[0].Messages))
	}
	if expired[0].Messages[0].Content != "第一句" || expired[0].Messages[3].Content != "第二答" {
		t.Fatalf("expired messages = %#v", expired[0].Messages)
	}
}

func TestHistoryStoreRotatesExpiredConversationOnNextTurnAndQueuesClosedSnapshot(t *testing.T) {
	t.Parallel()

	store, err := newHistoryStore(staticSessionWindow{window: 5 * time.Minute}, "")
	if err != nil {
		t.Fatalf("newHistoryStore() error = %v", err)
	}
	session := historyTestSession{id: "session-rotate"}
	start := time.Date(2026, 5, 4, 13, 0, 0, 0, time.Local)

	store.AppendTurn(session, start, "旧问题", "旧回答")
	store.AppendTurn(session, start.Add(7*time.Minute), "新问题", "新回答")

	closed := store.PopExpiredSessions(start.Add(7 * time.Minute))
	if len(closed) != 1 {
		t.Fatalf("len(closed) = %d, want 1", len(closed))
	}
	if len(closed[0].Messages) != 2 || closed[0].Messages[0].Content != "旧问题" {
		t.Fatalf("closed[0].Messages = %#v", closed[0].Messages)
	}

	current := store.Snapshot(session, start.Add(7*time.Minute))
	if len(current) != 2 {
		t.Fatalf("len(current) = %d, want 2", len(current))
	}
	if current[0].Content != "新问题" || current[1].Content != "新回答" {
		t.Fatalf("current = %#v", current)
	}
}

func TestHistoryStorePersistsConversation(t *testing.T) {
	t.Parallel()

	path := testmysql.NewDSN(t)
	start := time.Now()
	session := historyTestSession{id: "session-persist"}

	store, err := newHistoryStore(staticSessionWindow{window: 5 * time.Minute}, path)
	if err != nil {
		t.Fatalf("newHistoryStore() error = %v", err)
	}
	store.AppendTurn(session, start, "你好", "你好呀")

	reloaded, err := newHistoryStore(staticSessionWindow{window: 5 * time.Minute}, path)
	if err != nil {
		t.Fatalf("reload newHistoryStore() error = %v", err)
	}

	history := reloaded.Snapshot(session, start.Add(time.Minute))
	if len(history) != 2 {
		t.Fatalf("len(history) = %d, want 2", len(history))
	}
	if history[0].Content != "你好" || history[1].Content != "你好呀" {
		t.Fatalf("history = %#v", history)
	}
}

func TestHistoryStoreResetClearsPersistence(t *testing.T) {
	t.Parallel()

	path := testmysql.NewDSN(t)
	start := time.Now()
	session := historyTestSession{id: "session-reset"}

	store, err := newHistoryStore(staticSessionWindow{window: 5 * time.Minute}, path)
	if err != nil {
		t.Fatalf("newHistoryStore() error = %v", err)
	}
	store.AppendTurn(session, start, "你好", "你好呀")

	if err := store.Reset(); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}

	if snapshots := store.SnapshotAll(start.Add(time.Minute)); len(snapshots) != 0 {
		t.Fatalf("len(snapshots) = %d, want 0", len(snapshots))
	}

	reloaded, err := newHistoryStore(staticSessionWindow{window: 5 * time.Minute}, path)
	if err != nil {
		t.Fatalf("reload newHistoryStore() error = %v", err)
	}
	if snapshots := reloaded.SnapshotAll(start.Add(time.Minute)); len(snapshots) != 0 {
		t.Fatalf("len(reloaded snapshots) = %d, want 0", len(snapshots))
	}
}
