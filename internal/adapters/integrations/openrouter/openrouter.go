package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"bratok/internal/domain/entities"
	"bratok/internal/interfaces"
)

var _ interfaces.AIClient = (*Client)(nil)

// Options — настройки клиента (передаются явно, без зависимости от config).
type Options struct {
	APIKey         string
	Model          string
	URL            string
	Referer        string   // заголовок HTTP-Referer (атрибуция OpenRouter)
	Title          string   // заголовок X-Title (атрибуция OpenRouter)
	ProviderOrder  []string // приоритетный порядок провайдеров OpenRouter
	ProviderIgnore []string // провайдеры, которые не использовать (например, Azure)
	Temperature    float64  // «температура» генерации
}

// Client общается с эндпоинтом chat-completions OpenRouter.
type Client struct {
	httpClient *http.Client
	opts       Options
	log        *slog.Logger
}

// New собирает клиент; httpClient внедряется снаружи (таймаут, прокси).
func New(httpClient *http.Client, opts Options, log *slog.Logger) *Client {
	return &Client{httpClient: httpClient, opts: opts, log: log}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// providerPrefs — маршрутизация провайдеров OpenRouter.
type providerPrefs struct {
	Order  []string `json:"order,omitempty"`
	Ignore []string `json:"ignore,omitempty"`
}

type chatRequest struct {
	Model       string         `json:"model"`
	Messages    []chatMessage  `json:"messages"`
	Temperature float64        `json:"temperature"`
	Provider    *providerPrefs `json:"provider,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Complete отправляет сообщения модели и возвращает ответ ассистента.
func (c *Client) Complete(ctx context.Context, messages []entities.Message) (string, error) {
	payload := chatRequest{Model: c.opts.Model, Messages: toWire(messages), Temperature: c.opts.Temperature}
	if len(c.opts.ProviderOrder) > 0 || len(c.opts.ProviderIgnore) > 0 {
		payload.Provider = &providerPrefs{Order: c.opts.ProviderOrder, Ignore: c.opts.ProviderIgnore}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("openrouter: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.opts.URL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("openrouter: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.opts.APIKey)
	if c.opts.Referer != "" {
		req.Header.Set("HTTP-Referer", c.opts.Referer)
	}
	if c.opts.Title != "" {
		req.Header.Set("X-Title", c.opts.Title)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openrouter: do request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("openrouter: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Провайдер (часто Azure) мог отклонить запрос своим фильтром контента.
		if bytes.Contains(data, []byte("content_filter")) {
			return "", fmt.Errorf("openrouter: %w", interfaces.ErrContentFiltered)
		}
		return "", fmt.Errorf("openrouter: unexpected status %d: %s", resp.StatusCode, truncate(string(data), 500))
	}

	var parsed chatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("openrouter: decode response: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", fmt.Errorf("openrouter: api error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("openrouter: empty choices in response")
	}

	answer := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if answer == "" {
		return "", fmt.Errorf("openrouter: empty assistant content")
	}
	c.log.Debug("openrouter completion ok", "messages", len(messages))
	return answer, nil
}

// toWire преобразует доменные сообщения в формат API.
func toWire(messages []entities.Message) []chatMessage {
	out := make([]chatMessage, len(messages))
	for i, m := range messages {
		out[i] = chatMessage{Role: m.Role, Content: m.Content}
	}
	return out
}

// truncate укорачивает длинные тела ошибок для логов.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
