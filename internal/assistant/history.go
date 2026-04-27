package assistant

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/llm"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/storage"
)

type sessionKeyer interface {
	HistoryKey() string
}

type sessionWindowProvider interface {
	SessionWindow() time.Duration
}

type conversationHistory struct {
	id         string
	startedAt  time.Time
	lastActive time.Time
	messages   []llm.Message
}

type ConversationSnapshot struct {
	ID         string        `json:"id"`
	StartedAt  time.Time     `json:"started_at"`
	LastActive time.Time     `json:"last_active"`
	Messages   []llm.Message `json:"messages"`
}

type historyStore struct {
	mu       sync.Mutex
	settings sessionWindowProvider
	db       *sql.DB
	entries  map[string]*conversationHistory
}

func newHistoryStore(settings sessionWindowProvider, dsn string) (*historyStore, error) {
	db, err := storage.OpenRuntimeDB(dsn)
	if err != nil {
		return nil, err
	}

	store := &historyStore{
		settings: settings,
		db:       db,
		entries:  make(map[string]*conversationHistory),
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *historyStore) Snapshot(session any, now time.Time) []llm.Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	changed := s.pruneExpiredLocked(now)
	entry, created := s.ensureEntryLocked(session, now, false)
	if changed || created {
		s.saveLocked()
	}
	return cloneMessages(entry.messages)
}

func (s *historyStore) AppendTurn(session any, now time.Time, user string, assistant string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	changed := s.pruneExpiredLocked(now)
	entry, created := s.ensureEntryLocked(session, now, true)
	if user != "" {
		entry.messages = append(entry.messages, llm.Message{
			Role:    "user",
			Content: user,
		})
		changed = true
	}
	if assistant != "" {
		entry.messages = append(entry.messages, llm.Message{
			Role:    "assistant",
			Content: assistant,
		})
		changed = true
	}
	entry.lastActive = now
	if changed || created {
		s.saveLocked()
	}
}

func (s *historyStore) SnapshotAll(now time.Time) []ConversationSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pruneExpiredLocked(now) {
		s.saveLocked()
	}

	conversations := make([]ConversationSnapshot, 0, len(s.entries))
	for _, entry := range s.entries {
		conversations = append(conversations, ConversationSnapshot{
			ID:         entry.id,
			StartedAt:  entry.startedAt,
			LastActive: entry.lastActive,
			Messages:   cloneMessages(entry.messages),
		})
	}
	sort.Slice(conversations, func(i, j int) bool {
		return conversations[i].LastActive.After(conversations[j].LastActive)
	})
	return conversations
}

func (s *historyStore) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = make(map[string]*conversationHistory)
	if s.db == nil {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin conversation reset: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM conversation_messages`); err != nil {
		return fmt.Errorf("reset conversation messages: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM conversations`); err != nil {
		return fmt.Errorf("reset conversations: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit conversation reset: %w", err)
	}
	return nil
}

func (s *historyStore) load() error {
	if s.db == nil {
		return nil
	}

	rows, err := s.db.Query(`
		SELECT id, started_at, last_active
		FROM conversations
	`)
	if err != nil {
		return fmt.Errorf("query conversations: %w", err)
	}
	defer rows.Close()

	now := time.Now()
	for rows.Next() {
		var id string
		var startedAt, lastActive int64
		if err := rows.Scan(&id, &startedAt, &lastActive); err != nil {
			return fmt.Errorf("scan conversation: %w", err)
		}
		started := storage.TimeFromUnixMillis(startedAt)
		last := storage.TimeFromUnixMillis(lastActive)
		if last.IsZero() {
			last = started
		}
		if strings.TrimSpace(id) == "" || now.Sub(last) > s.sessionWindowLocked() {
			continue
		}
		s.entries[id] = &conversationHistory{
			id:         id,
			startedAt:  started,
			lastActive: last,
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate conversations: %w", err)
	}

	messageRows, err := s.db.Query(`
		SELECT conversation_id, role, content
		FROM conversation_messages
		ORDER BY conversation_id, message_index
	`)
	if err != nil {
		return fmt.Errorf("query conversation messages: %w", err)
	}
	defer messageRows.Close()

	for messageRows.Next() {
		var conversationID string
		var role string
		var content string
		if err := messageRows.Scan(&conversationID, &role, &content); err != nil {
			return fmt.Errorf("scan conversation message: %w", err)
		}
		entry, ok := s.entries[conversationID]
		if !ok {
			continue
		}
		entry.messages = append(entry.messages, llm.Message{
			Role:    role,
			Content: content,
		})
	}
	if err := messageRows.Err(); err != nil {
		return fmt.Errorf("iterate conversation messages: %w", err)
	}
	return nil
}

func (s *historyStore) ensureEntryLocked(session any, now time.Time, touch bool) (*conversationHistory, bool) {
	key := conversationKey(session)
	entry, ok := s.entries[key]
	if !ok || now.Sub(entry.lastActive) > s.sessionWindowLocked() {
		entry = &conversationHistory{
			id:         key,
			startedAt:  now,
			lastActive: now,
		}
		s.entries[key] = entry
		return entry, true
	}

	if touch {
		entry.lastActive = now
	}
	return entry, false
}

func (s *historyStore) pruneExpiredLocked(now time.Time) bool {
	changed := false
	window := s.sessionWindowLocked()
	for key, entry := range s.entries {
		if now.Sub(entry.lastActive) <= window {
			continue
		}
		delete(s.entries, key)
		changed = true
	}
	return changed
}

func (s *historyStore) saveLocked() {
	if s.db == nil {
		return
	}

	tx, err := s.db.Begin()
	if err != nil {
		log.Printf("conversation save failed: begin: %v", err)
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM conversation_messages`); err != nil {
		log.Printf("conversation save failed: clear messages: %v", err)
		return
	}
	if _, err := tx.Exec(`DELETE FROM conversations`); err != nil {
		log.Printf("conversation save failed: clear conversations: %v", err)
		return
	}

	conversationStmt, err := tx.Prepare(`
		INSERT INTO conversations (id, started_at, last_active)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		log.Printf("conversation save failed: prepare conversations: %v", err)
		return
	}
	defer conversationStmt.Close()

	messageStmt, err := tx.Prepare(`
		INSERT INTO conversation_messages (conversation_id, message_index, role, content)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		log.Printf("conversation save failed: prepare messages: %v", err)
		return
	}
	defer messageStmt.Close()

	conversations := make([]ConversationSnapshot, 0, len(s.entries))
	for _, entry := range s.entries {
		conversations = append(conversations, ConversationSnapshot{
			ID:         entry.id,
			StartedAt:  entry.startedAt,
			LastActive: entry.lastActive,
			Messages:   cloneMessages(entry.messages),
		})
	}
	sort.Slice(conversations, func(i, j int) bool {
		return conversations[i].LastActive.After(conversations[j].LastActive)
	})

	for _, conversation := range conversations {
		if _, err := conversationStmt.Exec(
			conversation.ID,
			storage.UnixMillis(conversation.StartedAt),
			storage.UnixMillis(conversation.LastActive),
		); err != nil {
			log.Printf("conversation save failed: insert conversation: %v", err)
			return
		}
		for index, message := range conversation.Messages {
			if _, err := messageStmt.Exec(
				conversation.ID,
				index,
				message.Role,
				message.Content,
			); err != nil {
				log.Printf("conversation save failed: insert message: %v", err)
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("conversation save failed: commit: %v", err)
	}
}

func (s *historyStore) sessionWindowLocked() time.Duration {
	if s.settings == nil {
		return 5 * time.Minute
	}
	window := s.settings.SessionWindow()
	if window <= 0 {
		return 5 * time.Minute
	}
	return window
}

func conversationKey(session any) string {
	if keyer, ok := session.(sessionKeyer); ok {
		if key := strings.TrimSpace(keyer.HistoryKey()); key != "" {
			return key
		}
	}
	return fmt.Sprintf("%p", session)
}

func cloneMessages(messages []llm.Message) []llm.Message {
	if len(messages) == 0 {
		return nil
	}
	cloned := make([]llm.Message, len(messages))
	copy(cloned, messages)
	return cloned
}
