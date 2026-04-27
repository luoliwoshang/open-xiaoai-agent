package im

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/settings"
)

type runtimeSettings interface {
	Snapshot() settings.Snapshot
	UpdateIMDelivery(enabled bool, accountID string, targetID string) (settings.Snapshot, error)
}

type Service struct {
	store    *Store
	settings runtimeSettings
	adapters map[string]Adapter

	deliveries chan deliveryRequest
}

type deliveryRequest struct {
	AccountID string
	TargetID  string
	Text      string
}

func NewService(dsn string, runtimeSettings runtimeSettings) (*Service, error) {
	store, err := NewStore(dsn)
	if err != nil {
		return nil, err
	}

	svc := &Service{
		store:    store,
		settings: runtimeSettings,
		adapters: map[string]Adapter{
			PlatformWeChat: NewWeChatAdapter(),
		},
		deliveries: make(chan deliveryRequest, 32),
	}
	go svc.runDeliveryLoop()
	return svc, nil
}

func (s *Service) Snapshot() Snapshot {
	if s == nil || s.store == nil {
		return Snapshot{}
	}
	snapshot, err := s.store.Snapshot(30)
	if err != nil {
		log.Printf("im snapshot failed: %v", err)
		return Snapshot{}
	}
	return snapshot
}

func (s *Service) StartWeChatLogin() (WeChatLoginStart, error) {
	adapter, ok := s.adapters[PlatformWeChat]
	if !ok {
		return WeChatLoginStart{}, fmt.Errorf("wechat adapter is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return adapter.StartLogin(ctx)
}

func (s *Service) PollWeChatLogin(sessionKey string) (WeChatLoginStatus, error) {
	adapter, ok := s.adapters[PlatformWeChat]
	if !ok {
		return WeChatLoginStatus{}, fmt.Errorf("wechat adapter is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	result, err := adapter.PollLogin(ctx, sessionKey)
	if err != nil {
		return WeChatLoginStatus{}, err
	}

	status := WeChatLoginStatus{
		Status:  result.Status,
		Message: result.Message,
	}
	if result.Status != "confirmed" {
		return status, nil
	}

	account, err := s.store.UpsertAccount(
		PlatformWeChat,
		result.RemoteAccountID,
		result.OwnerUserID,
		result.RemoteAccountID,
		result.BaseURL,
		result.Token,
	)
	if err != nil {
		return WeChatLoginStatus{}, err
	}
	if _, err := s.store.EnsureOwnerTarget(account.ID, result.OwnerUserID); err != nil {
		return WeChatLoginStatus{}, err
	}
	if err := s.store.AppendEvent(account.ID, "login", "微信账号登录成功"); err != nil {
		log.Printf("append im login event failed: %v", err)
	}

	status.Account = &account
	return status, nil
}

func (s *Service) UpsertTarget(accountID string, name string, targetUserID string, setDefault bool) (Target, error) {
	if s == nil || s.store == nil {
		return Target{}, fmt.Errorf("im service is not configured")
	}
	if _, ok, err := s.store.GetAccount(accountID); err != nil {
		return Target{}, err
	} else if !ok {
		return Target{}, fmt.Errorf("im account %q does not exist", strings.TrimSpace(accountID))
	}

	target, err := s.store.UpsertTarget(accountID, name, targetUserID, setDefault)
	if err != nil {
		return Target{}, err
	}
	if err := s.store.AppendEvent(accountID, "target", fmt.Sprintf("更新触达目标：%s", target.Name)); err != nil {
		log.Printf("append im target event failed: %v", err)
	}
	return target, nil
}

func (s *Service) SetDefaultTarget(accountID string, targetID string) error {
	if s == nil || s.store == nil {
		return nil
	}

	target, ok, err := s.store.GetTarget(targetID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("im target %q does not exist", strings.TrimSpace(targetID))
	}
	if target.AccountID != strings.TrimSpace(accountID) {
		return fmt.Errorf("im target %q does not belong to account %q", targetID, accountID)
	}
	if err := s.store.SetDefaultTarget(accountID, targetID); err != nil {
		return err
	}
	return nil
}

func (s *Service) DeleteTarget(targetID string) error {
	if s == nil || s.store == nil {
		return nil
	}

	target, ok, err := s.store.GetTarget(targetID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	if err := s.store.DeleteTarget(targetID); err != nil {
		return err
	}
	return s.repairDeliveryConfigAfterMutation(target.AccountID, target.ID)
}

func (s *Service) DeleteAccount(accountID string) error {
	if s == nil || s.store == nil {
		return nil
	}

	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return fmt.Errorf("im account id is required")
	}

	if err := s.store.DeleteAccount(accountID); err != nil {
		return err
	}
	return s.repairDeliveryConfigAfterMutation(accountID, "")
}

func (s *Service) UpdateDeliveryConfig(enabled bool, accountID string, targetID string) (settings.Snapshot, error) {
	if s == nil || s.settings == nil {
		return settings.Snapshot{}, fmt.Errorf("runtime settings are not configured")
	}

	accountID = strings.TrimSpace(accountID)
	targetID = strings.TrimSpace(targetID)
	if enabled {
		account, ok, err := s.store.GetAccount(accountID)
		if err != nil {
			return settings.Snapshot{}, err
		}
		if !ok {
			return settings.Snapshot{}, fmt.Errorf("im account %q does not exist", accountID)
		}
		target, ok, err := s.store.GetTarget(targetID)
		if err != nil {
			return settings.Snapshot{}, err
		}
		if !ok {
			return settings.Snapshot{}, fmt.Errorf("im target %q does not exist", targetID)
		}
		if target.AccountID != account.ID {
			return settings.Snapshot{}, fmt.Errorf("im target %q does not belong to account %q", targetID, accountID)
		}
	}

	return s.settings.UpdateIMDelivery(enabled, accountID, targetID)
}

func (s *Service) Reset() error {
	if s == nil || s.store == nil {
		return nil
	}
	if err := s.store.Reset(); err != nil {
		return err
	}
	if s.settings != nil {
		if _, err := s.settings.UpdateIMDelivery(false, "", ""); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) MirrorText(text string) {
	if s == nil || s.settings == nil || s.store == nil {
		return
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	cfg := s.settings.Snapshot()
	if !cfg.IMDeliveryEnabled {
		return
	}
	request := deliveryRequest{
		AccountID: cfg.IMSelectedAccountID,
		TargetID:  cfg.IMSelectedTargetID,
		Text:      text,
	}

	select {
	case s.deliveries <- request:
	default:
		log.Printf("im mirror queue is full, dropping message")
		_ = s.store.MarkDeliveryFailure(request.AccountID, "IM 镜像发送队列繁忙，本次消息已丢弃")
		_ = s.store.AppendEvent(request.AccountID, "send_drop", "IM 镜像发送队列繁忙，本次消息已丢弃")
	}
}

func (s *Service) runDeliveryLoop() {
	for request := range s.deliveries {
		if err := s.deliverText(request); err != nil {
			log.Printf("im mirror delivery failed: account=%s target=%s err=%v", request.AccountID, request.TargetID, err)
		}
	}
}

func (s *Service) deliverText(request deliveryRequest) error {
	account, ok, err := s.store.GetAccount(request.AccountID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("im account %q not found", request.AccountID)
	}

	target, ok, err := s.store.GetTarget(request.TargetID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("im target %q not found", request.TargetID)
	}
	if target.AccountID != account.ID {
		return fmt.Errorf("im target %q does not belong to account %q", target.ID, account.ID)
	}

	adapter, ok := s.adapters[account.Platform]
	if !ok {
		return fmt.Errorf("im adapter %q is not configured", account.Platform)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if _, err := adapter.SendText(ctx, account, target, request.Text); err != nil {
		_ = s.store.MarkDeliveryFailure(account.ID, err.Error())
		_ = s.store.AppendEvent(account.ID, "send_failed", fmt.Sprintf("发送到 %s 失败：%s", target.Name, err.Error()))
		return err
	}

	_ = s.store.MarkDeliverySuccess(account.ID)
	_ = s.store.AppendEvent(account.ID, "send", fmt.Sprintf("已发送到 %s：%s", target.Name, trimForEvent(request.Text)))
	return nil
}

func (s *Service) repairDeliveryConfigAfterMutation(accountID string, targetID string) error {
	if s == nil || s.settings == nil {
		return nil
	}

	accountID = strings.TrimSpace(accountID)
	targetID = strings.TrimSpace(targetID)
	current := s.settings.Snapshot()
	removedSelectedAccount := targetID == "" && current.IMSelectedAccountID == accountID
	removedSelectedTarget := targetID != "" && current.IMSelectedTargetID == targetID
	if !removedSelectedAccount && !removedSelectedTarget {
		return nil
	}

	_, err := s.settings.UpdateIMDelivery(false, "", "")
	return err
}

func trimForEvent(text string) string {
	text = strings.TrimSpace(text)
	const limit = 80
	if len([]rune(text)) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit]) + "..."
}
