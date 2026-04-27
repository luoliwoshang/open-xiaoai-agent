package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	mysqlcfg "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

const (
	SessionWindowSecondsSettingKey = "session.window_seconds"
	DefaultSessionWindowSeconds    = 300
	IMDeliveryEnabledSettingKey    = "im.delivery.enabled"
	IMSelectedAccountSettingKey    = "im.delivery.selected_account_id"
	IMSelectedTargetSettingKey     = "im.delivery.selected_target_id"
)

func OpenRuntimeDB(dsn string) (*sql.DB, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, nil
	}
	if strings.HasPrefix(dsn, "sqlite://") {
		return openSQLite(strings.TrimPrefix(dsn, "sqlite://"))
	}
	if strings.HasPrefix(dsn, "sqlite:") {
		return openSQLite(strings.TrimPrefix(dsn, "sqlite:"))
	}
	return openMySQL(dsn)
}

func UnixMillis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func TimeFromUnixMillis(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.UnixMilli(value)
}

func openMySQL(rawDSN string) (*sql.DB, error) {
	cfg, err := mysqlcfg.ParseDSN(rawDSN)
	if err != nil {
		return nil, fmt.Errorf("parse mysql dsn: %w", err)
	}
	if strings.TrimSpace(cfg.DBName) == "" {
		return nil, fmt.Errorf("mysql dsn must include database name")
	}
	if cfg.Net == "" {
		cfg.Net = "tcp"
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:3306"
	}
	cfg.ParseTime = true
	cfg.Collation = "utf8mb4_unicode_ci"
	if cfg.Params == nil {
		cfg.Params = make(map[string]string)
	}
	if _, ok := cfg.Params["charset"]; !ok {
		cfg.Params["charset"] = "utf8mb4"
	}
	if err := ensureMySQLDatabase(cfg); err != nil {
		return nil, err
	}

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	if err := ensureSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func openSQLite(path string) (*sql.DB, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite dir: %w", err)
	}

	dsn := fmt.Sprintf("%s?_busy_timeout=5000&_foreign_keys=on&_journal_mode=WAL&_synchronous=NORMAL", path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := ensureSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func ensureSchema(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS tasks (
			id VARCHAR(255) PRIMARY KEY,
			plugin VARCHAR(255) NOT NULL,
			kind VARCHAR(255) NOT NULL,
			title VARCHAR(512) NOT NULL,
			input LONGTEXT NOT NULL,
			parent_task_id VARCHAR(255) NOT NULL DEFAULT '',
			state VARCHAR(64) NOT NULL,
			summary LONGTEXT NOT NULL,
			result LONGTEXT NOT NULL,
			report_pending BOOLEAN NOT NULL DEFAULT FALSE,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS task_events (
			id VARCHAR(255) PRIMARY KEY,
			task_id VARCHAR(255) NOT NULL,
			type VARCHAR(255) NOT NULL,
			message LONGTEXT NOT NULL,
			created_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS conversations (
			id VARCHAR(255) PRIMARY KEY,
			started_at BIGINT NOT NULL,
			last_active BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS conversation_messages (
			conversation_id VARCHAR(255) NOT NULL,
			message_index BIGINT NOT NULL,
			role VARCHAR(64) NOT NULL,
			content LONGTEXT NOT NULL,
			PRIMARY KEY (conversation_id, message_index)
		)`,
		`CREATE TABLE IF NOT EXISTS claude_records (
			task_id VARCHAR(255) PRIMARY KEY,
			session_id VARCHAR(255) NOT NULL DEFAULT '',
			prompt LONGTEXT NOT NULL,
			working_directory LONGTEXT NOT NULL,
			status VARCHAR(64) NOT NULL,
			last_summary LONGTEXT NOT NULL,
			last_assistant_text LONGTEXT NOT NULL,
			result LONGTEXT NOT NULL,
			error LONGTEXT NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			setting_key VARCHAR(255) PRIMARY KEY,
			value VARCHAR(255) NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS im_accounts (
			id VARCHAR(255) PRIMARY KEY,
			platform VARCHAR(64) NOT NULL,
			remote_account_id VARCHAR(255) NOT NULL,
			owner_user_id VARCHAR(255) NOT NULL DEFAULT '',
			display_name VARCHAR(255) NOT NULL DEFAULT '',
			base_url LONGTEXT NOT NULL,
			token LONGTEXT NOT NULL,
			last_error LONGTEXT NOT NULL,
			last_sent_at BIGINT NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			UNIQUE (platform, remote_account_id)
		)`,
		`CREATE TABLE IF NOT EXISTS im_targets (
			id VARCHAR(255) PRIMARY KEY,
			account_id VARCHAR(255) NOT NULL,
			name VARCHAR(255) NOT NULL,
			target_user_id VARCHAR(255) NOT NULL,
			is_default BOOLEAN NOT NULL DEFAULT FALSE,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			UNIQUE (account_id, target_user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS im_events (
			id VARCHAR(255) PRIMARY KEY,
			account_id VARCHAR(255) NOT NULL,
			type VARCHAR(255) NOT NULL,
			message LONGTEXT NOT NULL,
			created_at BIGINT NOT NULL
		)`,
	}

	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("ensure runtime schema: %w", err)
		}
	}
	if err := ensureDefaultSettings(db); err != nil {
		return err
	}
	return nil
}

func ensureMySQLDatabase(cfg *mysqlcfg.Config) error {
	adminCfg := *cfg
	dbName := strings.TrimSpace(adminCfg.DBName)
	adminCfg.DBName = ""

	adminDB, err := sql.Open("mysql", adminCfg.FormatDSN())
	if err != nil {
		return fmt.Errorf("open mysql admin connection: %w", err)
	}
	defer adminDB.Close()

	adminDB.SetConnMaxLifetime(5 * time.Minute)
	adminDB.SetMaxOpenConns(2)
	adminDB.SetMaxIdleConns(2)

	if err := adminDB.Ping(); err != nil {
		return fmt.Errorf("ping mysql admin connection: %w", err)
	}

	query := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
		quoteMySQLIdentifier(dbName),
	)
	if _, err := adminDB.Exec(query); err != nil {
		return fmt.Errorf("ensure mysql database %q: %w", dbName, err)
	}
	return nil
}

func ensureDefaultSettings(db *sql.DB) error {
	defaults := map[string]string{
		SessionWindowSecondsSettingKey: strconv.Itoa(DefaultSessionWindowSeconds),
		IMDeliveryEnabledSettingKey:    "0",
		IMSelectedAccountSettingKey:    "",
		IMSelectedTargetSettingKey:     "",
	}

	for key, value := range defaults {
		var exists int
		err := db.QueryRow(`SELECT 1 FROM settings WHERE setting_key = ? LIMIT 1`, key).Scan(&exists)
		switch {
		case err == nil:
			continue
		case err != sql.ErrNoRows:
			return fmt.Errorf("query setting %q: %w", key, err)
		}

		if _, err := db.Exec(
			`INSERT INTO settings (setting_key, value, updated_at) VALUES (?, ?, ?)`,
			key,
			value,
			time.Now().UnixMilli(),
		); err != nil {
			return fmt.Errorf("insert default setting %q: %w", key, err)
		}
	}
	return nil
}

func quoteMySQLIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}
