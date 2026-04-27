package assistant

import (
	"testing"
	"time"
)

type historyTestSession struct {
	id string
}

func (s historyTestSession) HistoryKey() string {
	return s.id
}

func TestHistoryStoreResetsAfterWindow(t *testing.T) {
	t.Parallel()

	store, err := newHistoryStore(5*time.Minute, "")
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

func TestHistoryStoreSnapshotAllKeepsActiveAndSorts(t *testing.T) {
	t.Parallel()

	store, err := newHistoryStore(5*time.Minute, "")
	if err != nil {
		t.Fatalf("newHistoryStore() error = %v", err)
	}
	sessionA := historyTestSession{id: "session-a"}
	sessionB := historyTestSession{id: "session-b"}
	start := time.Date(2026, 4, 24, 16, 0, 0, 0, time.Local)

	store.AppendTurn(sessionA, start, "A1", "A2")
	store.AppendTurn(sessionB, start.Add(2*time.Minute), "B1", "B2")

	snapshots := store.SnapshotAll(start.Add(3 * time.Minute))
	if len(snapshots) != 2 {
		t.Fatalf("len(snapshots) = %d, want 2", len(snapshots))
	}
	if snapshots[0].Messages[0].Content != "B1" {
		t.Fatalf("snapshots[0].Messages[0].Content = %q, want B1", snapshots[0].Messages[0].Content)
	}

	snapshots = store.SnapshotAll(start.Add(7 * time.Minute))
	if len(snapshots) != 1 {
		t.Fatalf("len(snapshots) = %d, want 1 after expiration", len(snapshots))
	}
	if snapshots[0].Messages[0].Content != "B1" {
		t.Fatalf("snapshots[0].Messages[0].Content = %q, want B1", snapshots[0].Messages[0].Content)
	}
}

func TestHistoryStorePersistsConversation(t *testing.T) {
	t.Parallel()

	path := "sqlite://" + t.TempDir() + "/agent.db"
	start := time.Now()
	session := historyTestSession{id: "session-persist"}

	store, err := newHistoryStore(5*time.Minute, path)
	if err != nil {
		t.Fatalf("newHistoryStore() error = %v", err)
	}
	store.AppendTurn(session, start, "你好", "你好呀")

	reloaded, err := newHistoryStore(5*time.Minute, path)
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

	path := "sqlite://" + t.TempDir() + "/agent.db"
	start := time.Now()
	session := historyTestSession{id: "session-reset"}

	store, err := newHistoryStore(5*time.Minute, path)
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

	reloaded, err := newHistoryStore(5*time.Minute, path)
	if err != nil {
		t.Fatalf("reload newHistoryStore() error = %v", err)
	}
	if snapshots := reloaded.SnapshotAll(start.Add(time.Minute)); len(snapshots) != 0 {
		t.Fatalf("len(reloaded snapshots) = %d, want 0", len(snapshots))
	}
}
