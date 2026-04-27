package dashboard

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/assistant"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins/complextask"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/tasks"
)

type Server struct {
	addr          string
	tasks         *tasks.Manager
	claude        *complextask.Service
	conversations interface {
		SnapshotConversations() []assistant.ConversationSnapshot
	}
}

func New(addr string, tasks *tasks.Manager, claude *complextask.Service, conversations interface {
	SnapshotConversations() []assistant.ConversationSnapshot
}) *Server {
	return &Server{addr: addr, tasks: tasks, claude: claude, conversations: conversations}
}

func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/healthz", s.handleHealth)
	mux.HandleFunc("/api/state", s.handleState)

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
	writeJSON(w, map[string]any{
		"tasks":          tasksList,
		"events":         events,
		"claude_records": claudeRecords,
		"conversations":  conversations,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
