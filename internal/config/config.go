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
	SoulPath string `yaml:"soul_path"`
	Database struct {
		DSN string `yaml:"dsn"`
	} `yaml:"database"`
	Task struct {
		ArtifactCacheDir string `yaml:"artifact_cache_dir"`
	} `yaml:"task"`
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

	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config.yaml: %w", err)
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(configBytes, &cfg.FileConfig); err != nil {
		return nil, fmt.Errorf("decode config.yaml: %w", err)
	}
	if err := cfg.normalize(rootDir); err != nil {
		return nil, err
	}

	soulBytes, err := os.ReadFile(cfg.SoulPath)
	if err != nil {
		return nil, fmt.Errorf("read soul_path %q: %w", cfg.SoulPath, err)
	}

	cfg.Soul = strings.TrimSpace(string(soulBytes))
	if cfg.Soul == "" {
		return nil, fmt.Errorf("soul file %q is empty", cfg.SoulPath)
	}

	return &cfg, nil
}

func (c *AppConfig) normalize(rootDir string) error {
	c.SoulPath = strings.TrimSpace(c.SoulPath)
	if c.SoulPath == "" {
		return fmt.Errorf("soul_path is required")
	}
	if !filepath.IsAbs(c.SoulPath) {
		c.SoulPath = filepath.Join(rootDir, c.SoulPath)
	}
	absSoulPath, err := filepath.Abs(c.SoulPath)
	if err != nil {
		return fmt.Errorf("resolve soul_path: %w", err)
	}
	c.SoulPath = absSoulPath

	c.Database.DSN = strings.TrimSpace(c.Database.DSN)
	if c.Database.DSN == "" {
		return fmt.Errorf("database.dsn is required")
	}
	c.Task.ArtifactCacheDir = strings.TrimSpace(c.Task.ArtifactCacheDir)
	if c.Task.ArtifactCacheDir == "" {
		c.Task.ArtifactCacheDir = filepath.Join(rootDir, ".cache", "task-artifacts")
	} else if !filepath.IsAbs(c.Task.ArtifactCacheDir) {
		c.Task.ArtifactCacheDir = filepath.Join(rootDir, c.Task.ArtifactCacheDir)
	}
	absTaskArtifactCacheDir, err := filepath.Abs(c.Task.ArtifactCacheDir)
	if err != nil {
		return fmt.Errorf("resolve task.artifact_cache_dir: %w", err)
	}
	c.Task.ArtifactCacheDir = absTaskArtifactCacheDir
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
