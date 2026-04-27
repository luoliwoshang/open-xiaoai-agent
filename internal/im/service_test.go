package im

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/settings"
)

type stubAdapter struct {
	mu       sync.Mutex
	sendErr  error
	messages []string
}

func (s *stubAdapter) Platform() string {
	return "stub"
}

func (s *stubAdapter) StartLogin(ctx context.Context) (WeChatLoginStart, error) {
	return WeChatLoginStart{}, nil
}

func (s *stubAdapter) PollLogin(ctx context.Context, sessionKey string) (WeChatLoginResult, error) {
	return WeChatLoginResult{}, nil
}

func (s *stubAdapter) SendText(ctx context.Context, account Account, target Target, text string) (TextSendResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, text)
	if s.sendErr != nil {
		return TextSendResult{}, s.sendErr
	}
	return TextSendResult{MessageID: "msg_1"}, nil
}

func (s *stubAdapter) sentCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages)
}

func TestServiceUpdateDeliveryConfigPersistsSelection(t *testing.T) {
	t.Parallel()

	dsn := "sqlite://" + t.TempDir() + "/agent.db"
	settingsStore, err := settings.NewStore(dsn)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	service, err := NewService(dsn, settingsStore)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	account, err := service.store.UpsertAccount("stub", "bot@im.bot", "user@im.wechat", "Bot", "https://example.com", "token")
	if err != nil {
		t.Fatalf("UpsertAccount() error = %v", err)
	}
	target, err := service.store.UpsertTarget(account.ID, "我的微信", "user@im.wechat", true)
	if err != nil {
		t.Fatalf("UpsertTarget() error = %v", err)
	}

	snapshot, err := service.UpdateDeliveryConfig(true, account.ID, target.ID)
	if err != nil {
		t.Fatalf("UpdateDeliveryConfig() error = %v", err)
	}
	if !snapshot.IMDeliveryEnabled {
		t.Fatal("IMDeliveryEnabled = false, want true")
	}
	if snapshot.IMSelectedAccountID != account.ID {
		t.Fatalf("IMSelectedAccountID = %q, want %q", snapshot.IMSelectedAccountID, account.ID)
	}
	if snapshot.IMSelectedTargetID != target.ID {
		t.Fatalf("IMSelectedTargetID = %q, want %q", snapshot.IMSelectedTargetID, target.ID)
	}
}

func TestServiceMirrorTextUpdatesDeliverySuccess(t *testing.T) {
	t.Parallel()

	dsn := "sqlite://" + t.TempDir() + "/agent.db"
	settingsStore, err := settings.NewStore(dsn)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	service, err := NewService(dsn, settingsStore)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	adapter := &stubAdapter{}
	service.adapters["stub"] = adapter

	account, err := service.store.UpsertAccount("stub", "bot@im.bot", "user@im.wechat", "Bot", "https://example.com", "token")
	if err != nil {
		t.Fatalf("UpsertAccount() error = %v", err)
	}
	target, err := service.store.UpsertTarget(account.ID, "我的微信", "user@im.wechat", true)
	if err != nil {
		t.Fatalf("UpsertTarget() error = %v", err)
	}
	if _, err := service.UpdateDeliveryConfig(true, account.ID, target.ID); err != nil {
		t.Fatalf("UpdateDeliveryConfig() error = %v", err)
	}

	service.MirrorText("你好，小爱")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		refreshed, ok, err := service.store.GetAccount(account.ID)
		if err != nil {
			t.Fatalf("GetAccount() error = %v", err)
		}
		if ok && !refreshed.LastSentAt.IsZero() && adapter.sentCount() == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("mirror delivery did not complete, sentCount=%d", adapter.sentCount())
}

func TestServiceMirrorTextUpdatesDeliveryFailure(t *testing.T) {
	t.Parallel()

	dsn := "sqlite://" + t.TempDir() + "/agent.db"
	settingsStore, err := settings.NewStore(dsn)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	service, err := NewService(dsn, settingsStore)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	adapter := &stubAdapter{sendErr: errors.New("boom")}
	service.adapters["stub"] = adapter

	account, err := service.store.UpsertAccount("stub", "bot@im.bot", "user@im.wechat", "Bot", "https://example.com", "token")
	if err != nil {
		t.Fatalf("UpsertAccount() error = %v", err)
	}
	target, err := service.store.UpsertTarget(account.ID, "我的微信", "user@im.wechat", true)
	if err != nil {
		t.Fatalf("UpsertTarget() error = %v", err)
	}
	if _, err := service.UpdateDeliveryConfig(true, account.ID, target.ID); err != nil {
		t.Fatalf("UpdateDeliveryConfig() error = %v", err)
	}

	service.MirrorText("你好，小爱")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		refreshed, ok, err := service.store.GetAccount(account.ID)
		if err != nil {
			t.Fatalf("GetAccount() error = %v", err)
		}
		if ok && refreshed.LastError != "" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("mirror delivery failure did not update last_error")
}

func TestServiceDeleteAccountDisablesDelivery(t *testing.T) {
	t.Parallel()

	dsn := "sqlite://" + t.TempDir() + "/agent.db"
	settingsStore, err := settings.NewStore(dsn)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	service, err := NewService(dsn, settingsStore)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	account, err := service.store.UpsertAccount("stub", "bot@im.bot", "user@im.wechat", "Bot", "https://example.com", "token")
	if err != nil {
		t.Fatalf("UpsertAccount() error = %v", err)
	}
	target, err := service.store.UpsertTarget(account.ID, "我的微信", "user@im.wechat", true)
	if err != nil {
		t.Fatalf("UpsertTarget() error = %v", err)
	}
	if _, err := service.UpdateDeliveryConfig(true, account.ID, target.ID); err != nil {
		t.Fatalf("UpdateDeliveryConfig() error = %v", err)
	}

	if err := service.DeleteAccount(account.ID); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	snapshot := settingsStore.Snapshot()
	if snapshot.IMDeliveryEnabled {
		t.Fatal("IMDeliveryEnabled = true, want false")
	}
	if snapshot.IMSelectedAccountID != "" || snapshot.IMSelectedTargetID != "" {
		t.Fatalf("selection should be cleared, got account=%q target=%q", snapshot.IMSelectedAccountID, snapshot.IMSelectedTargetID)
	}
}
