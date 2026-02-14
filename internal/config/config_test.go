package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
polling_interval: 5m
companies:
  - name: acme
    ats: greenhouse
    board_token: "acme"
    enabled: true
filters:
  title_keywords:
    - engineer
  locations:
    - Remote
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PollingInterval != 5*time.Minute {
		t.Errorf("PollingInterval = %v, want 5m", cfg.PollingInterval)
	}
	if len(cfg.Companies) != 1 || cfg.Companies[0].Name != "acme" || cfg.Companies[0].BoardToken != "acme" {
		t.Errorf("Companies = %+v", cfg.Companies)
	}
	if len(cfg.Filters.TitleKeywords) != 1 || cfg.Filters.TitleKeywords[0] != "engineer" {
		t.Errorf("TitleKeywords = %v", cfg.Filters.TitleKeywords)
	}
	if len(cfg.Filters.Locations) != 1 || cfg.Filters.Locations[0] != "Remote" {
		t.Errorf("Locations = %v", cfg.Filters.Locations)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err == nil {
		t.Fatal("Load: expected error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("polling_interval: [broken"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load: expected error for invalid YAML")
	}
}

func TestLoad_ZeroPollingInterval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
polling_interval: 0
companies:
  - name: acme
    ats: greenhouse
    board_token: "acme"
    enabled: true
filters:
  title_keywords: []
  locations: []
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load: expected validation error for zero polling interval")
	}
}

func TestLoad_NoEnabledCompanies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
polling_interval: 5m
companies:
  - name: acme
    ats: greenhouse
    board_token: "acme"
    enabled: false
filters:
  title_keywords: []
  locations: []
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load: expected validation error when no company is enabled")
	}
}
