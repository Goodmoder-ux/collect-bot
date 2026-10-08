package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramToken   string
	MonobankToken   string
	JarAccountID    string
	JarURL          string
	AdminTelegramID int64
	SQLiteDBPath    string
}

func Load() (Config, error) {
	_ = godotenv.Load()

	adminID, err := requiredInt64("ADMIN_TELEGRAM_ID")
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		TelegramToken:   os.Getenv("TELEGRAM_TOKEN"),
		MonobankToken:   os.Getenv("MONOBANK_TOKEN"),
		JarAccountID:    os.Getenv("JAR_ACCOUNT_ID"),
		JarURL:          os.Getenv("JAR_URL"),
		AdminTelegramID: adminID,
		SQLiteDBPath:    os.Getenv("SQLITE_DB_PATH"),
	}
	for name, value := range map[string]string{
		"TELEGRAM_TOKEN": cfg.TelegramToken,
		"MONOBANK_TOKEN": cfg.MonobankToken,
		"JAR_ACCOUNT_ID": cfg.JarAccountID,
		"JAR_URL":        cfg.JarURL,
		"SQLITE_DB_PATH": cfg.SQLiteDBPath,
	} {
		if strings.TrimSpace(value) == "" {
			return Config{}, fmt.Errorf("config: %s is required", name)
		}
	}
	return cfg, nil
}

func requiredInt64(name string) (int64, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return 0, fmt.Errorf("config: %s is required", name)
	}
	result, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("config: %s: %w", name, err)
	}
	if result <= 0 {
		return 0, fmt.Errorf("config: %s must be positive", name)
	}
	return result, nil
}
