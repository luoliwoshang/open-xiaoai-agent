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
