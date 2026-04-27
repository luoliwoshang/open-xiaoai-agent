package dashboard

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/assistant"
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
}

func New(addr string, tasks *tasks.Manager, claude *complextask.Service, conversations interface {
	SnapshotConversations() []assistant.ConversationSnapshot
	ResetConversationData() error
}, runtimeSettings interface {
	Snapshot() settings.Snapshot
	UpdateSessionWindowSeconds(seconds int) (settings.Snapshot, error)
}) *Server {
	return &Server{
		addr:          addr,
		tasks:         tasks,
		claude:        claude,
		conversations: conversations,
		settings:      runtimeSettings,
	}
}

func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/healthz", s.handleHealth)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/settings/session", s.handleSessionSettings)
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
	writeJSON(w, map[string]any{
		"tasks":          tasksList,
		"events":         events,
		"claude_records": claudeRecords,
		"conversations":  conversations,
		"settings":       runtimeSettings,
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
		"session": s.settings.Snapshot(),
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

	writeJSON(w, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
