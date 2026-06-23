// Package handlers — транспортный слой: принимает обновления Telegram и
// вызывает use case через интерфейс ChatUsecase.
//
// Библиотека go-telegram-bot-api/v5 — зрелая, без внешних зависимостей;
// long polling делает бота «только исходящим».
package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"bratok/internal/domain/entities"
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

// Run запускает long polling и блокируется до отмены ctx (graceful shutdown).
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
		reply  string
		err    error
		markup interface{} // клавиатура, если нужна (для /role)
	)
	switch {
	case msg.IsCommand():
		switch msg.Command() {
		case "start":
			reply, err = t.uc.Start(ctx, chatID)
		case "role":
			reply, err = t.uc.RequestRole(ctx, chatID)
			markup = roleKeyboard()
		default:
			reply = "Не знаю такую команду. Доступны: /start и /role."
		}
	case strings.TrimSpace(msg.Text) == "":
		reply = "Я понимаю только текст. Напиши, пожалуйста, сообщение словами."
	default:
		// Индикатор «печатает», пока модель думает (best-effort).
		_, _ = t.api.Request(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping))
		reply, err = t.uc.HandleMessage(ctx, chatID, msg.Text)
	}

	if err != nil {
		t.log.Error("failed to handle update", "chat_id", chatID, "error", err)
		reply = "Упс, что-то пошло не так. Попробуй, пожалуйста, ещё раз чуть позже."
		markup = nil
	}
	t.send(chatID, reply, markup)
}

// send отправляет текстовый ответ (с опциональной клавиатурой), логируя ошибки.
func (t *Telegram) send(chatID int64, text string, markup interface{}) {
	if text == "" {
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	if markup != nil {
		msg.ReplyMarkup = markup
	}
	if _, err := t.api.Send(msg); err != nil {
		t.log.Error("failed to send message", "chat_id", chatID, "error", err)
	}
}

// roleKeyboard строит клавиатуру из предустановленных ролей (по 2 в ряд).
// Нажатие на кнопку отправляет название роли как обычное сообщение.
func roleKeyboard() tgbotapi.ReplyKeyboardMarkup {
	var rows [][]tgbotapi.KeyboardButton
	var row []tgbotapi.KeyboardButton
	for i, r := range entities.PredefinedRoles {
		row = append(row, tgbotapi.NewKeyboardButton(r.Name))
		if len(row) == 2 || i == len(entities.PredefinedRoles)-1 {
			rows = append(rows, row)
			row = nil
		}
	}
	kb := tgbotapi.NewReplyKeyboard(rows...)
	kb.ResizeKeyboard = true
	kb.OneTimeKeyboard = true
	return kb
}
