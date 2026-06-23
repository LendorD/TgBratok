// Package handlers — транспортный слой. Принимает обновления Telegram и
// вызывает use case через интерфейс ChatUsecase.
//
// Библиотека go-telegram-bot-api/v5 выбрана как зрелая и без внешних
// зависимостей; long polling делает бота «только исходящим».
package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"bratok/internal/interfaces"
)

// Telegram принимает обновления Telegram и направляет их в use case.
type Telegram struct {
	api *tgbotapi.BotAPI
	uc  interfaces.ChatUsecase
	log *slog.Logger
}

// NewTelegram создаёт обработчик; httpClient позволяет ходить через прокси.
func NewTelegram(token string, httpClient *http.Client, uc interfaces.ChatUsecase, log *slog.Logger) (*Telegram, error) {
	api, err := tgbotapi.NewBotAPIWithClient(token, tgbotapi.APIEndpoint, httpClient)
	if err != nil {
		return nil, err
	}
	return &Telegram{api: api, uc: uc, log: log}, nil
}

// Run запускает long polling и блокируется до отмены ctx, корректно завершая
// обработчики (graceful shutdown).
func (t *Telegram) Run(ctx context.Context) error {
	cfg := tgbotapi.NewUpdate(0)
	cfg.Timeout = 30
	updates := t.api.GetUpdatesChan(cfg)

	t.log.Info("telegram bot started", "username", t.api.Self.UserName)

	var wg sync.WaitGroup
	for {
		select {
		case <-ctx.Done():
			t.api.StopReceivingUpdates()
			wg.Wait()
			t.log.Info("telegram bot stopped")
			return nil
		case update, ok := <-updates:
			if !ok {
				wg.Wait()
				return nil
			}
			if update.Message == nil {
				continue
			}
			// Каждое сообщение — в своей горутине, чтобы медленный вызов модели
			// не блокировал остальных.
			wg.Add(1)
			go func(msg *tgbotapi.Message) {
				defer wg.Done()
				defer t.recover(msg.Chat.ID)
				t.handle(ctx, msg)
			}(update.Message)
		}
	}
}

// recover не даёт панике в обработчике уронить процесс.
func (t *Telegram) recover(chatID int64) {
	if r := recover(); r != nil {
		t.log.Error("recovered from panic in handler", "chat_id", chatID, "panic", r)
	}
}

// handle направляет сообщение в нужный метод use case и отправляет ответ.
func (t *Telegram) handle(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	var (
		reply string
		err   error
	)
	switch {
	case msg.IsCommand():
		switch msg.Command() {
		case "start":
			reply, err = t.uc.Start(ctx, chatID)
		case "role":
			reply, err = t.uc.RequestRole(ctx, chatID)
		default:
			reply = "Не знаю такую команду. Доступны: /start и /role."
		}
	case strings.TrimSpace(msg.Text) == "":
		reply = "Я понимаю только текст. Напиши, пожалуйста, сообщение словами."
	default:
		_, _ = t.api.Request(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping))
		reply, err = t.uc.HandleMessage(ctx, chatID, msg.Text)
	}

	if err != nil {
		t.log.Error("failed to handle update", "chat_id", chatID, "error", err)
		reply = "Упс, что-то пошло не так. Попробуй, пожалуйста, ещё раз чуть позже."
	}

	t.send(chatID, reply)
}

// send отправляет текстовый ответ, логируя ошибки отправки.
func (t *Telegram) send(chatID int64, text string) {
	if text == "" {
		return
	}
	if _, err := t.api.Send(tgbotapi.NewMessage(chatID, text)); err != nil {
		t.log.Error("failed to send message", "chat_id", chatID, "error", err)
	}
}
