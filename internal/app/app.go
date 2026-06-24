package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bratok/internal/adapters/handlers"
	"bratok/internal/adapters/integrations/openrouter"
	"bratok/internal/adapters/repositories/memory"
	"bratok/internal/config"
	"bratok/internal/usecases"
)

// Run — единственная публичная точка: поднимает приложение целиком.
func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	// HTTP-клиенты строим централизованно (таймаут + опциональный прокси).
	// У Telegram таймаут нулевой: long polling держит соединение дольше обычного.
	openRouterHTTP, err := newHTTPClient(cfg.ProxyURL, cfg.RequestTimeout)
	if err != nil {
		return err
	}
	telegramHTTP, err := newHTTPClient(cfg.ProxyURL, 0)
	if err != nil {
		return err
	}

	store := memory.New(cfg.HistoryLimit)
	ai := openrouter.New(openRouterHTTP, openrouter.Options{
		APIKey:         cfg.OpenRouterKey,
		Model:          cfg.OpenRouterModel,
		URL:            cfg.OpenRouterURL,
		Referer:        cfg.OpenRouterReferer,
		Title:          cfg.OpenRouterTitle,
		ProviderOrder:  cfg.ProviderOrder,
		ProviderIgnore: cfg.ProviderIgnore,
		Temperature:    cfg.Temperature,
	}, logger)
	chat := usecases.NewChatUseCase(store, store, ai, cfg.HistoryLimit, logger)

	logger.Info("starting bot",
		"model", cfg.OpenRouterModel,
		"history_limit", cfg.HistoryLimit,
		"proxy_enabled", cfg.ProxyURL != "",
	)

	bot, err := handlers.NewTelegram(cfg.TelegramToken, telegramHTTP, chat, logger)
	if err != nil {
		return fmt.Errorf("не удалось подключиться к Telegram (проверьте интернет, PROXY_URL или VPN): %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return bot.Run(ctx)
}

// newLogger строит JSON-логгер slog в stdout на заданном уровне.
func newLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

// newHTTPClient создаёт клиент с таймаутом и прокси (http/https/socks5).
// Если proxyURL пуст, используется прокси из окружения (HTTP(S)_PROXY).
func newHTTPClient(proxyURL string, timeout time.Duration) (*http.Client, error) {
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	if proxyURL != "" {
		u, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid PROXY_URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Timeout: timeout, Transport: transport}, nil
}
