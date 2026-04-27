package settings

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/storage"
)

const (
	MinSessionWindowSeconds = 30
	MaxSessionWindowSeconds = 3600
)

type Snapshot struct {
	SessionWindowSeconds int `json:"session_window_seconds"`
}

type Store struct {
	mu       sync.RWMutex
	db       *sql.DB
	snapshot Snapshot
}

func NewStore(dsn string) (*Store, error) {
	db, err := storage.OpenRuntimeDB(dsn)
	if err != nil {
		return nil, err
	}

	store := &Store{db: db}
	snapshot, err := store.load()
	if err != nil {
		return nil, err
	}
	store.snapshot = snapshot
	return store, nil
}

func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.snapshot
}

func (s *Store) SessionWindow() time.Duration {
	seconds := s.Snapshot().SessionWindowSeconds
	if seconds <= 0 {
		seconds = storage.DefaultSessionWindowSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (s *Store) UpdateSessionWindowSeconds(seconds int) (Snapshot, error) {
	if err := ValidateSessionWindowSeconds(seconds); err != nil {
		return Snapshot{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := Snapshot{SessionWindowSeconds: seconds}
	if s.db == nil {
		s.snapshot = snapshot
		return snapshot, nil
	}

	if _, err := s.db.Exec(
		`UPDATE settings SET value = ?, updated_at = ? WHERE setting_key = ?`,
		strconv.Itoa(seconds),
		time.Now().UnixMilli(),
		storage.SessionWindowSecondsSettingKey,
	); err != nil {
		return Snapshot{}, fmt.Errorf("update session window seconds: %w", err)
	}

	s.snapshot = snapshot
	return snapshot, nil
}

func ValidateSessionWindowSeconds(seconds int) error {
	switch {
	case seconds < MinSessionWindowSeconds:
		return fmt.Errorf("session window seconds must be at least %d", MinSessionWindowSeconds)
	case seconds > MaxSessionWindowSeconds:
		return fmt.Errorf("session window seconds must be at most %d", MaxSessionWindowSeconds)
	default:
		return nil
	}
}

func (s *Store) load() (Snapshot, error) {
	if s.db == nil {
		return Snapshot{SessionWindowSeconds: storage.DefaultSessionWindowSeconds}, nil
	}

	var raw string
	err := s.db.QueryRow(
		`SELECT value FROM settings WHERE setting_key = ?`,
		storage.SessionWindowSecondsSettingKey,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return s.repairDefault()
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("load session window seconds: %w", err)
	}

	seconds, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse session window seconds %q: %w", raw, err)
	}
	if err := ValidateSessionWindowSeconds(seconds); err != nil {
		return Snapshot{}, err
	}

	return Snapshot{SessionWindowSeconds: seconds}, nil
}

func (s *Store) repairDefault() (Snapshot, error) {
	snapshot := Snapshot{SessionWindowSeconds: storage.DefaultSessionWindowSeconds}
	if _, err := s.db.Exec(
		`INSERT INTO settings (setting_key, value, updated_at) VALUES (?, ?, ?)`,
		storage.SessionWindowSecondsSettingKey,
		strconv.Itoa(snapshot.SessionWindowSeconds),
		time.Now().UnixMilli(),
	); err != nil {
		return Snapshot{}, fmt.Errorf("insert default session window seconds: %w", err)
	}
	return snapshot, nil
}
