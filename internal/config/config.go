package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

var (
	ErrNoConfig    = errors.New("config file not found")
	ErrNoAPIKey    = errors.New("api_key not set in config")
	ErrInvalidJSON = errors.New("invalid config JSON")
)

// Config holds the global BB-7 configuration.
type Config struct {
	APIKey       string `json:"api_key"`
	BaseURL      string `json:"base_url"`
	DefaultModel string `json:"default_model"`
	TitleModel   string `json:"title_model"` // Model for auto-generating chat titles (cheap/fast)
}

// Load reads the config from ~/.config/bb7/config.json.
func Load() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(homeDir, ".config", "bb7", "config.json")
	return LoadFrom(configPath)
}

// LoadFrom reads the config from a specific path.
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoConfig
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, ErrInvalidJSON
	}

	if cfg.APIKey == "" {
		return nil, ErrNoAPIKey
	}

	// Set defaults
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://openrouter.ai/api/v1"
	}
	if cfg.DefaultModel == "" {
		cfg.DefaultModel = "anthropic/claude-sonnet-4"
	}
	if cfg.TitleModel == "" {
		cfg.TitleModel = "anthropic/claude-3-haiku"
	}

	return &cfg, nil
}
