package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFrom(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		content := `{
			"api_key": "sk-test-123",
			"base_url": "https://api.example.com",
			"default_model": "gpt-4"
		}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		cfg, err := LoadFrom(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.APIKey != "sk-test-123" {
			t.Errorf("APIKey = %q, want %q", cfg.APIKey, "sk-test-123")
		}
		if cfg.BaseURL != "https://api.example.com" {
			t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://api.example.com")
		}
		if cfg.DefaultModel != "gpt-4" {
			t.Errorf("DefaultModel = %q, want %q", cfg.DefaultModel, "gpt-4")
		}
	})

	t.Run("defaults applied", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		content := `{"api_key": "sk-test-123"}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		cfg, err := LoadFrom(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.BaseURL != "https://openrouter.ai/api/v1" {
			t.Errorf("BaseURL = %q, want default", cfg.BaseURL)
		}
		if cfg.DefaultModel != "anthropic/claude-sonnet-4" {
			t.Errorf("DefaultModel = %q, want default", cfg.DefaultModel)
		}
		if cfg.DiffMode == nil || *cfg.DiffMode != "search_replace_multi" {
			t.Errorf("DiffMode should default to \"search_replace_multi\", got %v", cfg.DiffMode)
		}
	})

	t.Run("diff_mode anchored", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		content := `{"api_key": "sk-test-123", "diff_mode": "anchored"}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadFrom(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.DiffMode == nil || *cfg.DiffMode != "anchored" {
			t.Errorf("DiffMode should be \"anchored\", got %v", cfg.DiffMode)
		}
	})

	t.Run("diff_mode off", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		content := `{"api_key": "sk-test-123", "diff_mode": "off"}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadFrom(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.DiffMode == nil || *cfg.DiffMode != "off" {
			t.Errorf("DiffMode should be \"off\", got %v", cfg.DiffMode)
		}
	})

	t.Run("diff_mode invalid", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		content := `{"api_key": "sk-test-123", "diff_mode": "bogus"}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadFrom(path)
		if err != ErrInvalidDiffMode {
			t.Errorf("error = %v, want ErrInvalidDiffMode", err)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := LoadFrom("/nonexistent/path/config.json")
		if err != ErrNoConfig {
			t.Errorf("error = %v, want ErrNoConfig", err)
		}
	})

	t.Run("missing api_key", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		content := `{"base_url": "https://api.example.com"}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := LoadFrom(path)
		if err != ErrNoAPIKey {
			t.Errorf("error = %v, want ErrNoAPIKey", err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := LoadFrom(path)
		if err != ErrInvalidJSON {
			t.Errorf("error = %v, want ErrInvalidJSON", err)
		}
	})
}
