package complextask

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Status string

const (
	StatusAccepted  Status = "accepted"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type Record struct {
	TaskID            string    `json:"task_id"`
	SessionID         string    `json:"session_id"`
	Prompt            string    `json:"prompt"`
	WorkingDirectory  string    `json:"working_directory"`
	Status            Status    `json:"status"`
	LastSummary       string    `json:"last_summary"`
	LastAssistantText string    `json:"last_assistant_text"`
	Result            string    `json:"result"`
	Error             string    `json:"error"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type stateFile struct {
	Version int      `json:"version"`
	Records []Record `json:"records"`
}

type Store struct {
	mu    sync.Mutex
	path  string
	state stateFile
}

func NewStore(path string) (*Store, error) {
	store := &Store{path: path}
	state, err := store.load()
	if err != nil {
		return nil, err
	}
	store.state = state
	return store, nil
}

func (s *Store) Snapshot() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()

	records := append([]Record(nil), s.state.Records...)
	sort.Slice(records, func(i, j int) bool {
		return records[i].UpdatedAt.After(records[j].UpdatedAt)
	})
	return records
}

func (s *Store) Get(taskID string) (Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, record := range s.state.Records {
		if record.TaskID == taskID {
			return record, true
		}
	}
	return Record{}, false
}

func (s *Store) Start(taskID string, prompt string, cwd string) error {
	return s.mutate(taskID, func(record *Record, now time.Time) {
		if record.CreatedAt.IsZero() {
			record.CreatedAt = now
		}
		record.TaskID = taskID
		record.Prompt = prompt
		record.WorkingDirectory = cwd
		record.Status = StatusAccepted
		record.UpdatedAt = now
	})
}

func (s *Store) MarkRunning(taskID string) error {
	return s.mutate(taskID, func(record *Record, now time.Time) {
		record.Status = StatusRunning
		record.UpdatedAt = now
	})
}

func (s *Store) SetSession(taskID string, sessionID string) error {
	return s.mutate(taskID, func(record *Record, now time.Time) {
		record.SessionID = sessionID
		record.UpdatedAt = now
	})
}

func (s *Store) UpdateSummary(taskID string, summary string, assistantText string) error {
	return s.mutate(taskID, func(record *Record, now time.Time) {
		record.LastSummary = summary
		if assistantText != "" {
			record.LastAssistantText = assistantText
		}
		record.UpdatedAt = now
	})
}

func (s *Store) Complete(taskID string, result string) error {
	return s.mutate(taskID, func(record *Record, now time.Time) {
		record.Status = StatusCompleted
		record.Result = result
		record.Error = ""
		record.UpdatedAt = now
	})
}

func (s *Store) Fail(taskID string, message string) error {
	return s.mutate(taskID, func(record *Record, now time.Time) {
		record.Status = StatusFailed
		record.Error = message
		record.UpdatedAt = now
	})
}

func (s *Store) mutate(taskID string, fn func(record *Record, now time.Time)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	record := s.findOrCreateLocked(taskID)
	fn(record, now)
	return s.saveLocked()
}

func (s *Store) findOrCreateLocked(taskID string) *Record {
	for index := range s.state.Records {
		if s.state.Records[index].TaskID == taskID {
			return &s.state.Records[index]
		}
	}
	s.state.Records = append(s.state.Records, Record{TaskID: taskID})
	return &s.state.Records[len(s.state.Records)-1]
}

func (s *Store) load() (stateFile, error) {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return stateFile{}, fmt.Errorf("create claude state dir: %w", err)
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return stateFile{Version: 1}, nil
		}
		return stateFile{}, fmt.Errorf("read claude state: %w", err)
	}
	if len(data) == 0 {
		return stateFile{Version: 1}, nil
	}

	var state stateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return stateFile{}, fmt.Errorf("decode claude state: %w", err)
	}
	if state.Version == 0 {
		state.Version = 1
	}
	return state, nil
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create claude state dir: %w", err)
	}

	s.state.Version = 1
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode claude state: %w", err)
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write claude temp state: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace claude state: %w", err)
	}
	return nil
}
