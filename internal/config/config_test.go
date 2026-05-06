package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/config"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SOUL.md"), "# 角色\n你是一个有边界感的语音助手。")
	writeFile(t, filepath.Join(dir, "config.yaml"), `
soul_path: ./SOUL.md
database:
  dsn: user:pass@tcp(127.0.0.1:3306)/open_xiaoai_agent
openai:
  base_url: https://api.openai.com/v1
amap:
  api_key: amap-key
intent:
  model: gpt-4.1-mini
  base_url: https://intent.example.com/v1
  api_key: intent-key
reply:
  model: gpt-4.1
  base_url: https://reply.example.com/v1
  api_key: reply-key
`)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.OpenAI.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("openai.base_url = %q", cfg.OpenAI.BaseURL)
	}
	if cfg.SoulPath != filepath.Join(dir, "SOUL.md") {
		t.Fatalf("soul_path = %q", cfg.SoulPath)
	}
	if cfg.Database.DSN != "user:pass@tcp(127.0.0.1:3306)/open_xiaoai_agent" {
		t.Fatalf("database.dsn = %q", cfg.Database.DSN)
	}
	if cfg.IM.MediaCacheDir != filepath.Join(dir, ".cache", "im-media") {
		t.Fatalf("im.media_cache_dir = %q", cfg.IM.MediaCacheDir)
	}
	if cfg.AMap.APIKey != "amap-key" {
		t.Fatalf("amap.api_key = %q", cfg.AMap.APIKey)
	}
	if cfg.Intent.Model != "gpt-4.1-mini" {
		t.Fatalf("intent.model = %q", cfg.Intent.Model)
	}
	if cfg.Reply.Model != "gpt-4.1" {
		t.Fatalf("reply.model = %q", cfg.Reply.Model)
	}
	if cfg.Intent.BaseURL != "https://intent.example.com/v1" {
		t.Fatalf("intent.base_url = %q", cfg.Intent.BaseURL)
	}
	if cfg.Reply.BaseURL != "https://reply.example.com/v1" {
		t.Fatalf("reply.base_url = %q", cfg.Reply.BaseURL)
	}
	if !strings.Contains(cfg.Soul, "语音助手") {
		t.Fatalf("soul = %q, want loaded soul content", cfg.Soul)
	}
}

func TestLoad_AllowsEmptyAMapKey(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SOUL.md"), "# 角色\n你是一个有边界感的语音助手。")
	writeFile(t, filepath.Join(dir, "config.yaml"), `
soul_path: ./SOUL.md
database:
  dsn: user:pass@tcp(127.0.0.1:3306)/open_xiaoai_agent
openai:
  base_url: https://api.openai.com/v1
amap:
  api_key: ""
intent:
  model: gpt-4.1-mini
  api_key: intent-key
reply:
  model: gpt-4.1
  api_key: reply-key
`)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AMap.APIKey != "" {
		t.Fatalf("amap.api_key = %q, want empty", cfg.AMap.APIKey)
	}
}

func TestLoad_DefaultsModelBaseURLFromOpenAI(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SOUL.md"), "# 角色\n你是一个有边界感的语音助手。")
	writeFile(t, filepath.Join(dir, "config.yaml"), `
soul_path: ./SOUL.md
database:
  dsn: user:pass@tcp(127.0.0.1:3306)/open_xiaoai_agent
openai:
  base_url: https://api.openai.com/v1
intent:
  model: gpt-4.1-mini
  api_key: intent-key
reply:
  model: gpt-4.1
  api_key: reply-key
`)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Intent.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("intent.base_url = %q", cfg.Intent.BaseURL)
	}
	if cfg.Reply.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("reply.base_url = %q", cfg.Reply.BaseURL)
	}
}

func TestLoad_TrimsDatabaseDSN(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SOUL.md"), "# 角色\n你是一个有边界感的语音助手。")
	writeFile(t, filepath.Join(dir, "config.yaml"), `
soul_path: ./SOUL.md
database:
  dsn: "  user:pass@tcp(127.0.0.1:3306)/open_xiaoai_agent  "
openai:
  base_url: https://api.openai.com/v1
intent:
  model: gpt-4.1-mini
  api_key: intent-key
reply:
  model: gpt-4.1
  api_key: reply-key
`)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Database.DSN != "user:pass@tcp(127.0.0.1:3306)/open_xiaoai_agent" {
		t.Fatalf("database.dsn = %q", cfg.Database.DSN)
	}
}

func TestLoad_UsesCustomMediaCacheDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SOUL.md"), "# 角色\n你是一个有边界感的语音助手。")
	writeFile(t, filepath.Join(dir, "config.yaml"), `
soul_path: ./SOUL.md
database:
  dsn: user:pass@tcp(127.0.0.1:3306)/open_xiaoai_agent
im:
  media_cache_dir: data/im-cache
openai:
  base_url: https://api.openai.com/v1
intent:
  model: gpt-4.1-mini
  api_key: intent-key
reply:
  model: gpt-4.1
  api_key: reply-key
`)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.IM.MediaCacheDir != filepath.Join(dir, "data", "im-cache") {
		t.Fatalf("im.media_cache_dir = %q", cfg.IM.MediaCacheDir)
	}
}

func TestLoad_UsesRelativeSoulPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "prompts", "assistant-soul.md"), "# 角色\n你是一个会记住上下文的语音助手。")
	writeFile(t, filepath.Join(dir, "config.yaml"), `
soul_path: prompts/assistant-soul.md
database:
  dsn: user:pass@tcp(127.0.0.1:3306)/open_xiaoai_agent
openai:
  base_url: https://api.openai.com/v1
intent:
  model: gpt-4.1-mini
  api_key: intent-key
reply:
  model: gpt-4.1
  api_key: reply-key
`)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.SoulPath != filepath.Join(dir, "prompts", "assistant-soul.md") {
		t.Fatalf("soul_path = %q", cfg.SoulPath)
	}
	if !strings.Contains(cfg.Soul, "记住上下文") {
		t.Fatalf("soul = %q, want custom relative soul content", cfg.Soul)
	}
}

func TestLoad_UsesAbsoluteSoulPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	externalSoulDir := t.TempDir()
	externalSoulPath := filepath.Join(externalSoulDir, "my-soul.md")
	writeFile(t, externalSoulPath, "# 角色\n你是一个语气温和的语音助手。")
	writeFile(t, filepath.Join(dir, "config.yaml"), `
soul_path: `+externalSoulPath+`
database:
  dsn: user:pass@tcp(127.0.0.1:3306)/open_xiaoai_agent
openai:
  base_url: https://api.openai.com/v1
intent:
  model: gpt-4.1-mini
  api_key: intent-key
reply:
  model: gpt-4.1
  api_key: reply-key
`)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.SoulPath != externalSoulPath {
		t.Fatalf("soul_path = %q", cfg.SoulPath)
	}
	if !strings.Contains(cfg.Soul, "语气温和") {
		t.Fatalf("soul = %q, want custom absolute soul content", cfg.Soul)
	}
}

func TestLoad_RejectsEmptySoul(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SOUL.md"), "   \n")
	writeFile(t, filepath.Join(dir, "config.yaml"), `
soul_path: ./SOUL.md
database:
  dsn: user:pass@tcp(127.0.0.1:3306)/open_xiaoai_agent
openai:
  base_url: https://api.openai.com/v1
intent:
  model: gpt-4.1-mini
  api_key: intent-key
reply:
  model: gpt-4.1
  api_key: reply-key
`)

	_, err := config.Load(dir)
	if err == nil {
		t.Fatal("Load() error = nil, want non-nil")
	}
}

func TestLoad_RejectsMissingSoulPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SOUL.md"), "# 角色\n你是一个有边界感的语音助手。")
	writeFile(t, filepath.Join(dir, "config.yaml"), `
database:
  dsn: user:pass@tcp(127.0.0.1:3306)/open_xiaoai_agent
openai:
  base_url: https://api.openai.com/v1
intent:
  model: gpt-4.1-mini
  api_key: intent-key
reply:
  model: gpt-4.1
  api_key: reply-key
`)

	_, err := config.Load(dir)
	if err == nil {
		t.Fatal("Load() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "soul_path is required") {
		t.Fatalf("Load() error = %v, want soul_path is required", err)
	}
}

func TestLoad_RejectsMissingDatabaseDSN(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SOUL.md"), "# 角色\n你是一个有边界感的语音助手。")
	writeFile(t, filepath.Join(dir, "config.yaml"), `
soul_path: ./SOUL.md
openai:
  base_url: https://api.openai.com/v1
intent:
  model: gpt-4.1-mini
  api_key: intent-key
reply:
  model: gpt-4.1
  api_key: reply-key
`)

	_, err := config.Load(dir)
	if err == nil {
		t.Fatal("Load() error = nil, want non-nil")
	}
}

func TestLoad_RejectsMissingModelConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SOUL.md"), "# 角色\n你是一个有边界感的语音助手。")
	writeFile(t, filepath.Join(dir, "config.yaml"), `
soul_path: ./SOUL.md
database:
  dsn: user:pass@tcp(127.0.0.1:3306)/open_xiaoai_agent
openai:
  base_url: https://api.openai.com/v1
intent:
  model: gpt-4.1-mini
  api_key: intent-key
reply:
  model: ""
  api_key: reply-key
`)

	_, err := config.Load(dir)
	if err == nil {
		t.Fatal("Load() error = nil, want non-nil")
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
