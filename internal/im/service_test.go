package im

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/settings"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/testmysql"
)

type stubAdapter struct {
	mu          sync.Mutex
	sendErr     error
	loginErr    error
	loginResult WeChatLoginResult
	messages    []string
}

func (s *stubAdapter) Platform() string {
	return "stub"
}

func (s *stubAdapter) StartLogin(ctx context.Context) (WeChatLoginStart, error) {
	return WeChatLoginStart{}, nil
}

func (s *stubAdapter) PollLogin(ctx context.Context, sessionKey string) (WeChatLoginResult, error) {
	if s.loginErr != nil {
		return WeChatLoginResult{}, s.loginErr
	}
	return s.loginResult, nil
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

	dsn := testmysql.NewDSN(t)
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

func TestServiceConfirmWeChatLoginPersistsAccountAfterExplicitConfirmation(t *testing.T) {
	t.Parallel()

	dsn := testmysql.NewDSN(t)
	settingsStore, err := settings.NewStore(dsn)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	service, err := NewService(dsn, settingsStore)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	adapter := &stubAdapter{
		loginResult: WeChatLoginResult{
			Status:          "confirmed",
			Message:         "微信账号登录成功。",
			RemoteAccountID: "bot@im.bot",
			OwnerUserID:     "user@im.wechat",
			BaseURL:         "https://example.com",
			Token:           "token",
		},
	}
	service.adapters[PlatformWeChat] = adapter

	status, err := service.PollWeChatLogin("session-1")
	if err != nil {
		t.Fatalf("PollWeChatLogin() error = %v", err)
	}
	if status.Status != "confirmed" {
		t.Fatalf("status = %q, want confirmed", status.Status)
	}
	if status.Candidate == nil {
		t.Fatal("Candidate = nil, want candidate data")
	}

	accounts, err := service.store.ListAccounts()
	if err != nil {
		t.Fatalf("ListAccounts() error = %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("ListAccounts() len = %d, want 0 before confirmation", len(accounts))
	}

	account, err := service.ConfirmWeChatLogin("session-1")
	if err != nil {
		t.Fatalf("ConfirmWeChatLogin() error = %v", err)
	}
	if account.RemoteAccountID != "bot@im.bot" {
		t.Fatalf("RemoteAccountID = %q, want bot@im.bot", account.RemoteAccountID)
	}

	accounts, err = service.store.ListAccounts()
	if err != nil {
		t.Fatalf("ListAccounts() error = %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("ListAccounts() len = %d, want 1 after confirmation", len(accounts))
	}

	targets, err := service.store.ListTargets()
	if err != nil {
		t.Fatalf("ListTargets() error = %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("ListTargets() len = %d, want 1 auto-created owner target", len(targets))
	}
}

func TestServiceSendTextToDefaultChannelUsesSavedSelectionEvenWhenMirrorDisabled(t *testing.T) {
	t.Parallel()

	dsn := testmysql.NewDSN(t)
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
	if _, err := service.UpdateDeliveryConfig(false, account.ID, target.ID); err != nil {
		t.Fatalf("UpdateDeliveryConfig() error = %v", err)
	}

	receipt, err := service.SendTextToDefaultChannel("调试消息")
	if err != nil {
		t.Fatalf("SendTextToDefaultChannel() error = %v", err)
	}
	if receipt.Account.ID != account.ID {
		t.Fatalf("receipt.Account.ID = %q, want %q", receipt.Account.ID, account.ID)
	}
	if receipt.Target.ID != target.ID {
		t.Fatalf("receipt.Target.ID = %q, want %q", receipt.Target.ID, target.ID)
	}
	if receipt.Text != "调试消息" {
		t.Fatalf("receipt.Text = %q, want 调试消息", receipt.Text)
	}
	if adapter.sentCount() != 1 {
		t.Fatalf("sentCount() = %d, want 1", adapter.sentCount())
	}
}

func TestServiceMirrorTextUpdatesDeliverySuccess(t *testing.T) {
	t.Parallel()

	dsn := testmysql.NewDSN(t)
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

	dsn := testmysql.NewDSN(t)
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

	dsn := testmysql.NewDSN(t)
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

func TestServiceDeleteNonSelectedTargetKeepsDeliverySelection(t *testing.T) {
	t.Parallel()

	dsn := testmysql.NewDSN(t)
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
	selectedTarget, err := service.store.UpsertTarget(account.ID, "主目标", "user-a@im.wechat", true)
	if err != nil {
		t.Fatalf("UpsertTarget() error = %v", err)
	}
	otherTarget, err := service.store.UpsertTarget(account.ID, "其他目标", "user-b@im.wechat", false)
	if err != nil {
		t.Fatalf("UpsertTarget() error = %v", err)
	}
	if _, err := service.UpdateDeliveryConfig(true, account.ID, selectedTarget.ID); err != nil {
		t.Fatalf("UpdateDeliveryConfig() error = %v", err)
	}

	if err := service.DeleteTarget(otherTarget.ID); err != nil {
		t.Fatalf("DeleteTarget() error = %v", err)
	}

	snapshot := settingsStore.Snapshot()
	if !snapshot.IMDeliveryEnabled {
		t.Fatal("IMDeliveryEnabled = false, want true")
	}
	if snapshot.IMSelectedAccountID != account.ID {
		t.Fatalf("IMSelectedAccountID = %q, want %q", snapshot.IMSelectedAccountID, account.ID)
	}
	if snapshot.IMSelectedTargetID != selectedTarget.ID {
		t.Fatalf("IMSelectedTargetID = %q, want %q", snapshot.IMSelectedTargetID, selectedTarget.ID)
	}
}

func TestServiceDeleteSelectedTargetDisablesDelivery(t *testing.T) {
	t.Parallel()

	dsn := testmysql.NewDSN(t)
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
	target, err := service.store.UpsertTarget(account.ID, "主目标", "user-a@im.wechat", true)
	if err != nil {
		t.Fatalf("UpsertTarget() error = %v", err)
	}
	if _, err := service.UpdateDeliveryConfig(true, account.ID, target.ID); err != nil {
		t.Fatalf("UpdateDeliveryConfig() error = %v", err)
	}

	if err := service.DeleteTarget(target.ID); err != nil {
		t.Fatalf("DeleteTarget() error = %v", err)
	}

	snapshot := settingsStore.Snapshot()
	if snapshot.IMDeliveryEnabled {
		t.Fatal("IMDeliveryEnabled = true, want false")
	}
	if snapshot.IMSelectedAccountID != "" || snapshot.IMSelectedTargetID != "" {
		t.Fatalf("selection should be cleared, got account=%q target=%q", snapshot.IMSelectedAccountID, snapshot.IMSelectedTargetID)
	}
}
