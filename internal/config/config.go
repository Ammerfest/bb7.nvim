package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

var (
	ErrNoConfig        = errors.New("config file not found")
	ErrNoAPIKey        = errors.New("api_key not set in config")
	ErrInvalidJSON     = errors.New("invalid config JSON")
	ErrInvalidDiffMode = errors.New("diff_mode must be \"search_replace\", \"search_replace_multi\", \"anchored\", or \"off\"")
)

// Config holds the global BB-7 configuration.
type Config struct {
	APIKey                string  `json:"api_key"`
	BaseURL               string  `json:"base_url"`
	DefaultModel          string  `json:"default_model"`
	TitleModel            string  `json:"title_model"`              // Model for auto-generating chat titles (cheap/fast)
	AllowDataRetention    *bool   `json:"allow_data_retention"`     // Allow providers that retain data (default: true)
	AllowTraining         *bool   `json:"allow_training"`           // Allow providers that train on data (default: false)
	DiffMode              *string `json:"diff_mode"`                // Diff tool mode: "search_replace_multi", "search_replace", "anchored", or "off"
	ExplicitCacheKey      *bool   `json:"explicit_cache_key"`       // Send prompt_cache_key with chat requests (default: false)
	AutoRetryPartialEdits *bool   `json:"auto_retry_partial_edits"` // Hidden repair retry after partial diff apply failures (default: false)
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
	if cfg.AllowDataRetention == nil {
		t := true
		cfg.AllowDataRetention = &t
	}
	if cfg.AllowTraining == nil {
		f := false
		cfg.AllowTraining = &f
	}
	if cfg.DiffMode == nil {
		dm := "search_replace_multi"
		cfg.DiffMode = &dm
	}
	if cfg.ExplicitCacheKey == nil {
		f := false
		cfg.ExplicitCacheKey = &f
	}
	if cfg.AutoRetryPartialEdits == nil {
		f := false
		cfg.AutoRetryPartialEdits = &f
	}
	switch *cfg.DiffMode {
	case "search_replace", "search_replace_multi", "anchored", "off":
		// valid
	default:
		return nil, ErrInvalidDiffMode
	}

	return &cfg, nil
}
