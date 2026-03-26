package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type AppConfig struct {
	ID                     string `json:"id" yaml:"id"`
	Key                    string `json:"key" yaml:"key"`
	Secret                 string `json:"secret" yaml:"secret"`
	WebhookURL             string `json:"webhook_url" yaml:"webhook_url"`
	ClientEvents           bool   `json:"client_events" yaml:"client_events"`
	MaxConnections         int    `json:"max_connections" yaml:"max_connections"`
	MaxClientEventsPerSec  int    `json:"max_client_events_per_sec" yaml:"max_client_events_per_sec"`
	MaxBackendEventsPerSec int    `json:"max_backend_events_per_sec" yaml:"max_backend_events_per_sec"`
}

type Config struct {
	Host           string      `json:"host" yaml:"host"`
	Port           int         `json:"port" yaml:"port"`
	StatsPort      int         `json:"stats_port" yaml:"stats_port"`
	LogLevel       string      `json:"log_level" yaml:"log_level"`
	TLSCert        string      `json:"tls_cert" yaml:"tls_cert"`
	TLSKey         string      `json:"tls_key" yaml:"tls_key"`
	MaxMessageSize int64       `json:"max_message_size" yaml:"max_message_size"`
	AllowedOrigins []string    `json:"allowed_origins" yaml:"allowed_origins"`
	Apps           []AppConfig `json:"apps" yaml:"apps"`
}

const DefaultMaxMessageSize int64 = 100 * 1024 // 100 KB

func DefaultConfig() Config {
	return Config{
		Host:           "0.0.0.0",
		Port:           6001,
		StatsPort:      6060,
		MaxMessageSize: DefaultMaxMessageSize,
	}
}

func LoadConfig() (Config, error) {
	cfg := DefaultConfig()

	// Try config file first
	if path := os.Getenv("GOCKET_CONFIG_FILE"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("reading config file: %w", err)
		}
		switch filepath.Ext(path) {
		case ".yaml", ".yml":
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return cfg, fmt.Errorf("parsing config file: %w", err)
			}
		default:
			if err := json.Unmarshal(data, &cfg); err != nil {
				return cfg, fmt.Errorf("parsing config file: %w", err)
			}
		}
	}

	// Env vars override
	if h := os.Getenv("GOCKET_HOST"); h != "" {
		cfg.Host = h
	}
	if ll := os.Getenv("GOCKET_LOG_LEVEL"); ll != "" {
		cfg.LogLevel = ll
	}
	if p := os.Getenv("GOCKET_PORT"); p != "" {
		port, err := strconv.Atoi(p)
		if err != nil {
			return cfg, fmt.Errorf("invalid GOCKET_PORT: %w", err)
		}
		cfg.Port = port
	}
	if p := os.Getenv("GOCKET_STATS_PORT"); p != "" {
		port, err := strconv.Atoi(p)
		if err != nil {
			return cfg, fmt.Errorf("invalid GOCKET_STATS_PORT: %w", err)
		}
		cfg.StatsPort = port
	}
	if apps := os.Getenv("GOCKET_APPS"); apps != "" {
		var appConfigs []AppConfig
		if err := json.Unmarshal([]byte(apps), &appConfigs); err != nil {
			return cfg, fmt.Errorf("invalid GOCKET_APPS: %w", err)
		}
		cfg.Apps = appConfigs
	}

	if origins := os.Getenv("GOCKET_ALLOWED_ORIGINS"); origins != "" {
		cfg.AllowedOrigins = strings.Split(origins, ",")
	}

	if ms := os.Getenv("GOCKET_MAX_MESSAGE_SIZE"); ms != "" {
		size, err := strconv.ParseInt(ms, 10, 64)
		if err != nil {
			return cfg, fmt.Errorf("invalid GOCKET_MAX_MESSAGE_SIZE: %w", err)
		}
		cfg.MaxMessageSize = size
	}

	if c := os.Getenv("GOCKET_TLS_CERT"); c != "" {
		cfg.TLSCert = c
	}
	if k := os.Getenv("GOCKET_TLS_KEY"); k != "" {
		cfg.TLSKey = k
	}

	if len(cfg.Apps) == 0 {
		return cfg, fmt.Errorf("no apps configured")
	}

	// Validate webhook URLs against SSRF
	for _, ac := range cfg.Apps {
		if ac.WebhookURL != "" {
			if err := validateWebhookURL(ac.WebhookURL); err != nil {
				return cfg, fmt.Errorf("app %q webhook_url: %w", ac.ID, err)
			}
		}
	}

	// Validate TLS config: both or neither
	if (cfg.TLSCert == "") != (cfg.TLSKey == "") {
		return cfg, fmt.Errorf("both tls_cert and tls_key must be set, or neither")
	}

	return cfg, nil
}

func validateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https, got %q", u.Scheme)
	}

	host := u.Hostname()
	ips, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("cannot resolve host %q: %w", host, err)
	}

	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("host %q resolves to internal IP %s", host, ipStr)
		}
	}

	return nil
}

func ParseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
