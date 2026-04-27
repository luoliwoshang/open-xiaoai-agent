package assistant

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/llm"
)

type sessionKeyer interface {
	HistoryKey() string
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

type conversationFile struct {
	Version       int                    `json:"version"`
	Conversations []ConversationSnapshot `json:"conversations"`
}

type historyStore struct {
	mu      sync.Mutex
	window  time.Duration
	path    string
	entries map[string]*conversationHistory
}

func newHistoryStore(window time.Duration, path string) (*historyStore, error) {
	if window <= 0 {
		window = 5 * time.Minute
	}

	store := &historyStore{
		window:  window,
		path:    strings.TrimSpace(path),
		entries: make(map[string]*conversationHistory),
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
	entry, created := s.ensureEntryLocked(session, now)
	if changed || created {
		s.saveLocked()
	}
	return cloneMessages(entry.messages)
}

func (s *historyStore) AppendTurn(session any, now time.Time, user string, assistant string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	changed := s.pruneExpiredLocked(now)
	entry, created := s.ensureEntryLocked(session, now)
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

func (s *historyStore) load() error {
	if strings.TrimSpace(s.path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create conversation dir: %w", err)
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read conversations: %w", err)
	}
	if len(data) == 0 {
		return nil
	}

	var state conversationFile
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("decode conversations: %w", err)
	}

	now := time.Now()
	for _, item := range state.Conversations {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		if now.Sub(item.StartedAt) > s.window {
			continue
		}
		s.entries[item.ID] = &conversationHistory{
			id:         item.ID,
			startedAt:  item.StartedAt,
			lastActive: item.LastActive,
			messages:   cloneMessages(item.Messages),
		}
	}
	return nil
}

func (s *historyStore) ensureEntryLocked(session any, now time.Time) (*conversationHistory, bool) {
	key := conversationKey(session)
	entry, ok := s.entries[key]
	if !ok || now.Sub(entry.startedAt) > s.window {
		entry = &conversationHistory{
			id:         key,
			startedAt:  now,
			lastActive: now,
		}
		s.entries[key] = entry
		return entry, true
	}

	entry.lastActive = now
	return entry, false
}

func (s *historyStore) pruneExpiredLocked(now time.Time) bool {
	changed := false
	for key, entry := range s.entries {
		if now.Sub(entry.startedAt) <= s.window {
			continue
		}
		delete(s.entries, key)
		changed = true
	}
	return changed
}

func (s *historyStore) saveLocked() {
	if strings.TrimSpace(s.path) == "" {
		return
	}
	state := conversationFile{
		Version:       1,
		Conversations: make([]ConversationSnapshot, 0, len(s.entries)),
	}
	for _, entry := range s.entries {
		state.Conversations = append(state.Conversations, ConversationSnapshot{
			ID:         entry.id,
			StartedAt:  entry.startedAt,
			LastActive: entry.lastActive,
			Messages:   cloneMessages(entry.messages),
		})
	}
	sort.Slice(state.Conversations, func(i, j int) bool {
		return state.Conversations[i].LastActive.After(state.Conversations[j].LastActive)
	})

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		log.Printf("conversation save failed: encode: %v", err)
		return
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, append(data, '\n'), 0o644); err != nil {
		log.Printf("conversation save failed: write temp: %v", err)
		return
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		log.Printf("conversation save failed: replace: %v", err)
	}
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
