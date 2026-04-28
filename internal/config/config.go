package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ModelConfig struct {
	Model   string `yaml:"model"`
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
}

type FileConfig struct {
	Database struct {
		DSN string `yaml:"dsn"`
	} `yaml:"database"`
	IM struct {
		MediaCacheDir string `yaml:"media_cache_dir"`
	} `yaml:"im"`
	OpenAI struct {
		BaseURL string `yaml:"base_url"`
	} `yaml:"openai"`
	AMap struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"amap"`
	Intent ModelConfig `yaml:"intent"`
	Reply  ModelConfig `yaml:"reply"`
}

type AppConfig struct {
	FileConfig
	Soul string
}

func Load(rootDir string) (*AppConfig, error) {
	configPath := filepath.Join(rootDir, "config.yaml")
	soulPath := filepath.Join(rootDir, "SOUL.md")

	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config.yaml: %w", err)
	}

	soulBytes, err := os.ReadFile(soulPath)
	if err != nil {
		return nil, fmt.Errorf("read SOUL.md: %w", err)
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(configBytes, &cfg.FileConfig); err != nil {
		return nil, fmt.Errorf("decode config.yaml: %w", err)
	}

	cfg.Soul = strings.TrimSpace(string(soulBytes))
	if cfg.Soul == "" {
		return nil, fmt.Errorf("SOUL.md is empty")
	}
	if err := cfg.normalize(rootDir); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *AppConfig) normalize(rootDir string) error {
	c.Database.DSN = strings.TrimSpace(c.Database.DSN)
	if c.Database.DSN == "" {
		return fmt.Errorf("database.dsn is required")
	}
	c.IM.MediaCacheDir = strings.TrimSpace(c.IM.MediaCacheDir)
	if c.IM.MediaCacheDir == "" {
		c.IM.MediaCacheDir = filepath.Join(rootDir, ".cache", "im-media")
	} else if !filepath.IsAbs(c.IM.MediaCacheDir) {
		c.IM.MediaCacheDir = filepath.Join(rootDir, c.IM.MediaCacheDir)
	}
	absMediaCacheDir, err := filepath.Abs(c.IM.MediaCacheDir)
	if err != nil {
		return fmt.Errorf("resolve im.media_cache_dir: %w", err)
	}
	c.IM.MediaCacheDir = absMediaCacheDir
	defaultBaseURL := strings.TrimRight(strings.TrimSpace(c.OpenAI.BaseURL), "/")
	if defaultBaseURL == "" {
		defaultBaseURL = "https://api.openai.com/v1"
		c.OpenAI.BaseURL = defaultBaseURL
	}

	if err := normalizeModelConfig("intent", &c.Intent, defaultBaseURL); err != nil {
		return err
	}
	if err := normalizeModelConfig("reply", &c.Reply, defaultBaseURL); err != nil {
		return err
	}

	return nil
}

func normalizeModelConfig(name string, cfg *ModelConfig, defaultBaseURL string) error {
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)

	if cfg.Model == "" {
		return fmt.Errorf("%s.model is required", name)
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.BaseURL == "" {
		return fmt.Errorf("%s.base_url is required", name)
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("%s.api_key is required", name)
	}

	return nil
}
