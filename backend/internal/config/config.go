// Package config читает настройки приложения из переменных окружения.
// Секретов на Этапе 1 нет, но конфигурация сразу вынесена из кода,
// чтобы на Этапе 2 (ключи LLM-провайдеров) ничего не переделывать.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

// Config — полный набор настроек сервера.
type Config struct {
	Port             string
	DBPath           string
	CheckTimeout     time.Duration
	CheckConcurrency int
	LogLevel         slog.Level
}

// Значения по умолчанию рассчитаны на запуск в контейнере
// (см. docs/main/design-stage1.md, п. 4.3).
const (
	defaultPort             = "8080"
	defaultDBPath           = "/data/hopecore.db"
	defaultCheckTimeout     = 10 * time.Second
	defaultCheckConcurrency = 4
)

// Load собирает конфигурацию из окружения. Ошибка возвращается только на
// заведомо некорректных значениях, чтобы приложение падало на старте,
// а не вело себя неожиданно во время работы.
func Load() (Config, error) {
	cfg := Config{
		Port:             envString("PORT", defaultPort),
		DBPath:           envString("DB_PATH", defaultDBPath),
		CheckTimeout:     defaultCheckTimeout,
		CheckConcurrency: defaultCheckConcurrency,
		LogLevel:         slog.LevelInfo,
	}

	if raw := os.Getenv("CHECK_TIMEOUT"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("CHECK_TIMEOUT=%q: %w", raw, err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("CHECK_TIMEOUT=%q: должен быть больше нуля", raw)
		}
		cfg.CheckTimeout = d
	}

	if raw := os.Getenv("CHECK_CONCURRENCY"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("CHECK_CONCURRENCY=%q: %w", raw, err)
		}
		if n < 1 {
			return Config{}, fmt.Errorf("CHECK_CONCURRENCY=%q: должен быть не меньше 1", raw)
		}
		cfg.CheckConcurrency = n
	}

	if raw := os.Getenv("LOG_LEVEL"); raw != "" {
		lvl, err := parseLogLevel(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.LogLevel = lvl
	}

	return cfg, nil
}

func envString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseLogLevel(raw string) (slog.Level, error) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(raw)); err != nil {
		return 0, fmt.Errorf("LOG_LEVEL=%q: ожидается debug|info|warn|error", raw)
	}
	return lvl, nil
}
