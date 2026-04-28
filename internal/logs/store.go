package logs

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/storage"
)

type Store struct {
	db  *sql.DB
	seq uint64
}

func NewStore(dsn string) (*Store, error) {
	db, err := storage.OpenRuntimeDB(dsn)
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Append(entry Entry) error {
	if s == nil || s.db == nil {
		return nil
	}

	entry.Level = normalizeLevel(entry.Level)
	entry.Source = normalizeField(entry.Source)
	entry.Message = normalizeField(entry.Message)
	entry.Raw = normalizeField(entry.Raw)
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	if entry.ID == "" {
		entry.ID = s.nextID()
	}

	if _, err := s.db.Exec(
		`INSERT INTO runtime_logs (id, level, source, message, raw, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		entry.ID,
		entry.Level,
		entry.Source,
		entry.Message,
		entry.Raw,
		storage.UnixMillis(entry.CreatedAt),
	); err != nil {
		return fmt.Errorf("insert runtime log: %w", err)
	}
	return nil
}

func (s *Store) List(query ListQuery) (ListPage, error) {
	query, err := NormalizeQuery(query)
	if err != nil {
		return ListPage{}, err
	}
	if s == nil || s.db == nil {
		return ListPage{Page: query.Page, PageSize: query.PageSize}, nil
	}

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM runtime_logs`).Scan(&total); err != nil {
		return ListPage{}, fmt.Errorf("count runtime logs: %w", err)
	}

	rows, err := s.db.Query(
		`SELECT id, level, source, message, raw, created_at
		FROM runtime_logs
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?`,
		query.PageSize,
		(query.Page-1)*query.PageSize,
	)
	if err != nil {
		return ListPage{}, fmt.Errorf("query runtime logs: %w", err)
	}
	defer rows.Close()

	page := ListPage{
		Page:     query.Page,
		PageSize: query.PageSize,
		Total:    total,
	}
	for rows.Next() {
		var item Entry
		var createdAt int64
		if err := rows.Scan(&item.ID, &item.Level, &item.Source, &item.Message, &item.Raw, &createdAt); err != nil {
			return ListPage{}, fmt.Errorf("scan runtime log row: %w", err)
		}
		item.CreatedAt = storage.TimeFromUnixMillis(createdAt)
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return ListPage{}, fmt.Errorf("iterate runtime log rows: %w", err)
	}
	page.HasMore = query.Page*query.PageSize < total
	return page, nil
}

func (s *Store) Reset() error {
	if s == nil || s.db == nil {
		return nil
	}
	if _, err := s.db.Exec(`DELETE FROM runtime_logs`); err != nil {
		return fmt.Errorf("reset runtime logs: %w", err)
	}
	return nil
}

func (s *Store) nextID() string {
	return fmt.Sprintf("log_%d_%d", time.Now().UnixMilli(), atomic.AddUint64(&s.seq, 1))
}

func normalizeField(value string) string {
	if value == "" {
		return ""
	}
	return value
}
