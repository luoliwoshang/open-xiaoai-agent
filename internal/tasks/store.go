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
		SELECT id, plugin, kind, title, input, parent_task_id, state, summary, result, result_report_pending, created_at, updated_at
		FROM tasks
	`)
	if err != nil {
		return fileState{}, fmt.Errorf("query tasks: %w", err)
	}
	defer taskRows.Close()

	for taskRows.Next() {
		var task Task
		var resultReportPending bool
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
			&resultReportPending,
			&createdAt,
			&updatedAt,
		); err != nil {
			return fileState{}, fmt.Errorf("scan task row: %w", err)
		}
		task.ResultReportPending = resultReportPending
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

	artifactRows, err := s.db.Query(`
		SELECT id, task_id, kind, file_name, mime_type, storage_path, size_bytes, created_at
		FROM task_artifacts
	`)
	if err != nil {
		return fileState{}, fmt.Errorf("query task artifacts: %w", err)
	}
	defer artifactRows.Close()

	for artifactRows.Next() {
		var artifact Artifact
		var createdAt int64
		if err := artifactRows.Scan(
			&artifact.ID,
			&artifact.TaskID,
			&artifact.Kind,
			&artifact.FileName,
			&artifact.MIMEType,
			&artifact.StoragePath,
			&artifact.SizeBytes,
			&createdAt,
		); err != nil {
			return fileState{}, fmt.Errorf("scan task artifact row: %w", err)
		}
		artifact.CreatedAt = storage.TimeFromUnixMillis(createdAt)
		state.Artifacts = append(state.Artifacts, artifact)
	}
	if err := artifactRows.Err(); err != nil {
		return fileState{}, fmt.Errorf("iterate task artifact rows: %w", err)
	}

	deliveryRows, err := s.db.Query(`
		SELECT id, task_id, artifact_id, account_id, target_id, channel_label, status, provider_message_id, last_error, created_at, updated_at, delivered_at
		FROM task_artifact_deliveries
	`)
	if err != nil {
		return fileState{}, fmt.Errorf("query task artifact deliveries: %w", err)
	}
	defer deliveryRows.Close()

	for deliveryRows.Next() {
		var delivery ArtifactDelivery
		var createdAt, updatedAt, deliveredAt int64
		if err := deliveryRows.Scan(
			&delivery.ID,
			&delivery.TaskID,
			&delivery.ArtifactID,
			&delivery.AccountID,
			&delivery.TargetID,
			&delivery.ChannelLabel,
			&delivery.Status,
			&delivery.ProviderMessageID,
			&delivery.LastError,
			&createdAt,
			&updatedAt,
			&deliveredAt,
		); err != nil {
			return fileState{}, fmt.Errorf("scan task artifact delivery row: %w", err)
		}
		delivery.CreatedAt = storage.TimeFromUnixMillis(createdAt)
		delivery.UpdatedAt = storage.TimeFromUnixMillis(updatedAt)
		delivery.DeliveredAt = storage.TimeFromUnixMillis(deliveredAt)
		state.Deliveries = append(state.Deliveries, delivery)
	}
	if err := deliveryRows.Err(); err != nil {
		return fileState{}, fmt.Errorf("iterate task artifact delivery rows: %w", err)
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
	if _, err := tx.Exec(`DELETE FROM task_artifact_deliveries`); err != nil {
		return fmt.Errorf("clear task artifact deliveries: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM task_artifacts`); err != nil {
		return fmt.Errorf("clear task artifacts: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM tasks`); err != nil {
		return fmt.Errorf("clear tasks: %w", err)
	}

	taskStmt, err := tx.Prepare(`
		INSERT INTO tasks (id, plugin, kind, title, input, parent_task_id, state, summary, result, result_report_pending, created_at, updated_at)
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
			task.ResultReportPending,
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

	artifactStmt, err := tx.Prepare(`
		INSERT INTO task_artifacts (id, task_id, kind, file_name, mime_type, storage_path, size_bytes, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare task artifact insert: %w", err)
	}
	defer artifactStmt.Close()

	for _, artifact := range state.Artifacts {
		if _, err := artifactStmt.Exec(
			artifact.ID,
			artifact.TaskID,
			artifact.Kind,
			artifact.FileName,
			artifact.MIMEType,
			artifact.StoragePath,
			artifact.SizeBytes,
			storage.UnixMillis(artifact.CreatedAt),
		); err != nil {
			return fmt.Errorf("insert task artifact %q: %w", artifact.ID, err)
		}
	}

	deliveryStmt, err := tx.Prepare(`
		INSERT INTO task_artifact_deliveries (id, task_id, artifact_id, account_id, target_id, channel_label, status, provider_message_id, last_error, created_at, updated_at, delivered_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare task artifact delivery insert: %w", err)
	}
	defer deliveryStmt.Close()

	for _, delivery := range state.Deliveries {
		if _, err := deliveryStmt.Exec(
			delivery.ID,
			delivery.TaskID,
			delivery.ArtifactID,
			delivery.AccountID,
			delivery.TargetID,
			delivery.ChannelLabel,
			string(delivery.Status),
			delivery.ProviderMessageID,
			delivery.LastError,
			storage.UnixMillis(delivery.CreatedAt),
			storage.UnixMillis(delivery.UpdatedAt),
			storage.UnixMillis(delivery.DeliveredAt),
		); err != nil {
			return fmt.Errorf("insert task artifact delivery %q: %w", delivery.ID, err)
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
	if _, err := tx.Exec(`DELETE FROM task_artifact_deliveries`); err != nil {
		return fmt.Errorf("reset task artifact deliveries: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM task_artifacts`); err != nil {
		return fmt.Errorf("reset task artifacts: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM tasks`); err != nil {
		return fmt.Errorf("reset tasks: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit task reset: %w", err)
	}
	return nil
}
