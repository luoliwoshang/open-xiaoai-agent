package complextask

import (
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/storage"
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
	db    *sql.DB
	state stateFile
}

func NewStore(dsn string) (*Store, error) {
	db, err := storage.OpenRuntimeDB(dsn)
	if err != nil {
		return nil, err
	}

	store := &Store{db: db}
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

	if record, ok := s.findLocked(taskID); ok {
		return *record, true
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
	return s.mutateExisting(taskID, func(record *Record, now time.Time) {
		record.Status = StatusRunning
		record.UpdatedAt = now
	})
}

func (s *Store) SetSession(taskID string, sessionID string) error {
	return s.mutateExisting(taskID, func(record *Record, now time.Time) {
		record.SessionID = sessionID
		record.UpdatedAt = now
	})
}

func (s *Store) UpdateSummary(taskID string, summary string, assistantText string) error {
	return s.mutateExisting(taskID, func(record *Record, now time.Time) {
		record.LastSummary = summary
		if assistantText != "" {
			record.LastAssistantText = assistantText
		}
		record.UpdatedAt = now
	})
}

func (s *Store) Complete(taskID string, result string) error {
	return s.mutateExisting(taskID, func(record *Record, now time.Time) {
		record.Status = StatusCompleted
		record.Result = result
		record.Error = ""
		record.UpdatedAt = now
	})
}

func (s *Store) Fail(taskID string, message string) error {
	return s.mutateExisting(taskID, func(record *Record, now time.Time) {
		record.Status = StatusFailed
		record.Error = message
		record.UpdatedAt = now
	})
}

func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state = stateFile{Version: 1}
	if s.db == nil {
		return nil
	}
	if _, err := s.db.Exec(`DELETE FROM claude_records`); err != nil {
		return fmt.Errorf("reset claude records: %w", err)
	}
	return nil
}

func (s *Store) mutate(taskID string, fn func(record *Record, now time.Time)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	record := s.findOrCreateLocked(taskID)
	fn(record, now)
	return s.saveLocked()
}

func (s *Store) mutateExisting(taskID string, fn func(record *Record, now time.Time)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.findLocked(taskID)
	if !ok {
		return nil
	}

	fn(record, time.Now())
	return s.saveLocked()
}

func (s *Store) findLocked(taskID string) (*Record, bool) {
	for index := range s.state.Records {
		if s.state.Records[index].TaskID == taskID {
			return &s.state.Records[index], true
		}
	}
	return nil, false
}

func (s *Store) findOrCreateLocked(taskID string) *Record {
	if record, ok := s.findLocked(taskID); ok {
		return record
	}
	s.state.Records = append(s.state.Records, Record{TaskID: taskID})
	return &s.state.Records[len(s.state.Records)-1]
}

func (s *Store) load() (stateFile, error) {
	if s.db == nil {
		return stateFile{Version: 1}, nil
	}

	rows, err := s.db.Query(`
		SELECT task_id, session_id, prompt, working_directory, status, last_summary, last_assistant_text, result, error, created_at, updated_at
		FROM claude_records
	`)
	if err != nil {
		return stateFile{}, fmt.Errorf("query claude records: %w", err)
	}
	defer rows.Close()

	state := stateFile{Version: 1}
	for rows.Next() {
		var record Record
		var createdAt, updatedAt int64
		if err := rows.Scan(
			&record.TaskID,
			&record.SessionID,
			&record.Prompt,
			&record.WorkingDirectory,
			&record.Status,
			&record.LastSummary,
			&record.LastAssistantText,
			&record.Result,
			&record.Error,
			&createdAt,
			&updatedAt,
		); err != nil {
			return stateFile{}, fmt.Errorf("scan claude record: %w", err)
		}
		record.CreatedAt = storage.TimeFromUnixMillis(createdAt)
		record.UpdatedAt = storage.TimeFromUnixMillis(updatedAt)
		state.Records = append(state.Records, record)
	}
	if err := rows.Err(); err != nil {
		return stateFile{}, fmt.Errorf("iterate claude records: %w", err)
	}
	return state, nil
}

func (s *Store) saveLocked() error {
	if s.db == nil {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin claude save: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM claude_records`); err != nil {
		return fmt.Errorf("clear claude records: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO claude_records (task_id, session_id, prompt, working_directory, status, last_summary, last_assistant_text, result, error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare claude insert: %w", err)
	}
	defer stmt.Close()

	for _, record := range s.state.Records {
		if _, err := stmt.Exec(
			record.TaskID,
			record.SessionID,
			record.Prompt,
			record.WorkingDirectory,
			string(record.Status),
			record.LastSummary,
			record.LastAssistantText,
			record.Result,
			record.Error,
			storage.UnixMillis(record.CreatedAt),
			storage.UnixMillis(record.UpdatedAt),
		); err != nil {
			return fmt.Errorf("insert claude record %q: %w", record.TaskID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit claude save: %w", err)
	}
	return nil
}
