package settings

import (
	"testing"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/testmysql"
)

func TestNewStoreLoadsDefaultSessionWindowSeconds(t *testing.T) {
	t.Parallel()

	store, err := NewStore(testmysql.NewDSN(t))
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

	dsn := testmysql.NewDSN(t)
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

	store, err := NewStore(testmysql.NewDSN(t))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	for _, value := range []int{0, 29, 3601} {
		if _, err := store.UpdateSessionWindowSeconds(value); err == nil {
			t.Fatalf("UpdateSessionWindowSeconds(%d) error = nil, want error", value)
		}
	}
}

func TestStoreUpdateIMDelivery(t *testing.T) {
	t.Parallel()

	dsn := testmysql.NewDSN(t)
	store, err := NewStore(dsn)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	snapshot, err := store.UpdateIMDelivery(true, "account_1", "target_1")
	if err != nil {
		t.Fatalf("UpdateIMDelivery() error = %v", err)
	}
	if !snapshot.IMDeliveryEnabled {
		t.Fatal("IMDeliveryEnabled = false, want true")
	}
	if snapshot.IMSelectedAccountID != "account_1" {
		t.Fatalf("IMSelectedAccountID = %q, want %q", snapshot.IMSelectedAccountID, "account_1")
	}
	if snapshot.IMSelectedTargetID != "target_1" {
		t.Fatalf("IMSelectedTargetID = %q, want %q", snapshot.IMSelectedTargetID, "target_1")
	}

	reloaded, err := NewStore(dsn)
	if err != nil {
		t.Fatalf("reload NewStore() error = %v", err)
	}
	if !reloaded.Snapshot().IMDeliveryEnabled {
		t.Fatal("reloaded IMDeliveryEnabled = false, want true")
	}
	if reloaded.Snapshot().IMSelectedAccountID != "account_1" {
		t.Fatalf("reloaded IMSelectedAccountID = %q, want %q", reloaded.Snapshot().IMSelectedAccountID, "account_1")
	}
	if reloaded.Snapshot().IMSelectedTargetID != "target_1" {
		t.Fatalf("reloaded IMSelectedTargetID = %q, want %q", reloaded.Snapshot().IMSelectedTargetID, "target_1")
	}
}

func TestStoreRejectsInvalidIMDeliveryWhenEnabled(t *testing.T) {
	t.Parallel()

	store, err := NewStore(testmysql.NewDSN(t))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	if _, err := store.UpdateIMDelivery(true, "", "target_1"); err == nil {
		t.Fatal("UpdateIMDelivery() error = nil, want error for missing account id")
	}
	if _, err := store.UpdateIMDelivery(true, "account_1", ""); err == nil {
		t.Fatal("UpdateIMDelivery() error = nil, want error for missing target id")
	}
}
