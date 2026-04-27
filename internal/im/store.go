package im

import (
	"database/sql"
	"fmt"
	"strings"
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

func (s *Store) Snapshot(eventLimit int) (Snapshot, error) {
	accounts, err := s.ListAccounts()
	if err != nil {
		return Snapshot{}, err
	}
	targets, err := s.ListTargets()
	if err != nil {
		return Snapshot{}, err
	}
	events, err := s.ListEvents(eventLimit)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		Accounts: accounts,
		Targets:  targets,
		Events:   events,
	}, nil
}

func (s *Store) ListAccounts() ([]Account, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}

	rows, err := s.db.Query(`
		SELECT id, platform, remote_account_id, owner_user_id, display_name, base_url, token, last_error, last_sent_at, created_at, updated_at
		FROM im_accounts
		ORDER BY updated_at DESC, created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query im accounts: %w", err)
	}
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		account, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate im accounts: %w", err)
	}
	return accounts, nil
}

func (s *Store) ListTargets() ([]Target, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}

	rows, err := s.db.Query(`
		SELECT id, account_id, name, target_user_id, is_default, created_at, updated_at
		FROM im_targets
		ORDER BY is_default DESC, updated_at DESC, created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query im targets: %w", err)
	}
	defer rows.Close()

	var targets []Target
	for rows.Next() {
		target, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate im targets: %w", err)
	}
	return targets, nil
}

func (s *Store) ListEvents(limit int) ([]Event, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.Query(`
		SELECT id, account_id, type, message, created_at
		FROM im_events
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query im events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		var createdAt int64
		if err := rows.Scan(&event.ID, &event.AccountID, &event.Type, &event.Message, &createdAt); err != nil {
			return nil, fmt.Errorf("scan im event: %w", err)
		}
		event.CreatedAt = storage.TimeFromUnixMillis(createdAt)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate im events: %w", err)
	}
	return events, nil
}

func (s *Store) GetAccount(id string) (Account, bool, error) {
	if s == nil || s.db == nil {
		return Account{}, false, nil
	}

	row := s.db.QueryRow(`
		SELECT id, platform, remote_account_id, owner_user_id, display_name, base_url, token, last_error, last_sent_at, created_at, updated_at
		FROM im_accounts
		WHERE id = ?
	`, strings.TrimSpace(id))
	account, err := scanAccount(row)
	switch {
	case err == sql.ErrNoRows:
		return Account{}, false, nil
	case err != nil:
		return Account{}, false, err
	default:
		return account, true, nil
	}
}

func (s *Store) GetTarget(id string) (Target, bool, error) {
	if s == nil || s.db == nil {
		return Target{}, false, nil
	}

	row := s.db.QueryRow(`
		SELECT id, account_id, name, target_user_id, is_default, created_at, updated_at
		FROM im_targets
		WHERE id = ?
	`, strings.TrimSpace(id))
	target, err := scanTarget(row)
	switch {
	case err == sql.ErrNoRows:
		return Target{}, false, nil
	case err != nil:
		return Target{}, false, err
	default:
		return target, true, nil
	}
}

func (s *Store) UpsertAccount(platform string, remoteAccountID string, ownerUserID string, displayName string, baseURL string, token string) (Account, error) {
	if s == nil || s.db == nil {
		return Account{}, fmt.Errorf("im account store is not configured")
	}

	platform = strings.TrimSpace(platform)
	remoteAccountID = strings.TrimSpace(remoteAccountID)
	ownerUserID = strings.TrimSpace(ownerUserID)
	displayName = strings.TrimSpace(displayName)
	baseURL = strings.TrimSpace(baseURL)
	token = strings.TrimSpace(token)

	if platform == "" {
		return Account{}, fmt.Errorf("im platform is required")
	}
	if remoteAccountID == "" {
		return Account{}, fmt.Errorf("im remote account id is required")
	}
	if displayName == "" {
		displayName = remoteAccountID
	}

	tx, err := s.db.Begin()
	if err != nil {
		return Account{}, fmt.Errorf("begin upsert im account: %w", err)
	}
	defer tx.Rollback()

	var account Account
	row := tx.QueryRow(`
		SELECT id, platform, remote_account_id, owner_user_id, display_name, base_url, token, last_error, last_sent_at, created_at, updated_at
		FROM im_accounts
		WHERE platform = ? AND remote_account_id = ?
	`, platform, remoteAccountID)
	account, err = scanAccount(row)
	now := time.Now()
	switch {
	case err == sql.ErrNoRows:
		account = Account{
			ID:              s.nextID("imacct"),
			Platform:        platform,
			RemoteAccountID: remoteAccountID,
			OwnerUserID:     ownerUserID,
			DisplayName:     displayName,
			BaseURL:         baseURL,
			Token:           token,
			LastError:       "",
			LastSentAt:      time.Time{},
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if _, err := tx.Exec(`
			INSERT INTO im_accounts (id, platform, remote_account_id, owner_user_id, display_name, base_url, token, last_error, last_sent_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			account.ID,
			account.Platform,
			account.RemoteAccountID,
			account.OwnerUserID,
			account.DisplayName,
			account.BaseURL,
			account.Token,
			account.LastError,
			storage.UnixMillis(account.LastSentAt),
			storage.UnixMillis(account.CreatedAt),
			storage.UnixMillis(account.UpdatedAt),
		); err != nil {
			return Account{}, fmt.Errorf("insert im account: %w", err)
		}
	case err != nil:
		return Account{}, fmt.Errorf("query existing im account: %w", err)
	default:
		account.OwnerUserID = ownerUserID
		account.DisplayName = displayName
		account.BaseURL = baseURL
		account.Token = token
		account.LastError = ""
		account.UpdatedAt = now
		if _, err := tx.Exec(`
			UPDATE im_accounts
			SET owner_user_id = ?, display_name = ?, base_url = ?, token = ?, last_error = ?, updated_at = ?
			WHERE id = ?
		`,
			account.OwnerUserID,
			account.DisplayName,
			account.BaseURL,
			account.Token,
			account.LastError,
			storage.UnixMillis(account.UpdatedAt),
			account.ID,
		); err != nil {
			return Account{}, fmt.Errorf("update im account: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Account{}, fmt.Errorf("commit upsert im account: %w", err)
	}
	return account, nil
}

func (s *Store) UpsertTarget(accountID string, name string, targetUserID string, setDefault bool) (Target, error) {
	if s == nil || s.db == nil {
		return Target{}, fmt.Errorf("im target store is not configured")
	}

	accountID = strings.TrimSpace(accountID)
	name = strings.TrimSpace(name)
	targetUserID = strings.TrimSpace(targetUserID)
	if accountID == "" {
		return Target{}, fmt.Errorf("im target account id is required")
	}
	if targetUserID == "" {
		return Target{}, fmt.Errorf("im target user id is required")
	}
	if name == "" {
		name = targetUserID
	}

	tx, err := s.db.Begin()
	if err != nil {
		return Target{}, fmt.Errorf("begin upsert im target: %w", err)
	}
	defer tx.Rollback()

	var target Target
	row := tx.QueryRow(`
		SELECT id, account_id, name, target_user_id, is_default, created_at, updated_at
		FROM im_targets
		WHERE account_id = ? AND target_user_id = ?
	`, accountID, targetUserID)
	target, err = scanTarget(row)
	now := time.Now()
	switch {
	case err == sql.ErrNoRows:
		target = Target{
			ID:           s.nextID("imtarget"),
			AccountID:    accountID,
			Name:         name,
			TargetUserID: targetUserID,
			IsDefault:    setDefault,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if _, err := tx.Exec(`
			INSERT INTO im_targets (id, account_id, name, target_user_id, is_default, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`,
			target.ID,
			target.AccountID,
			target.Name,
			target.TargetUserID,
			target.IsDefault,
			storage.UnixMillis(target.CreatedAt),
			storage.UnixMillis(target.UpdatedAt),
		); err != nil {
			return Target{}, fmt.Errorf("insert im target: %w", err)
		}
	case err != nil:
		return Target{}, fmt.Errorf("query existing im target: %w", err)
	default:
		target.Name = name
		target.UpdatedAt = now
		target.IsDefault = target.IsDefault || setDefault
		if _, err := tx.Exec(`
			UPDATE im_targets
			SET name = ?, is_default = ?, updated_at = ?
			WHERE id = ?
		`,
			target.Name,
			target.IsDefault,
			storage.UnixMillis(target.UpdatedAt),
			target.ID,
		); err != nil {
			return Target{}, fmt.Errorf("update im target: %w", err)
		}
	}

	if target.IsDefault {
		if err := clearOtherDefaultTargets(tx, target.AccountID, target.ID); err != nil {
			return Target{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return Target{}, fmt.Errorf("commit upsert im target: %w", err)
	}
	return target, nil
}

func (s *Store) EnsureOwnerTarget(accountID string, ownerUserID string) (Target, error) {
	if strings.TrimSpace(ownerUserID) == "" {
		return Target{}, nil
	}

	hasDefault, err := s.hasDefaultTarget(accountID)
	if err != nil {
		return Target{}, err
	}
	return s.UpsertTarget(accountID, "扫码用户", ownerUserID, !hasDefault)
}

func (s *Store) SetDefaultTarget(accountID string, targetID string) error {
	if s == nil || s.db == nil {
		return nil
	}

	accountID = strings.TrimSpace(accountID)
	targetID = strings.TrimSpace(targetID)
	if accountID == "" || targetID == "" {
		return fmt.Errorf("account id and target id are required")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin set default im target: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE im_targets SET is_default = FALSE, updated_at = ? WHERE account_id = ?`, time.Now().UnixMilli(), accountID); err != nil {
		return fmt.Errorf("clear default im target: %w", err)
	}
	if _, err := tx.Exec(`UPDATE im_targets SET is_default = TRUE, updated_at = ? WHERE id = ? AND account_id = ?`, time.Now().UnixMilli(), targetID, accountID); err != nil {
		return fmt.Errorf("set default im target: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set default im target: %w", err)
	}
	return nil
}

func (s *Store) DeleteTarget(id string) error {
	if s == nil || s.db == nil {
		return nil
	}
	if _, err := s.db.Exec(`DELETE FROM im_targets WHERE id = ?`, strings.TrimSpace(id)); err != nil {
		return fmt.Errorf("delete im target: %w", err)
	}
	return nil
}

func (s *Store) DeleteAccount(id string) error {
	if s == nil || s.db == nil {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin delete im account: %w", err)
	}
	defer tx.Rollback()

	accountID := strings.TrimSpace(id)
	if _, err := tx.Exec(`DELETE FROM im_events WHERE account_id = ?`, accountID); err != nil {
		return fmt.Errorf("delete im events: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM im_targets WHERE account_id = ?`, accountID); err != nil {
		return fmt.Errorf("delete im targets: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM im_accounts WHERE id = ?`, accountID); err != nil {
		return fmt.Errorf("delete im account: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete im account: %w", err)
	}
	return nil
}

func (s *Store) AppendEvent(accountID string, eventType string, message string) error {
	if s == nil || s.db == nil {
		return nil
	}
	if _, err := s.db.Exec(`
		INSERT INTO im_events (id, account_id, type, message, created_at)
		VALUES (?, ?, ?, ?, ?)
	`,
		s.nextID("imevent"),
		strings.TrimSpace(accountID),
		strings.TrimSpace(eventType),
		strings.TrimSpace(message),
		time.Now().UnixMilli(),
	); err != nil {
		return fmt.Errorf("insert im event: %w", err)
	}
	return nil
}

func (s *Store) MarkDeliverySuccess(accountID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	now := time.Now().UnixMilli()
	if _, err := s.db.Exec(`
		UPDATE im_accounts
		SET last_error = '', last_sent_at = ?, updated_at = ?
		WHERE id = ?
	`, now, now, strings.TrimSpace(accountID)); err != nil {
		return fmt.Errorf("update im account delivery success: %w", err)
	}
	return nil
}

func (s *Store) MarkDeliveryFailure(accountID string, message string) error {
	if s == nil || s.db == nil {
		return nil
	}
	if _, err := s.db.Exec(`
		UPDATE im_accounts
		SET last_error = ?, updated_at = ?
		WHERE id = ?
	`, strings.TrimSpace(message), time.Now().UnixMilli(), strings.TrimSpace(accountID)); err != nil {
		return fmt.Errorf("update im account delivery failure: %w", err)
	}
	return nil
}

func (s *Store) Reset() error {
	if s == nil || s.db == nil {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin reset im state: %w", err)
	}
	defer tx.Rollback()

	for _, query := range []string{
		`DELETE FROM im_events`,
		`DELETE FROM im_targets`,
		`DELETE FROM im_accounts`,
	} {
		if _, err := tx.Exec(query); err != nil {
			return fmt.Errorf("reset im state: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reset im state: %w", err)
	}
	return nil
}

func (s *Store) hasDefaultTarget(accountID string) (bool, error) {
	if s == nil || s.db == nil {
		return false, nil
	}

	var exists int
	err := s.db.QueryRow(`SELECT 1 FROM im_targets WHERE account_id = ? AND is_default = TRUE LIMIT 1`, strings.TrimSpace(accountID)).Scan(&exists)
	switch {
	case err == sql.ErrNoRows:
		return false, nil
	case err != nil:
		return false, fmt.Errorf("query default im target: %w", err)
	default:
		return true, nil
	}
}

func clearOtherDefaultTargets(tx *sql.Tx, accountID string, currentTargetID string) error {
	if _, err := tx.Exec(`
		UPDATE im_targets
		SET is_default = FALSE, updated_at = ?
		WHERE account_id = ? AND id <> ?
	`, time.Now().UnixMilli(), accountID, currentTargetID); err != nil {
		return fmt.Errorf("clear other default im targets: %w", err)
	}
	return nil
}

func scanAccount(scanner interface {
	Scan(dest ...any) error
}) (Account, error) {
	var account Account
	var lastSentAt, createdAt, updatedAt int64
	if err := scanner.Scan(
		&account.ID,
		&account.Platform,
		&account.RemoteAccountID,
		&account.OwnerUserID,
		&account.DisplayName,
		&account.BaseURL,
		&account.Token,
		&account.LastError,
		&lastSentAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return Account{}, err
	}
	account.LastSentAt = storage.TimeFromUnixMillis(lastSentAt)
	account.CreatedAt = storage.TimeFromUnixMillis(createdAt)
	account.UpdatedAt = storage.TimeFromUnixMillis(updatedAt)
	return account, nil
}

func scanTarget(scanner interface {
	Scan(dest ...any) error
}) (Target, error) {
	var target Target
	var isDefault bool
	var createdAt, updatedAt int64
	if err := scanner.Scan(
		&target.ID,
		&target.AccountID,
		&target.Name,
		&target.TargetUserID,
		&isDefault,
		&createdAt,
		&updatedAt,
	); err != nil {
		return Target{}, err
	}
	target.IsDefault = isDefault
	target.CreatedAt = storage.TimeFromUnixMillis(createdAt)
	target.UpdatedAt = storage.TimeFromUnixMillis(updatedAt)
	return target, nil
}

func (s *Store) nextID(prefix string) string {
	value := atomic.AddUint64(&s.seq, 1)
	return fmt.Sprintf("%s_%d", prefix, value)
}
