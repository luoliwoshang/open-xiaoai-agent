package im

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/settings"
)

type runtimeSettings interface {
	Snapshot() settings.Snapshot
	UpdateIMDelivery(enabled bool, accountID string, targetID string) (settings.Snapshot, error)
}

type Service struct {
	store      *Store
	settings   runtimeSettings
	mediaCache *MediaCache
	adapters   map[string]Adapter

	pendingMu     sync.Mutex
	pendingLogins map[string]pendingWeChatLogin
	deliveries    chan deliveryRequest
}

type deliveryRequest struct {
	AccountID string
	TargetID  string
	Text      string
}

type pendingWeChatLogin struct {
	Result      WeChatLoginResult
	ConfirmedAt time.Time
}

func NewService(dsn string, runtimeSettings runtimeSettings, mediaCacheDir string) (*Service, error) {
	store, err := NewStore(dsn)
	if err != nil {
		return nil, err
	}
	mediaCache, err := NewMediaCache(mediaCacheDir)
	if err != nil {
		return nil, err
	}

	svc := &Service{
		store:      store,
		settings:   runtimeSettings,
		mediaCache: mediaCache,
		adapters: map[string]Adapter{
			PlatformWeChat: NewWeChatAdapter(),
		},
		pendingLogins: make(map[string]pendingWeChatLogin),
		deliveries:    make(chan deliveryRequest, 32),
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
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return WeChatLoginStatus{}, fmt.Errorf("wechat login session key is required")
	}
	if pending, ok := s.getPendingWeChatLogin(sessionKey); ok {
		return WeChatLoginStatus{
			Status:    pending.Status,
			Message:   pending.Message,
			Candidate: buildWeChatLoginCandidate(pending),
		}, nil
	}

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

	s.rememberPendingWeChatLogin(sessionKey, result)
	status.Candidate = buildWeChatLoginCandidate(result)
	return status, nil
}

func (s *Service) ConfirmWeChatLogin(sessionKey string) (Account, error) {
	if s == nil || s.store == nil {
		return Account{}, fmt.Errorf("im service is not configured")
	}

	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return Account{}, fmt.Errorf("wechat login session key is required")
	}

	result, ok := s.getPendingWeChatLogin(sessionKey)
	if !ok {
		return Account{}, fmt.Errorf("当前扫码结果不存在或已失效，请重新发起登录")
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
		return Account{}, err
	}
	if _, err := s.store.EnsureOwnerTarget(account.ID, result.OwnerUserID); err != nil {
		return Account{}, err
	}
	if err := s.store.AppendEvent(account.ID, "login", "微信账号登录成功"); err != nil {
		log.Printf("append im login event failed: %v", err)
	}

	s.deletePendingWeChatLogin(sessionKey)
	return account, nil
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

func (s *Service) SendTextToDefaultChannel(text string) (DeliveryReceipt, error) {
	if s == nil || s.store == nil || s.settings == nil {
		return DeliveryReceipt{}, fmt.Errorf("im service is not configured")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return DeliveryReceipt{}, fmt.Errorf("debug text is required")
	}

	cfg := s.settings.Snapshot()
	if strings.TrimSpace(cfg.IMSelectedAccountID) == "" || strings.TrimSpace(cfg.IMSelectedTargetID) == "" {
		return DeliveryReceipt{}, fmt.Errorf("默认渠道尚未配置，请先保存账号和触达目标")
	}

	return s.deliverText(deliveryRequest{
		AccountID: cfg.IMSelectedAccountID,
		TargetID:  cfg.IMSelectedTargetID,
		Text:      text,
	})
}

func (s *Service) SendImageToDefaultChannel(req ImageSendRequest) (DeliveryReceipt, error) {
	if s == nil || s.store == nil || s.settings == nil || s.mediaCache == nil {
		return DeliveryReceipt{}, fmt.Errorf("im service is not configured")
	}

	cfg := s.settings.Snapshot()
	if strings.TrimSpace(cfg.IMSelectedAccountID) == "" || strings.TrimSpace(cfg.IMSelectedTargetID) == "" {
		return DeliveryReceipt{}, fmt.Errorf("默认渠道尚未配置，请先保存账号和触达目标")
	}

	prepared, err := s.mediaCache.StoreImage(req)
	if err != nil {
		return DeliveryReceipt{}, err
	}
	return s.deliverImage(cfg.IMSelectedAccountID, cfg.IMSelectedTargetID, prepared, req.Caption)
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
	s.pendingMu.Lock()
	s.pendingLogins = make(map[string]pendingWeChatLogin)
	s.pendingMu.Unlock()
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
		s.appendEvent(request.AccountID, "send_drop", "IM 镜像发送队列繁忙，本次消息已丢弃")
	}
}

func (s *Service) runDeliveryLoop() {
	for request := range s.deliveries {
		if _, err := s.deliverText(request); err != nil {
			log.Printf("im mirror delivery failed: account=%s target=%s err=%v", request.AccountID, request.TargetID, err)
		}
	}
}

func (s *Service) deliverText(request deliveryRequest) (DeliveryReceipt, error) {
	account, target, adapter, err := s.resolveDelivery(request.AccountID, request.TargetID)
	if err != nil {
		return DeliveryReceipt{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	result, err := adapter.SendText(ctx, account, target, request.Text)
	if err != nil {
		_ = s.store.MarkDeliveryFailure(account.ID, err.Error())
		s.appendEvent(account.ID, "send_failed", fmt.Sprintf("发送到 %s 失败：%s", target.Name, err.Error()))
		return DeliveryReceipt{}, err
	}

	_ = s.store.MarkDeliverySuccess(account.ID)
	s.appendEvent(account.ID, "send", fmt.Sprintf("已发送到 %s：%s", target.Name, trimForEvent(request.Text)))
	return DeliveryReceipt{
		Account:   account,
		Target:    target,
		MessageID: result.MessageID,
		Kind:      DeliveryKindText,
		Text:      request.Text,
	}, nil
}

func (s *Service) deliverImage(accountID string, targetID string, image PreparedImage, caption string) (DeliveryReceipt, error) {
	account, target, adapter, err := s.resolveDelivery(accountID, targetID)
	if err != nil {
		return DeliveryReceipt{}, err
	}

	log.Printf("im image delivery started: account=%s target=%s platform=%s file=%q mime=%s size=%d caption_len=%d", account.ID, target.ID, account.Platform, image.FileName, image.MimeType, image.Size, len(strings.TrimSpace(caption)))

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	result, err := adapter.SendImage(ctx, account, target, image, caption)
	if err != nil {
		_ = s.store.MarkDeliveryFailure(account.ID, err.Error())
		s.appendEvent(account.ID, "send_failed", fmt.Sprintf("发送图片到 %s 失败：%s", target.Name, err.Error()))
		log.Printf("im image delivery failed: account=%s target=%s platform=%s file=%q err=%v", account.ID, target.ID, account.Platform, image.FileName, err)
		return DeliveryReceipt{}, err
	}

	_ = s.store.MarkDeliverySuccess(account.ID)
	eventLabel := strings.TrimSpace(caption)
	if eventLabel == "" {
		eventLabel = image.FileName
	}
	s.appendEvent(account.ID, "send_image", fmt.Sprintf("已发送图片到 %s：%s", target.Name, trimForEvent(eventLabel)))
	log.Printf("im image delivery succeeded: account=%s target=%s platform=%s file=%q message_id=%s", account.ID, target.ID, account.Platform, image.FileName, result.MessageID)
	return DeliveryReceipt{
		Account:       account,
		Target:        target,
		MessageID:     result.MessageID,
		Kind:          DeliveryKindImage,
		Caption:       strings.TrimSpace(caption),
		MediaFileName: image.FileName,
		MediaMimeType: image.MimeType,
	}, nil
}

func (s *Service) appendEvent(accountID string, eventType string, message string) {
	if s == nil || s.store == nil {
		return
	}
	if err := s.store.AppendEvent(accountID, eventType, message); err != nil {
		log.Printf("append im event failed: account=%s type=%s err=%v", accountID, eventType, err)
	}
}

func (s *Service) resolveDelivery(accountID string, targetID string) (Account, Target, Adapter, error) {
	account, ok, err := s.store.GetAccount(accountID)
	if err != nil {
		return Account{}, Target{}, nil, err
	}
	if !ok {
		return Account{}, Target{}, nil, fmt.Errorf("im account %q not found", accountID)
	}

	target, ok, err := s.store.GetTarget(targetID)
	if err != nil {
		return Account{}, Target{}, nil, err
	}
	if !ok {
		return Account{}, Target{}, nil, fmt.Errorf("im target %q not found", targetID)
	}
	if target.AccountID != account.ID {
		return Account{}, Target{}, nil, fmt.Errorf("im target %q does not belong to account %q", target.ID, account.ID)
	}

	adapter, ok := s.adapters[account.Platform]
	if !ok {
		return Account{}, Target{}, nil, fmt.Errorf("im adapter %q is not configured", account.Platform)
	}
	return account, target, adapter, nil
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

func (s *Service) getPendingWeChatLogin(sessionKey string) (WeChatLoginResult, bool) {
	if s == nil {
		return WeChatLoginResult{}, false
	}

	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()

	pending, ok := s.pendingLogins[sessionKey]
	if !ok {
		return WeChatLoginResult{}, false
	}
	if time.Since(pending.ConfirmedAt) > 30*time.Minute {
		delete(s.pendingLogins, sessionKey)
		return WeChatLoginResult{}, false
	}
	return pending.Result, true
}

func (s *Service) rememberPendingWeChatLogin(sessionKey string, result WeChatLoginResult) {
	if s == nil {
		return
	}

	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()

	s.pendingLogins[sessionKey] = pendingWeChatLogin{
		Result:      result,
		ConfirmedAt: time.Now(),
	}
}

func (s *Service) deletePendingWeChatLogin(sessionKey string) {
	if s == nil {
		return
	}

	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()

	delete(s.pendingLogins, sessionKey)
}

func buildWeChatLoginCandidate(result WeChatLoginResult) *WeChatLoginCandidate {
	if strings.TrimSpace(result.RemoteAccountID) == "" {
		return nil
	}
	return &WeChatLoginCandidate{
		RemoteAccountID: strings.TrimSpace(result.RemoteAccountID),
		OwnerUserID:     strings.TrimSpace(result.OwnerUserID),
		DisplayName:     strings.TrimSpace(result.RemoteAccountID),
		BaseURL:         strings.TrimSpace(result.BaseURL),
	}
}
