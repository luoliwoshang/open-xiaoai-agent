package tasks

import (
	"database/sql"
	"fmt"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/storage"
)

type Store struct {
	db *sql.DB
}

func NewStore(dsn string) (*Store, error) {
	db, err := storage.OpenRuntimeDB(dsn)
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Load() (fileState, error) {
	if s == nil || s.db == nil {
		return fileState{Version: 1}, nil
	}

	state := fileState{Version: 1}

	taskRows, err := s.db.Query(`
		SELECT id, plugin, kind, title, input, parent_task_id, state, summary, result, report_pending, created_at, updated_at
		FROM tasks
	`)
	if err != nil {
		return fileState{}, fmt.Errorf("query tasks: %w", err)
	}
	defer taskRows.Close()

	for taskRows.Next() {
		var task Task
		var reportPending bool
		var createdAt, updatedAt int64
		if err := taskRows.Scan(
			&task.ID,
			&task.Plugin,
			&task.Kind,
			&task.Title,
			&task.Input,
			&task.ParentTaskID,
			&task.State,
			&task.Summary,
			&task.Result,
			&reportPending,
			&createdAt,
			&updatedAt,
		); err != nil {
			return fileState{}, fmt.Errorf("scan task row: %w", err)
		}
		task.ReportPending = reportPending
		task.CreatedAt = storage.TimeFromUnixMillis(createdAt)
		task.UpdatedAt = storage.TimeFromUnixMillis(updatedAt)
		state.Tasks = append(state.Tasks, task)
	}
	if err := taskRows.Err(); err != nil {
		return fileState{}, fmt.Errorf("iterate task rows: %w", err)
	}

	eventRows, err := s.db.Query(`
		SELECT id, task_id, type, message, created_at
		FROM task_events
	`)
	if err != nil {
		return fileState{}, fmt.Errorf("query task events: %w", err)
	}
	defer eventRows.Close()

	for eventRows.Next() {
		var event Event
		var createdAt int64
		if err := eventRows.Scan(
			&event.ID,
			&event.TaskID,
			&event.Type,
			&event.Message,
			&createdAt,
		); err != nil {
			return fileState{}, fmt.Errorf("scan task event row: %w", err)
		}
		event.CreatedAt = storage.TimeFromUnixMillis(createdAt)
		state.Events = append(state.Events, event)
	}
	if err := eventRows.Err(); err != nil {
		return fileState{}, fmt.Errorf("iterate task event rows: %w", err)
	}

	return state, nil
}

func (s *Store) Save(state fileState) error {
	if s == nil || s.db == nil {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin task save: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM task_events`); err != nil {
		return fmt.Errorf("clear task events: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM tasks`); err != nil {
		return fmt.Errorf("clear tasks: %w", err)
	}

	taskStmt, err := tx.Prepare(`
		INSERT INTO tasks (id, plugin, kind, title, input, parent_task_id, state, summary, result, report_pending, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare task insert: %w", err)
	}
	defer taskStmt.Close()

	for _, task := range state.Tasks {
		if _, err := taskStmt.Exec(
			task.ID,
			task.Plugin,
			task.Kind,
			task.Title,
			task.Input,
			task.ParentTaskID,
			string(task.State),
			task.Summary,
			task.Result,
			task.ReportPending,
			storage.UnixMillis(task.CreatedAt),
			storage.UnixMillis(task.UpdatedAt),
		); err != nil {
			return fmt.Errorf("insert task %q: %w", task.ID, err)
		}
	}

	eventStmt, err := tx.Prepare(`
		INSERT INTO task_events (id, task_id, type, message, created_at)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare event insert: %w", err)
	}
	defer eventStmt.Close()

	for _, event := range state.Events {
		if _, err := eventStmt.Exec(
			event.ID,
			event.TaskID,
			event.Type,
			event.Message,
			storage.UnixMillis(event.CreatedAt),
		); err != nil {
			return fmt.Errorf("insert task event %q: %w", event.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit task save: %w", err)
	}
	return nil
}

func (s *Store) Reset() error {
	if s == nil || s.db == nil {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin task reset: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM task_events`); err != nil {
		return fmt.Errorf("reset task events: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM tasks`); err != nil {
		return fmt.Errorf("reset tasks: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit task reset: %w", err)
	}
	return nil
}
