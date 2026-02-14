package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration for the FirstIn poller.
type Config struct {
	PollingInterval time.Duration
	Companies       []CompanyConfig
	Filters         FilterConfig
	Notification    NotificationConfig
}

// NotificationConfig controls which notifier is used and its settings.
type NotificationConfig struct {
	Type       string `yaml:"type"`        // "log" or "slack"
	WebhookURL string `yaml:"webhook_url"` // required if type is "slack"
}

// CompanyConfig describes a single company board to poll.
type CompanyConfig struct {
	Name       string `yaml:"name"`
	ATS        string `yaml:"ats"`
	BoardToken string `yaml:"board_token"`
	Enabled    bool   `yaml:"enabled"`
}

// FilterConfig holds keyword and location filter settings.
type FilterConfig struct {
	TitleKeywords []string
	Locations     []string
}

// rawConfig is used for YAML unmarshaling (snake_case fields and duration as string).
type rawConfig struct {
	PollingInterval string             `yaml:"polling_interval"`
	Companies       []CompanyConfig    `yaml:"companies"`
	Filters         rawFilterConfig    `yaml:"filters"`
	Notification    NotificationConfig `yaml:"notification"`
}

type rawFilterConfig struct {
	TitleKeywords []string `yaml:"title_keywords"`
	Locations     []string `yaml:"locations"`
}

// Load reads and parses the YAML config file at path, validates it, and returns Config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	interval, err := time.ParseDuration(raw.PollingInterval)
	if err != nil {
		return nil, fmt.Errorf("parse polling_interval %q: %w", raw.PollingInterval, err)
	}

	cfg := &Config{
		PollingInterval: interval,
		Companies:       raw.Companies,
		Filters: FilterConfig{
			TitleKeywords: raw.Filters.TitleKeywords,
			Locations:     raw.Filters.Locations,
		},
		Notification: raw.Notification,
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func validate(cfg *Config) error {
	if cfg.PollingInterval <= 0 {
		return fmt.Errorf("polling_interval must be positive, got %v", cfg.PollingInterval)
	}
	enabled := 0
	for _, c := range cfg.Companies {
		if c.Enabled {
			enabled++
		}
	}
	if enabled == 0 {
		return fmt.Errorf("at least one company must be enabled")
	}

	if cfg.Notification.Type == "slack" {
		if cfg.Notification.WebhookURL == "" {
			return fmt.Errorf("notification.webhook_url is required when type is \"slack\"")
		}
		if len(cfg.Notification.WebhookURL) < len("https://hooks.slack.com/") ||
			cfg.Notification.WebhookURL[:len("https://hooks.slack.com/")] != "https://hooks.slack.com/" {
			return fmt.Errorf("notification.webhook_url must start with https://hooks.slack.com/")
		}
	}

	return nil
}
