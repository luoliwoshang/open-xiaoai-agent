package dashboard

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/assistant"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/im"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins/complextask"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/settings"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/tasks"
)

type Server struct {
	addr          string
	tasks         *tasks.Manager
	claude        *complextask.Service
	conversations interface {
		SnapshotConversations() []assistant.ConversationSnapshot
		ResetConversationData() error
	}
	settings interface {
		Snapshot() settings.Snapshot
		UpdateSessionWindowSeconds(seconds int) (settings.Snapshot, error)
	}
	im interface {
		Snapshot() im.Snapshot
		StartWeChatLogin() (im.WeChatLoginStart, error)
		PollWeChatLogin(sessionKey string) (im.WeChatLoginStatus, error)
		ConfirmWeChatLogin(sessionKey string) (im.Account, error)
		SendTextToDefaultChannel(text string) (im.DeliveryReceipt, error)
		SendImageToDefaultChannel(req im.ImageSendRequest) (im.DeliveryReceipt, error)
		UpsertTarget(accountID string, name string, targetUserID string, setDefault bool) (im.Target, error)
		SetDefaultTarget(accountID string, targetID string) error
		DeleteTarget(targetID string) error
		DeleteAccount(accountID string) error
		UpdateDeliveryConfig(enabled bool, accountID string, targetID string) (settings.Snapshot, error)
		Reset() error
	}
}

func New(addr string, tasks *tasks.Manager, claude *complextask.Service, conversations interface {
	SnapshotConversations() []assistant.ConversationSnapshot
	ResetConversationData() error
}, runtimeSettings interface {
	Snapshot() settings.Snapshot
	UpdateSessionWindowSeconds(seconds int) (settings.Snapshot, error)
}, imGateway interface {
	Snapshot() im.Snapshot
	StartWeChatLogin() (im.WeChatLoginStart, error)
	PollWeChatLogin(sessionKey string) (im.WeChatLoginStatus, error)
	ConfirmWeChatLogin(sessionKey string) (im.Account, error)
	SendTextToDefaultChannel(text string) (im.DeliveryReceipt, error)
	SendImageToDefaultChannel(req im.ImageSendRequest) (im.DeliveryReceipt, error)
	UpsertTarget(accountID string, name string, targetUserID string, setDefault bool) (im.Target, error)
	SetDefaultTarget(accountID string, targetID string) error
	DeleteTarget(targetID string) error
	DeleteAccount(accountID string) error
	UpdateDeliveryConfig(enabled bool, accountID string, targetID string) (settings.Snapshot, error)
	Reset() error
}) *Server {
	return &Server{
		addr:          addr,
		tasks:         tasks,
		claude:        claude,
		conversations: conversations,
		settings:      runtimeSettings,
		im:            imGateway,
	}
}

func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/healthz", s.handleHealth)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/settings/session", s.handleSessionSettings)
	mux.HandleFunc("/api/settings/im-delivery", s.handleIMDeliverySettings)
	mux.HandleFunc("/api/im/wechat/login/start", s.handleWeChatLoginStart)
	mux.HandleFunc("/api/im/wechat/login/status", s.handleWeChatLoginStatus)
	mux.HandleFunc("/api/im/wechat/login/confirm", s.handleWeChatLoginConfirm)
	mux.HandleFunc("/api/im/debug/send-default", s.handleIMDebugSendDefault)
	mux.HandleFunc("/api/im/debug/send-image-default", s.handleIMDebugSendImageDefault)
	mux.HandleFunc("/api/im/targets", s.handleIMTargets)
	mux.HandleFunc("/api/im/targets/default", s.handleIMTargetDefault)
	mux.HandleFunc("/api/im/targets/delete", s.handleIMTargetDelete)
	mux.HandleFunc("/api/im/accounts/delete", s.handleIMAccountDelete)
	mux.HandleFunc("/api/reset", s.handleReset)

	log.Printf("dashboard api listening on %s", s.addr)
	return http.ListenAndServe(s.addr, mux)
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	tasksList, events := s.tasks.Snapshot()
	claudeRecords := []complextask.Record(nil)
	if s.claude != nil {
		claudeRecords = s.claude.Snapshot()
	}
	conversations := []assistant.ConversationSnapshot(nil)
	if s.conversations != nil {
		conversations = s.conversations.SnapshotConversations()
	}
	var runtimeSettings settings.Snapshot
	if s.settings != nil {
		runtimeSettings = s.settings.Snapshot()
	}
	var imState im.Snapshot
	if s.im != nil {
		imState = s.im.Snapshot()
	}
	writeJSON(w, map[string]any{
		"tasks":          tasksList,
		"events":         events,
		"claude_records": claudeRecords,
		"conversations":  conversations,
		"settings":       runtimeSettings,
		"im":             imState,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.settings == nil {
		http.Error(w, "settings are not configured", http.StatusServiceUnavailable)
		return
	}

	writeJSON(w, map[string]any{
		"settings": s.settings.Snapshot(),
	})
}

func (s *Server) handleSessionSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.settings == nil {
		http.Error(w, "settings are not configured", http.StatusServiceUnavailable)
		return
	}

	var payload struct {
		WindowSeconds int `json:"window_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, fmt.Sprintf("decode session settings: %v", err), http.StatusBadRequest)
		return
	}

	snapshot, err := s.settings.UpdateSessionWindowSeconds(payload.WindowSeconds)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, map[string]any{
		"ok":      true,
		"session": snapshot,
	})
}

func (s *Server) handleIMDeliverySettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.im == nil {
		http.Error(w, "im gateway is not configured", http.StatusServiceUnavailable)
		return
	}

	var payload struct {
		Enabled           bool   `json:"enabled"`
		SelectedAccountID string `json:"selected_account_id"`
		SelectedTargetID  string `json:"selected_target_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, fmt.Sprintf("decode im delivery settings: %v", err), http.StatusBadRequest)
		return
	}

	snapshot, err := s.im.UpdateDeliveryConfig(payload.Enabled, payload.SelectedAccountID, payload.SelectedTargetID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, map[string]any{
		"ok":       true,
		"settings": snapshot,
	})
}

func (s *Server) handleWeChatLoginStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.im == nil {
		http.Error(w, "im gateway is not configured", http.StatusServiceUnavailable)
		return
	}

	start, err := s.im.StartWeChatLogin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{
		"ok":    true,
		"login": start,
	})
}

func (s *Server) handleWeChatLoginStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.im == nil {
		http.Error(w, "im gateway is not configured", http.StatusServiceUnavailable)
		return
	}

	sessionKey := r.URL.Query().Get("session_key")
	status, err := s.im.PollWeChatLogin(sessionKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{
		"ok":     true,
		"status": status,
	})
}

func (s *Server) handleWeChatLoginConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.im == nil {
		http.Error(w, "im gateway is not configured", http.StatusServiceUnavailable)
		return
	}

	var payload struct {
		SessionKey string `json:"session_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, fmt.Sprintf("decode wechat login confirm payload: %v", err), http.StatusBadRequest)
		return
	}

	account, err := s.im.ConfirmWeChatLogin(payload.SessionKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{
		"ok":      true,
		"account": account,
	})
}

func (s *Server) handleIMDebugSendDefault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.im == nil {
		http.Error(w, "im gateway is not configured", http.StatusServiceUnavailable)
		return
	}

	var payload struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, fmt.Sprintf("decode im debug send payload: %v", err), http.StatusBadRequest)
		return
	}

	receipt, err := s.im.SendTextToDefaultChannel(payload.Text)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{
		"ok":      true,
		"receipt": receipt,
	})
}

func (s *Server) handleIMDebugSendImageDefault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.im == nil {
		http.Error(w, "im gateway is not configured", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		http.Error(w, fmt.Sprintf("parse image upload form: %v", err), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, fmt.Sprintf("read image upload file: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, fmt.Sprintf("read image upload content: %v", err), http.StatusBadRequest)
		return
	}

	receipt, err := s.im.SendImageToDefaultChannel(im.ImageSendRequest{
		FileName: header.Filename,
		MimeType: header.Header.Get("Content-Type"),
		Content:  content,
		Caption:  r.FormValue("caption"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{
		"ok":      true,
		"receipt": receipt,
	})
}

func (s *Server) handleIMTargets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.im == nil {
		http.Error(w, "im gateway is not configured", http.StatusServiceUnavailable)
		return
	}

	var payload struct {
		AccountID    string `json:"account_id"`
		Name         string `json:"name"`
		TargetUserID string `json:"target_user_id"`
		SetDefault   bool   `json:"set_default"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, fmt.Sprintf("decode im target payload: %v", err), http.StatusBadRequest)
		return
	}

	target, err := s.im.UpsertTarget(payload.AccountID, payload.Name, payload.TargetUserID, payload.SetDefault)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{
		"ok":     true,
		"target": target,
	})
}

func (s *Server) handleIMTargetDefault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.im == nil {
		http.Error(w, "im gateway is not configured", http.StatusServiceUnavailable)
		return
	}

	var payload struct {
		AccountID string `json:"account_id"`
		TargetID  string `json:"target_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, fmt.Sprintf("decode default im target payload: %v", err), http.StatusBadRequest)
		return
	}
	if err := s.im.SetDefaultTarget(payload.AccountID, payload.TargetID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleIMTargetDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.im == nil {
		http.Error(w, "im gateway is not configured", http.StatusServiceUnavailable)
		return
	}

	var payload struct {
		TargetID string `json:"target_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, fmt.Sprintf("decode im target delete payload: %v", err), http.StatusBadRequest)
		return
	}
	if err := s.im.DeleteTarget(payload.TargetID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleIMAccountDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.im == nil {
		http.Error(w, "im gateway is not configured", http.StatusServiceUnavailable)
		return
	}

	var payload struct {
		AccountID string `json:"account_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, fmt.Sprintf("decode im account delete payload: %v", err), http.StatusBadRequest)
		return
	}
	if err := s.im.DeleteAccount(payload.AccountID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.tasks != nil {
		if err := s.tasks.Reset(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if s.claude != nil {
		if err := s.claude.Reset(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if s.conversations != nil {
		if err := s.conversations.ResetConversationData(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if s.im != nil {
		if err := s.im.Reset(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
