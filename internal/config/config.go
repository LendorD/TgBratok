// Package config читает и валидирует конфигурацию из окружения в одном месте.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultModel          = "openai/gpt-4o-mini"
	defaultURL            = "https://openrouter.ai/api/v1/chat/completions"
	defaultTitle          = "Bratok Bot"
	defaultHistoryLimit   = 20
	defaultRequestTimeout = 60 * time.Second
)

// Config — все настройки бота времени выполнения.
type Config struct {
	TelegramToken     string        // токен @BotFather (обязательный)
	OpenRouterKey     string        // ключ OpenRouter (обязательный)
	OpenRouterModel   string        // slug модели
	OpenRouterURL     string        // эндпоинт chat-completions
	OpenRouterReferer string        // заголовок HTTP-Referer (атрибуция)
	OpenRouterTitle   string        // заголовок X-Title (атрибуция)
	ProxyURL          string        // прокси для Telegram и OpenRouter (опц.)
	HistoryLimit      int           // глубина истории диалога
	RequestTimeout    time.Duration // таймаут запроса к OpenRouter
	LogLevel          slog.Level    // минимальный уровень логирования
}

// Load читает конфигурацию из env и валидирует обязательные поля.
func Load() (Config, error) {
	// Подтягиваем .env для локального запуска; реальные env имеют приоритет.
	loadDotEnv(".env")

	cfg := Config{
		TelegramToken:     os.Getenv("TELEGRAM_BOT_TOKEN"),
		OpenRouterKey:     os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterModel:   getEnv("OPENROUTER_MODEL", defaultModel),
		OpenRouterURL:     getEnv("OPENROUTER_URL", defaultURL),
		OpenRouterReferer: os.Getenv("OPENROUTER_REFERER"),
		OpenRouterTitle:   getEnv("OPENROUTER_TITLE", defaultTitle),
		ProxyURL:          os.Getenv("PROXY_URL"),
	}

	if cfg.TelegramToken == "" {
		return Config{}, errors.New("TELEGRAM_BOT_TOKEN is required")
	}
	if cfg.OpenRouterKey == "" {
		return Config{}, errors.New("OPENROUTER_API_KEY is required")
	}

	limit, err := getEnvInt("HISTORY_LIMIT", defaultHistoryLimit)
	if err != nil {
		return Config{}, err
	}
	if limit <= 0 {
		return Config{}, fmt.Errorf("HISTORY_LIMIT must be positive, got %d", limit)
	}
	cfg.HistoryLimit = limit

	timeout, err := getEnvDuration("REQUEST_TIMEOUT", defaultRequestTimeout)
	if err != nil {
		return Config{}, err
	}
	cfg.RequestTimeout = timeout

	cfg.LogLevel = parseLogLevel(getEnv("LOG_LEVEL", "info"))
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return v, nil
}

func getEnvDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return v, nil
}

func parseLogLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
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
