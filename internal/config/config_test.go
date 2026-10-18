package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("TELEGRAM_TOKEN", "tg")
	t.Setenv("MONOBANK_TOKEN", "mono")
	t.Setenv("JAR_ACCOUNT_ID", "jar")
	t.Setenv("JAR_URL", "https://send.monobank.ua/jar/x")
	t.Setenv("ADMIN_TELEGRAM_ID", "42")
}

func clearOptional(t *testing.T) {
	t.Helper()
	for _, name := range []string{"DB_PATH", "SQLITE_DB_PATH", "TZ", "REMINDER_HOUR", "POLL_INTERVAL", "LOG_LEVEL", "LOG_FORMAT", "HEALTH_PORT"} {
		t.Setenv(name, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Chdir(t.TempDir())
	clearOptional(t)
	setRequired(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.AdminTelegramID != 42 || cfg.SQLiteDBPath != defaultDBPath {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.Location.String() != defaultTimezone || cfg.ReminderHour != 15 || cfg.PollInterval != 70*time.Second {
		t.Fatalf("unexpected schedule defaults: %+v", cfg)
	}
	if cfg.LogLevel != slog.LevelInfo || cfg.LogFormat != "text" || cfg.HealthPort != 8080 {
		t.Fatalf("unexpected logging defaults: %+v", cfg)
	}
}

func TestLoadReportsAllMissingVariables(t *testing.T) {
	t.Chdir(t.TempDir())
	clearOptional(t)
	for _, name := range []string{"TELEGRAM_TOKEN", "MONOBANK_TOKEN", "JAR_ACCOUNT_ID", "JAR_URL", "ADMIN_TELEGRAM_ID"} {
		t.Setenv(name, "")
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load() succeeded without required variables")
	}
	for _, name := range []string{"TELEGRAM_TOKEN", "MONOBANK_TOKEN", "JAR_ACCOUNT_ID", "JAR_URL", "ADMIN_TELEGRAM_ID"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not mention %s: %v", name, err)
		}
	}
}

func TestLoadValidatesOptionalVariables(t *testing.T) {
	t.Chdir(t.TempDir())
	clearOptional(t)
	setRequired(t)
	t.Setenv("TZ", "Mars/Olympus")
	t.Setenv("REMINDER_HOUR", "24")
	t.Setenv("POLL_INTERVAL", "10s")
	t.Setenv("LOG_LEVEL", "loud")
	t.Setenv("LOG_FORMAT", "xml")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() accepted invalid values")
	}
	for _, name := range []string{"TZ", "REMINDER_HOUR", "POLL_INTERVAL", "LOG_LEVEL", "LOG_FORMAT"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not mention %s: %v", name, err)
		}
	}
}

func TestLoadAcceptsDeprecatedDBPathAlias(t *testing.T) {
	t.Chdir(t.TempDir())
	clearOptional(t)
	setRequired(t)
	t.Setenv("SQLITE_DB_PATH", "/tmp/legacy.db")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.SQLiteDBPath != "/tmp/legacy.db" {
		t.Fatalf("SQLiteDBPath = %q", cfg.SQLiteDBPath)
	}
}
