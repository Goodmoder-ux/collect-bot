package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	defaultDBPath       = "data/app.db"
	defaultTimezone     = "Europe/Kyiv"
	defaultReminderHour = 15
	defaultPollInterval = 70 * time.Second
	// minPollInterval keeps polling within Monobank's one-request-per-minute limit.
	minPollInterval   = 60 * time.Second
	defaultHealthPort = 8080
)

type Config struct {
	TelegramToken   string
	MonobankToken   string
	JarAccountID    string
	JarURL          string
	AdminTelegramID int64
	SQLiteDBPath    string

	Location     *time.Location
	ReminderHour int
	PollInterval time.Duration
	LogLevel     slog.Level
	LogFormat    string
	// HealthPort is the localhost port of the health endpoint; 0 disables it.
	HealthPort int
}

// Load reads configuration from the environment (and an optional .env file).
// All problems are reported together so a new install can be fixed in one pass.
func Load() (Config, error) {
	_ = godotenv.Load()

	var errs []error
	cfg := Config{
		TelegramToken: required("TELEGRAM_TOKEN", &errs),
		MonobankToken: required("MONOBANK_TOKEN", &errs),
		JarAccountID:  required("JAR_ACCOUNT_ID", &errs),
		JarURL:        required("JAR_URL", &errs),
		SQLiteDBPath:  firstNonEmpty(env("DB_PATH"), env("SQLITE_DB_PATH"), defaultDBPath),
		LogFormat:     strings.ToLower(firstNonEmpty(env("LOG_FORMAT"), "text")),
	}

	if value := required("ADMIN_TELEGRAM_ID", &errs); value != "" {
		id, err := strconv.ParseInt(value, 10, 64)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("ADMIN_TELEGRAM_ID: %w", err))
		case id <= 0:
			errs = append(errs, errors.New("ADMIN_TELEGRAM_ID must be positive"))
		default:
			cfg.AdminTelegramID = id
		}
	}

	location, err := time.LoadLocation(firstNonEmpty(env("TZ"), defaultTimezone))
	if err != nil {
		errs = append(errs, fmt.Errorf("TZ: %w", err))
	}
	cfg.Location = location

	cfg.ReminderHour = intVar("REMINDER_HOUR", defaultReminderHour, &errs)
	if cfg.ReminderHour < 0 || cfg.ReminderHour > 23 {
		errs = append(errs, errors.New("REMINDER_HOUR must be between 0 and 23"))
	}

	cfg.PollInterval = defaultPollInterval
	if value := env("POLL_INTERVAL"); value != "" {
		interval, err := time.ParseDuration(value)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("POLL_INTERVAL: %w", err))
		case interval < minPollInterval:
			errs = append(errs, fmt.Errorf("POLL_INTERVAL must be at least %s (Monobank rate limit)", minPollInterval))
		default:
			cfg.PollInterval = interval
		}
	}

	if err := cfg.LogLevel.UnmarshalText([]byte(firstNonEmpty(env("LOG_LEVEL"), "info"))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}
	if cfg.LogFormat != "text" && cfg.LogFormat != "json" {
		errs = append(errs, errors.New("LOG_FORMAT must be text or json"))
	}

	cfg.HealthPort = intVar("HEALTH_PORT", defaultHealthPort, &errs)
	if cfg.HealthPort < 0 || cfg.HealthPort > 65535 {
		errs = append(errs, errors.New("HEALTH_PORT must be between 0 and 65535"))
	}

	if err := errors.Join(errs...); err != nil {
		return Config{}, fmt.Errorf("config:\n%w", err)
	}
	return cfg, nil
}

// HealthPortFromEnv returns the health endpoint port without loading the full config,
// so the healthcheck subcommand works without access to the secrets.
func HealthPortFromEnv() int {
	_ = godotenv.Load()
	port, err := strconv.Atoi(env("HEALTH_PORT"))
	if err != nil {
		return defaultHealthPort
	}
	return port
}

// DBPathFromEnv returns the database path without validating the rest of the config.
func DBPathFromEnv() string {
	_ = godotenv.Load()
	return firstNonEmpty(env("DB_PATH"), env("SQLITE_DB_PATH"), defaultDBPath)
}

func env(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

func required(name string, errs *[]error) string {
	value := env(name)
	if value == "" {
		*errs = append(*errs, fmt.Errorf("%s is required", name))
	}
	return value
}

func intVar(name string, fallback int, errs *[]error) int {
	value := env(name)
	if value == "" {
		return fallback
	}
	result, err := strconv.Atoi(value)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", name, err))
		return fallback
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
