package settings

import (
	"testing"
	"time"
)

func TestNewStoreLoadsDefaultSessionWindowSeconds(t *testing.T) {
	t.Parallel()

	store, err := NewStore("sqlite://" + t.TempDir() + "/agent.db")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	snapshot := store.Snapshot()
	if snapshot.SessionWindowSeconds != 300 {
		t.Fatalf("SessionWindowSeconds = %d, want 300", snapshot.SessionWindowSeconds)
	}
	if store.SessionWindow() != 300*time.Second {
		t.Fatalf("SessionWindow() = %s, want 300s", store.SessionWindow())
	}
}

func TestStoreUpdateSessionWindowSeconds(t *testing.T) {
	t.Parallel()

	dsn := "sqlite://" + t.TempDir() + "/agent.db"
	store, err := NewStore(dsn)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	snapshot, err := store.UpdateSessionWindowSeconds(420)
	if err != nil {
		t.Fatalf("UpdateSessionWindowSeconds() error = %v", err)
	}
	if snapshot.SessionWindowSeconds != 420 {
		t.Fatalf("SessionWindowSeconds = %d, want 420", snapshot.SessionWindowSeconds)
	}

	reloaded, err := NewStore(dsn)
	if err != nil {
		t.Fatalf("reload NewStore() error = %v", err)
	}
	if reloaded.Snapshot().SessionWindowSeconds != 420 {
		t.Fatalf("reloaded SessionWindowSeconds = %d, want 420", reloaded.Snapshot().SessionWindowSeconds)
	}
}

func TestStoreRejectsInvalidSessionWindowSeconds(t *testing.T) {
	t.Parallel()

	store, err := NewStore("sqlite://" + t.TempDir() + "/agent.db")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	for _, value := range []int{0, 29, 3601} {
		if _, err := store.UpdateSessionWindowSeconds(value); err == nil {
			t.Fatalf("UpdateSessionWindowSeconds(%d) error = nil, want error", value)
		}
	}
}
