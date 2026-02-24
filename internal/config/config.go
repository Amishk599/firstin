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
	Companies      []CompanyConfig
	Filters        FilterConfig
	Notification   NotificationConfig
	RateLimit      RateLimitConfig
	AI             AIConfig
}

// AIConfig controls the optional OpenAI enrichment layer.
type AIConfig struct {
	Enabled bool
	BaseURL string        // defaults to https://api.openai.com/v1
	Model   string        // OpenAI model identifier, e.g. "gpt-4o-mini"
	APIKey  string        // expanded from env var by Load
	Timeout time.Duration // per-request timeout
}

// RateLimitConfig controls ATS-level rate limiting.
type RateLimitConfig struct {
	MinDelay     time.Duration            // minimum gap between requests to the same ATS
	ATSOverrides map[string]time.Duration // per-ATS overrides, keyed by ATS name
}

// MinDelayFor returns the configured delay for the given ATS, falling back to MinDelay.
func (r RateLimitConfig) MinDelayFor(ats string) time.Duration {
	if d, ok := r.ATSOverrides[ats]; ok {
		return d
	}
	return r.MinDelay
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
	WorkdayURL string `yaml:"workday_url"`
	Enabled    bool   `yaml:"enabled"`
}

// FilterConfig holds keyword and location filter settings.
type FilterConfig struct {
	TitleKeywords        []string
	TitleExcludeKeywords []string
	Locations            []string
	ExcludeLocations     []string
	MaxAge               time.Duration // max age of a job posting to be considered fresh
}

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

// rawConfig is used for YAML unmarshaling (snake_case fields and duration as string).
type rawConfig struct {
	PollingInterval string             `yaml:"polling_interval"`
	Companies       []CompanyConfig    `yaml:"companies"`
	Filters         rawFilterConfig    `yaml:"filters"`
	Notification    NotificationConfig `yaml:"notification"`
	RateLimit       rawRateLimitConfig `yaml:"rate_limit"`
	AI              rawAIConfig        `yaml:"ai"`
}

type rawAIConfig struct {
	Enabled bool   `yaml:"enabled"`
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
	APIKey  string `yaml:"api_key"`
	Timeout string `yaml:"timeout"`
}

type rawRateLimitConfig struct {
	MinDelay     string            `yaml:"min_delay"`
	ATSOverrides map[string]string `yaml:"ats_overrides"`
}

type rawFilterConfig struct {
	TitleKeywords        []string `yaml:"title_keywords"`
	TitleExcludeKeywords []string `yaml:"title_exclude_keywords"`
	Locations            []string `yaml:"locations"`
	ExcludeLocations     []string `yaml:"exclude_locations"`
	MaxAge               string   `yaml:"max_age"`
}

// Load reads and parses the YAML config file at path, validates it, and returns Config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	var raw rawConfig
	if err := yaml.Unmarshal([]byte(expanded), &raw); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	interval, err := time.ParseDuration(raw.PollingInterval)
	if err != nil {
		return nil, fmt.Errorf("parse polling_interval %q: %w", raw.PollingInterval, err)
	}

	maxAge := 1 * time.Hour // default: 1 hour
	if raw.Filters.MaxAge != "" {
		maxAge, err = time.ParseDuration(raw.Filters.MaxAge)
		if err != nil {
			return nil, fmt.Errorf("parse filters.max_age %q: %w", raw.Filters.MaxAge, err)
		}
	}

	rateLimitDelay := 600 * time.Second // default: 5 mins
	if raw.RateLimit.MinDelay != "" {
		rateLimitDelay, err = time.ParseDuration(raw.RateLimit.MinDelay)
		if err != nil {
			return nil, fmt.Errorf("parse rate_limit.min_delay %q: %w", raw.RateLimit.MinDelay, err)
		}
	}

	atsOverrides := make(map[string]time.Duration)
	for ats, raw := range raw.RateLimit.ATSOverrides {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return nil, fmt.Errorf("parse rate_limit.ats_overrides[%q]: %w", ats, err)
		}
		atsOverrides[ats] = d
	}

	aiTimeout := 30 * time.Second // default
	if raw.AI.Timeout != "" {
		aiTimeout, err = time.ParseDuration(raw.AI.Timeout)
		if err != nil {
			return nil, fmt.Errorf("parse ai.timeout %q: %w", raw.AI.Timeout, err)
		}
	}

	aiBaseURL := raw.AI.BaseURL
	if aiBaseURL == "" {
		aiBaseURL = defaultOpenAIBaseURL
	}

	cfg := &Config{
		PollingInterval: interval,
		Companies: raw.Companies,
		Filters: FilterConfig{
			TitleKeywords:        raw.Filters.TitleKeywords,
			TitleExcludeKeywords: raw.Filters.TitleExcludeKeywords,
			Locations:            raw.Filters.Locations,
			ExcludeLocations:     raw.Filters.ExcludeLocations,
			MaxAge:               maxAge,
		},
		Notification: raw.Notification,
		RateLimit: RateLimitConfig{
			MinDelay:     rateLimitDelay,
			ATSOverrides: atsOverrides,
		},
		AI: AIConfig{
			Enabled: raw.AI.Enabled,
			BaseURL: aiBaseURL,
			Model:   raw.AI.Model,
			APIKey:  raw.AI.APIKey,
			Timeout: aiTimeout,
		},
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

	if cfg.Filters.MaxAge < 1*time.Hour || cfg.Filters.MaxAge > 24*time.Hour {
		return fmt.Errorf("filters.max_age must be between 1h and 24h, got %v", cfg.Filters.MaxAge)
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

	if cfg.AI.Enabled {
		if cfg.AI.APIKey == "" {
			return fmt.Errorf("ai.api_key is required when ai.enabled is true")
		}
		if cfg.AI.BaseURL == "" {
			return fmt.Errorf("ai.base_url (or a known ai.provider) is required when ai.enabled is true")
		}
		if cfg.AI.Model == "" {
			return fmt.Errorf("ai.model is required when ai.enabled is true")
		}
	}

	return nil
}
