package testmysql

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	mysqlcfg "github.com/go-sql-driver/mysql"
)

const (
	envKey     = "OPEN_XIAOAI_AGENT_TEST_MYSQL_DSN"
	defaultDSN = "root:root@tcp(127.0.0.1:3306)/open_xiaoai_agent_test"
)

func NewDSN(tb testing.TB) string {
	tb.Helper()

	baseDSN, explicit := lookupBaseDSN()
	cfg, err := mysqlcfg.ParseDSN(baseDSN)
	if err != nil {
		tb.Fatalf("parse test mysql dsn: %v", err)
	}
	if strings.TrimSpace(cfg.DBName) == "" {
		tb.Fatal("test mysql dsn must include database name")
	}

	if err := pingAdmin(cfg); err != nil {
		if explicit {
			tb.Fatalf("connect test mysql: %v", err)
		}
		tb.Skipf("skip mysql-backed test because local mysql is unavailable: %v", err)
	}

	cfg.DBName = uniqueDatabaseName(cfg.DBName, tb.Name())
	return cfg.FormatDSN()
}

func lookupBaseDSN() (string, bool) {
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw != "" {
		return raw, true
	}
	return defaultDSN, false
}

func pingAdmin(cfg *mysqlcfg.Config) error {
	adminCfg := *cfg
	adminCfg.DBName = ""
	if adminCfg.Net == "" {
		adminCfg.Net = "tcp"
	}
	if adminCfg.Addr == "" {
		adminCfg.Addr = "127.0.0.1:3306"
	}

	db, err := sql.Open("mysql", adminCfg.FormatDSN())
	if err != nil {
		return err
	}
	defer db.Close()

	db.SetConnMaxLifetime(5 * time.Second)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db.Ping()
}

func uniqueDatabaseName(prefix string, testName string) string {
	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	sanitized := sanitizeName(testName)
	if len(sanitized) > 24 {
		sanitized = sanitized[:24]
	}
	name := fmt.Sprintf("%s_%s_%s", prefix, sanitized, suffix)
	if len(name) > 64 {
		name = name[:64]
	}
	return strings.TrimRight(name, "_")
}

func sanitizeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "test"
	}

	var b strings.Builder
	b.Grow(len(value))
	lastUnderscore := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastUnderscore = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if lastUnderscore {
				continue
			}
			b.WriteByte('_')
			lastUnderscore = true
		}
	}

	result := strings.Trim(b.String(), "_")
	if result == "" {
		return "test"
	}
	return result
}
