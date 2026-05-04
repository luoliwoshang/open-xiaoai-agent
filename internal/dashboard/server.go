package dashboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/assistant"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/im"
	runtimelogs "github.com/luoliwoshang/open-xiaoai-agent/internal/logs"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/memory/filememory"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins/complextask"
	agentserver "github.com/luoliwoshang/open-xiaoai-agent/internal/server"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/settings"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/tasks"
)

type memoryLogMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type memoryLogItem struct {
	ID             string             `json:"id"`
	MemoryKey      string             `json:"memory_key"`
	Source         string             `json:"source"`
	SourceLabel    string             `json:"source_label"`
	Preview        string             `json:"preview"`
	SummaryContext []memoryLogMessage `json:"summary_context,omitempty"`
	Before         string             `json:"before"`
	After          string             `json:"after"`
	CreatedAt      string             `json:"created_at"`
}

type memoryLogPageResponse struct {
	Items    []memoryLogItem `json:"items"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	Total    int             `json:"total"`
	HasMore  bool            `json:"has_more"`
}

type Server struct {
	addr          string
	tasks         *tasks.Manager
	claude        *complextask.Service
	conversations interface {
		SnapshotConversations() []assistant.ConversationSnapshot
		RuntimeStatus() assistant.RuntimeStatus
		ResetConversationData() error
		SubmitRecognizedText(text string) error
	}
	xiaoai interface {
		ConnectionStatus() agentserver.ConnectionStatus
	}
	settings interface {
		Snapshot() settings.Snapshot
		UpdateSessionWindowSeconds(seconds int) (settings.Snapshot, error)
		UpdateMemoryStorageDir(dir string) (settings.Snapshot, error)
	}
	memory interface {
		GetFile(memoryKey string) (filememory.ManagedFile, error)
		SaveFile(memoryKey string, content string, source string) (filememory.ManagedFile, error)
		ListLogs(query filememory.ListQuery) (filememory.ListPage, error)
	}
	im interface {
		Snapshot() im.Snapshot
		StartWeChatLogin() (im.WeChatLoginStart, error)
		PollWeChatLogin(sessionKey string) (im.WeChatLoginStatus, error)
		ConfirmWeChatLogin(sessionKey string) (im.Account, error)
		SendTextToDefaultChannel(text string) (im.DeliveryReceipt, error)
		SendImageToDefaultChannel(req im.ImageSendRequest) (im.DeliveryReceipt, error)
		SendFileToDefaultChannel(req im.FileSendRequest) (im.DeliveryReceipt, error)
		UpsertTarget(accountID string, name string, targetUserID string, setDefault bool) (im.Target, error)
		SetDefaultTarget(accountID string, targetID string) error
		DeleteTarget(targetID string) error
		DeleteAccount(accountID string) error
		UpdateDeliveryConfig(enabled bool, accountID string, targetID string) (settings.Snapshot, error)
		Reset() error
	}
	logs interface {
		List(query runtimelogs.ListQuery) (runtimelogs.ListPage, error)
	}
}

func New(addr string, tasks *tasks.Manager, claude *complextask.Service, conversations interface {
	SnapshotConversations() []assistant.ConversationSnapshot
	RuntimeStatus() assistant.RuntimeStatus
	ResetConversationData() error
	SubmitRecognizedText(text string) error
}, xiaoaiStatus interface {
	ConnectionStatus() agentserver.ConnectionStatus
}, runtimeSettings interface {
	Snapshot() settings.Snapshot
	UpdateSessionWindowSeconds(seconds int) (settings.Snapshot, error)
	UpdateMemoryStorageDir(dir string) (settings.Snapshot, error)
}, memoryManager interface {
	GetFile(memoryKey string) (filememory.ManagedFile, error)
	SaveFile(memoryKey string, content string, source string) (filememory.ManagedFile, error)
	ListLogs(query filememory.ListQuery) (filememory.ListPage, error)
}, imGateway interface {
	Snapshot() im.Snapshot
	StartWeChatLogin() (im.WeChatLoginStart, error)
	PollWeChatLogin(sessionKey string) (im.WeChatLoginStatus, error)
	ConfirmWeChatLogin(sessionKey string) (im.Account, error)
	SendTextToDefaultChannel(text string) (im.DeliveryReceipt, error)
	SendImageToDefaultChannel(req im.ImageSendRequest) (im.DeliveryReceipt, error)
	SendFileToDefaultChannel(req im.FileSendRequest) (im.DeliveryReceipt, error)
	UpsertTarget(accountID string, name string, targetUserID string, setDefault bool) (im.Target, error)
	SetDefaultTarget(accountID string, targetID string) error
	DeleteTarget(targetID string) error
	DeleteAccount(accountID string) error
	UpdateDeliveryConfig(enabled bool, accountID string, targetID string) (settings.Snapshot, error)
	Reset() error
}, logStore interface {
	List(query runtimelogs.ListQuery) (runtimelogs.ListPage, error)
}) *Server {
	return &Server{
		addr:          addr,
		tasks:         tasks,
		claude:        claude,
		conversations: conversations,
		xiaoai:        xiaoaiStatus,
		settings:      runtimeSettings,
		memory:        memoryManager,
		im:            imGateway,
		logs:          logStore,
	}
}

func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/healthz", s.handleHealth)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/assistant/asr", s.handleAssistantASR)
	mux.HandleFunc("/api/xiaoai/status", s.handleXiaoAIStatus)
	mux.HandleFunc("/api/tasks/", s.handleTaskArtifactDownload)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/settings/session", s.handleSessionSettings)
	mux.HandleFunc("/api/settings/memory", s.handleMemorySettings)
	mux.HandleFunc("/api/memory/file", s.handleMemoryFile)
	mux.HandleFunc("/api/memory/logs", s.handleMemoryLogs)
	mux.HandleFunc("/api/settings/im-delivery", s.handleIMDeliverySettings)
	mux.HandleFunc("/api/im/wechat/login/start", s.handleWeChatLoginStart)
	mux.HandleFunc("/api/im/wechat/login/status", s.handleWeChatLoginStatus)
	mux.HandleFunc("/api/im/wechat/login/confirm", s.handleWeChatLoginConfirm)
	mux.HandleFunc("/api/im/debug/send-default", s.handleIMDebugSendDefault)
	mux.HandleFunc("/api/im/debug/send-image-default", s.handleIMDebugSendImageDefault)
	mux.HandleFunc("/api/im/debug/send-file-default", s.handleIMDebugSendFileDefault)
	mux.HandleFunc("/api/im/targets", s.handleIMTargets)
	mux.HandleFunc("/api/im/targets/default", s.handleIMTargetDefault)
	mux.HandleFunc("/api/im/targets/delete", s.handleIMTargetDelete)
	mux.HandleFunc("/api/im/accounts/delete", s.handleIMAccountDelete)
	mux.HandleFunc("/api/reset", s.handleReset)

	log.Printf("dashboard api listening on %s", s.addr)
	return http.ListenAndServe(s.addr, mux)
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	tasksList := []tasks.Task(nil)
	events := []tasks.Event(nil)
	artifacts := []tasks.Artifact(nil)
	if s.tasks != nil {
		tasksList, events = s.tasks.Snapshot()
		artifacts = s.tasks.ArtifactsSnapshot()
	}
	claudeRecords := []complextask.Record(nil)
	if s.claude != nil {
		claudeRecords = s.claude.Snapshot()
	}
	conversations := []assistant.ConversationSnapshot(nil)
	var assistantRuntime assistant.RuntimeStatus
	if s.conversations != nil {
		conversations = s.conversations.SnapshotConversations()
		assistantRuntime = s.conversations.RuntimeStatus()
	}
	var xiaoaiStatus agentserver.ConnectionStatus
	if s.xiaoai != nil {
		xiaoaiStatus = s.xiaoai.ConnectionStatus()
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
		"artifacts":      artifacts,
		"claude_records": claudeRecords,
		"conversations":  conversations,
		"assistant":      assistantRuntime,
		"xiaoai":         xiaoaiStatus,
		"settings":       runtimeSettings,
		"im":             imState,
	})
}

func (s *Server) handleXiaoAIStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.xiaoai == nil {
		http.Error(w, "xiaoai status is not configured", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]any{
		"xiaoai": s.xiaoai.ConnectionStatus(),
	})
}

func (s *Server) handleTaskArtifactDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	parts := strings.Split(path, "/")
	if len(parts) != 4 || parts[1] != "artifacts" || parts[3] != "download" {
		http.NotFound(w, r)
		return
	}

	taskID := strings.TrimSpace(parts[0])
	artifactID := strings.TrimSpace(parts[2])
	if taskID == "" || artifactID == "" {
		http.NotFound(w, r)
		return
	}
	if s.tasks == nil {
		http.Error(w, "task manager is not configured", http.StatusServiceUnavailable)
		return
	}

	artifact, ok := s.tasks.GetArtifact(taskID, artifactID)
	if !ok {
		http.NotFound(w, r)
		return
	}

	file, err := os.Open(artifact.StoragePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("open artifact: %v", err), http.StatusNotFound)
		return
	}
	defer file.Close()

	mimeType := strings.TrimSpace(artifact.MIMEType)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", buildAttachmentDisposition(artifact.FileName))
	http.ServeContent(w, r, artifact.FileName, artifact.CreatedAt, file)
}

func buildAttachmentDisposition(fileName string) string {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		fileName = "artifact"
	}
	if value := mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(fileName)}); value != "" {
		return value
	}
	return "attachment; filename*=UTF-8''" + url.PathEscape(filepath.Base(fileName))
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.logs == nil {
		http.Error(w, "runtime logs are not configured", http.StatusServiceUnavailable)
		return
	}

	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid page: %v", err), http.StatusBadRequest)
			return
		}
		page = value
	}

	pageSize := runtimelogs.DefaultPageSize
	if raw := r.URL.Query().Get("page_size"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid page_size: %v", err), http.StatusBadRequest)
			return
		}
		pageSize = value
	}

	query, err := runtimelogs.NormalizeQuery(runtimelogs.ListQuery{
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	logPage, err := s.logs.List(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, logPage)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleAssistantASR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.conversations == nil {
		http.Error(w, "assistant is not configured", http.StatusServiceUnavailable)
		return
	}

	var payload struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, fmt.Sprintf("decode assistant asr payload: %v", err), http.StatusBadRequest)
		return
	}

	err := s.conversations.SubmitRecognizedText(payload.Text)
	switch {
	case err == nil:
		writeJSON(w, map[string]any{
			"ok":   true,
			"text": strings.TrimSpace(payload.Text),
		})
	case errors.Is(err, assistant.ErrVoiceChannelBusy):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
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

func (s *Server) handleMemorySettings(w http.ResponseWriter, r *http.Request) {
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
		StorageDir string `json:"memory_storage_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, fmt.Sprintf("decode memory settings: %v", err), http.StatusBadRequest)
		return
	}

	snapshot, err := s.settings.UpdateMemoryStorageDir(payload.StorageDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, map[string]any{
		"ok":       true,
		"settings": snapshot,
	})
}

func (s *Server) handleMemoryFile(w http.ResponseWriter, r *http.Request) {
	if s.memory == nil {
		http.Error(w, "memory service is not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		memoryKey := strings.TrimSpace(r.URL.Query().Get("memory_key"))
		if memoryKey == "" {
			memoryKey = assistant.MainVoiceHistoryKey
		}
		file, err := s.memory.GetFile(memoryKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{
			"ok":   true,
			"file": file,
		})
	case http.MethodPost:
		var payload struct {
			MemoryKey string `json:"memory_key"`
			Content   string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, fmt.Sprintf("decode memory file payload: %v", err), http.StatusBadRequest)
			return
		}
		memoryKey := strings.TrimSpace(payload.MemoryKey)
		if memoryKey == "" {
			memoryKey = assistant.MainVoiceHistoryKey
		}
		file, err := s.memory.SaveFile(memoryKey, payload.Content, filememory.DashboardManualSource)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{
			"ok":   true,
			"file": file,
		})
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMemoryLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.memory == nil {
		http.Error(w, "memory service is not configured", http.StatusServiceUnavailable)
		return
	}

	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid page: %v", err), http.StatusBadRequest)
			return
		}
		page = value
	}
	pageSize := filememory.DefaultPageSize
	if raw := r.URL.Query().Get("page_size"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid page_size: %v", err), http.StatusBadRequest)
			return
		}
		pageSize = value
	}
	query, err := filememory.NormalizeQuery(filememory.ListQuery{
		Page:      page,
		PageSize:  pageSize,
		MemoryKey: strings.TrimSpace(r.URL.Query().Get("memory_key")),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := s.memory.ListLogs(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]memoryLogItem, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, buildMemoryLogItem(item))
	}
	writeJSON(w, memoryLogPageResponse{
		Items:    items,
		Page:     result.Page,
		PageSize: result.PageSize,
		Total:    result.Total,
		HasMore:  result.HasMore,
	})
}

func buildMemoryLogItem(item filememory.UpdateLog) memoryLogItem {
	contextItems := make([]memoryLogMessage, 0, len(item.Messages))
	for _, message := range item.Messages {
		role := strings.TrimSpace(message.Role)
		content := strings.TrimSpace(message.Content)
		if role == "" || content == "" {
			continue
		}
		contextItems = append(contextItems, memoryLogMessage{
			Role:    role,
			Content: content,
		})
	}
	return memoryLogItem{
		ID:             item.ID,
		MemoryKey:      item.MemoryKey,
		Source:         item.Source,
		SourceLabel:    memoryLogSourceLabel(item.Source),
		Preview:        memoryLogPreview(item.Source, contextItems, item.After),
		SummaryContext: contextItems,
		Before:         item.Before,
		After:          item.After,
		CreatedAt:      item.CreatedAt.Format(time.RFC3339),
	}
}

func memoryLogSourceLabel(source string) string {
	switch strings.TrimSpace(source) {
	case filememory.SessionSummarySource:
		return "会话总结"
	case filememory.DashboardManualSource:
		return "手动编辑"
	default:
		if strings.TrimSpace(source) == "" {
			return "memory"
		}
		return strings.TrimSpace(source)
	}
}

func memoryLogPreview(source string, messages []memoryLogMessage, after string) string {
	if strings.TrimSpace(source) == filememory.SessionSummarySource {
		for _, message := range messages {
			if strings.EqualFold(strings.TrimSpace(message.Role), "user") {
				return fmt.Sprintf("%s：%s", message.Role, message.Content)
			}
		}
	}
	if len(messages) > 0 {
		return fmt.Sprintf("%s：%s", messages[0].Role, messages[0].Content)
	}
	after = strings.TrimSpace(after)
	if after == "" {
		return "手动编辑了记忆文件"
	}
	lines := strings.Split(after, "\n")
	return strings.TrimSpace(lines[0])
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
	upload, err := parseUploadedFile(r, 16<<20, "file")
	if err != nil {
		http.Error(w, fmt.Sprintf("parse image upload form: %v", err), http.StatusBadRequest)
		return
	}

	receipt, err := s.im.SendImageToDefaultChannel(im.ImageSendRequest{
		FileName: upload.FileName,
		MimeType: upload.MimeType,
		Content:  upload.Content,
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

func (s *Server) handleIMDebugSendFileDefault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.im == nil {
		http.Error(w, "im gateway is not configured", http.StatusServiceUnavailable)
		return
	}
	upload, err := parseUploadedFile(r, 32<<20, "file")
	if err != nil {
		http.Error(w, fmt.Sprintf("parse file upload form: %v", err), http.StatusBadRequest)
		return
	}

	receipt, err := s.im.SendFileToDefaultChannel(im.FileSendRequest{
		FileName: upload.FileName,
		MimeType: upload.MimeType,
		Content:  upload.Content,
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

type uploadedFormFile struct {
	FileName string
	MimeType string
	Content  []byte
}

func parseUploadedFile(r *http.Request, maxMemory int64, fieldName string) (uploadedFormFile, error) {
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		return uploadedFormFile{}, err
	}

	file, header, err := r.FormFile(fieldName)
	if err != nil {
		return uploadedFormFile{}, err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return uploadedFormFile{}, err
	}

	return uploadedFormFile{
		FileName: header.Filename,
		MimeType: header.Header.Get("Content-Type"),
		Content:  content,
	}, nil
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
