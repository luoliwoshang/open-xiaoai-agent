package settings

import (
	"database/sql"
	"fmt"
	"log"
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
	SessionWindowSeconds int    `json:"session_window_seconds"`
	IMDeliveryEnabled    bool   `json:"im_delivery_enabled"`
	IMSelectedAccountID  string `json:"im_selected_account_id"`
	IMSelectedTargetID   string `json:"im_selected_target_id"`
	MemoryStorageDir     string `json:"memory_storage_dir"`
}

type IMDeliveryConfig struct {
	Enabled           bool
	SelectedAccountID string
	SelectedTargetID  string
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

func (s *Store) DeliveryConfig() IMDeliveryConfig {
	snapshot := s.Snapshot()
	return IMDeliveryConfig{
		Enabled:           snapshot.IMDeliveryEnabled,
		SelectedAccountID: snapshot.IMSelectedAccountID,
		SelectedTargetID:  snapshot.IMSelectedTargetID,
	}
}

func (s *Store) MemoryStorageDir() string {
	dir := strings.TrimSpace(s.Snapshot().MemoryStorageDir)
	if dir == "" {
		return storage.DefaultMemoryStorageDir
	}
	return dir
}

func (s *Store) UpdateSessionWindowSeconds(seconds int) (Snapshot, error) {
	if err := ValidateSessionWindowSeconds(seconds); err != nil {
		return Snapshot{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := s.snapshot
	snapshot.SessionWindowSeconds = seconds
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

func (s *Store) UpdateIMDelivery(enabled bool, accountID string, targetID string) (Snapshot, error) {
	accountID = strings.TrimSpace(accountID)
	targetID = strings.TrimSpace(targetID)
	if err := ValidateIMDelivery(enabled, accountID, targetID); err != nil {
		return Snapshot{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	next := s.snapshot
	next.IMDeliveryEnabled = enabled
	next.IMSelectedAccountID = accountID
	next.IMSelectedTargetID = targetID

	if s.db == nil {
		s.snapshot = next
		return next, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return Snapshot{}, fmt.Errorf("begin update im delivery: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UnixMilli()
	updates := []struct {
		key   string
		value string
	}{
		{key: storage.IMDeliveryEnabledSettingKey, value: boolToSettingValue(enabled)},
		{key: storage.IMSelectedAccountSettingKey, value: accountID},
		{key: storage.IMSelectedTargetSettingKey, value: targetID},
	}
	for _, item := range updates {
		if _, err := tx.Exec(
			`UPDATE settings SET value = ?, updated_at = ? WHERE setting_key = ?`,
			item.value,
			now,
			item.key,
		); err != nil {
			return Snapshot{}, fmt.Errorf("update setting %q: %w", item.key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, fmt.Errorf("commit update im delivery: %w", err)
	}

	s.snapshot = next
	return next, nil
}

func (s *Store) UpdateMemoryStorageDir(dir string) (Snapshot, error) {
	dir = strings.TrimSpace(dir)
	if err := ValidateMemoryStorageDir(dir); err != nil {
		return Snapshot{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	next := s.snapshot
	next.MemoryStorageDir = dir

	if s.db == nil {
		s.snapshot = next
		return next, nil
	}

	if _, err := s.db.Exec(
		`UPDATE settings SET value = ?, updated_at = ? WHERE setting_key = ?`,
		dir,
		time.Now().UnixMilli(),
		storage.MemoryStorageDirSettingKey,
	); err != nil {
		return Snapshot{}, fmt.Errorf("update memory storage dir: %w", err)
	}

	s.snapshot = next
	return next, nil
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

func ValidateIMDelivery(enabled bool, accountID string, targetID string) error {
	if !enabled {
		return nil
	}
	if strings.TrimSpace(accountID) == "" {
		return fmt.Errorf("im selected account id is required when delivery is enabled")
	}
	if strings.TrimSpace(targetID) == "" {
		return fmt.Errorf("im selected target id is required when delivery is enabled")
	}
	return nil
}

func ValidateMemoryStorageDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("memory storage dir is required")
	}
	return nil
}

func (s *Store) load() (Snapshot, error) {
	if s.db == nil {
		return Snapshot{
			SessionWindowSeconds: storage.DefaultSessionWindowSeconds,
			IMDeliveryEnabled:    false,
			MemoryStorageDir:     storage.DefaultMemoryStorageDir,
		}, nil
	}

	rawSessionWindow, err := s.readSettingValue(storage.SessionWindowSecondsSettingKey)
	if err == sql.ErrNoRows {
		return s.repairDefault()
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("load session window seconds: %w", err)
	}
	sessionWindowSeconds, err := strconv.Atoi(strings.TrimSpace(rawSessionWindow))
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse session window seconds %q: %w", rawSessionWindow, err)
	}
	if err := ValidateSessionWindowSeconds(sessionWindowSeconds); err != nil {
		return Snapshot{}, err
	}

	rawIMEnabled, err := s.readSettingValue(storage.IMDeliveryEnabledSettingKey)
	if err == sql.ErrNoRows {
		return s.repairDefault()
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("load im delivery enabled: %w", err)
	}
	imDeliveryEnabled, err := parseBoolSetting(rawIMEnabled)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse im delivery enabled %q: %w", rawIMEnabled, err)
	}

	selectedAccountID, err := s.readSettingValue(storage.IMSelectedAccountSettingKey)
	if err == sql.ErrNoRows {
		return s.repairDefault()
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("load im selected account id: %w", err)
	}

	selectedTargetID, err := s.readSettingValue(storage.IMSelectedTargetSettingKey)
	if err == sql.ErrNoRows {
		return s.repairDefault()
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("load im selected target id: %w", err)
	}

	selectedAccountID = strings.TrimSpace(selectedAccountID)
	selectedTargetID = strings.TrimSpace(selectedTargetID)
	if err := ValidateIMDelivery(imDeliveryEnabled, selectedAccountID, selectedTargetID); err != nil {
		log.Printf("settings: invalid im delivery config found in database, falling back to disabled delivery: %v", err)
		imDeliveryEnabled = false
		selectedAccountID = ""
		selectedTargetID = ""
	}

	memoryStorageDir, err := s.readSettingValue(storage.MemoryStorageDirSettingKey)
	if err == sql.ErrNoRows {
		return s.repairDefault()
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("load memory storage dir: %w", err)
	}
	memoryStorageDir = strings.TrimSpace(memoryStorageDir)
	if err := ValidateMemoryStorageDir(memoryStorageDir); err != nil {
		log.Printf("settings: invalid memory storage dir found in database, falling back to default: %v", err)
		memoryStorageDir = storage.DefaultMemoryStorageDir
	}

	return Snapshot{
		SessionWindowSeconds: sessionWindowSeconds,
		IMDeliveryEnabled:    imDeliveryEnabled,
		IMSelectedAccountID:  selectedAccountID,
		IMSelectedTargetID:   selectedTargetID,
		MemoryStorageDir:     memoryStorageDir,
	}, nil
}

func (s *Store) repairDefault() (Snapshot, error) {
	snapshot := Snapshot{
		SessionWindowSeconds: storage.DefaultSessionWindowSeconds,
		IMDeliveryEnabled:    false,
		IMSelectedAccountID:  "",
		IMSelectedTargetID:   "",
		MemoryStorageDir:     storage.DefaultMemoryStorageDir,
	}
	values := []struct {
		key   string
		value string
	}{
		{key: storage.SessionWindowSecondsSettingKey, value: strconv.Itoa(snapshot.SessionWindowSeconds)},
		{key: storage.IMDeliveryEnabledSettingKey, value: boolToSettingValue(snapshot.IMDeliveryEnabled)},
		{key: storage.IMSelectedAccountSettingKey, value: snapshot.IMSelectedAccountID},
		{key: storage.IMSelectedTargetSettingKey, value: snapshot.IMSelectedTargetID},
		{key: storage.MemoryStorageDirSettingKey, value: snapshot.MemoryStorageDir},
	}
	for _, item := range values {
		if _, err := s.db.Exec(
			`INSERT INTO settings (setting_key, value, updated_at) VALUES (?, ?, ?)`,
			item.key,
			item.value,
			time.Now().UnixMilli(),
		); err != nil {
			return Snapshot{}, fmt.Errorf("insert default setting %q: %w", item.key, err)
		}
	}
	return snapshot, nil
}

func (s *Store) readSettingValue(key string) (string, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE setting_key = ?`, key).Scan(&raw)
	return raw, err
}

func boolToSettingValue(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func parseBoolSetting(value string) (bool, error) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off", "":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value")
	}
}
