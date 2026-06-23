package interfaces

import "context"

// ChatUsecase — бизнес-логика бота, на которую опирается транспортный слой.
type ChatUsecase interface {
	// Start обрабатывает /start и возвращает приветствие.
	Start(ctx context.Context, chatID int64) (string, error)
	// RequestRole обрабатывает /role и возвращает меню выбора роли.
	RequestRole(ctx context.Context, chatID int64) (string, error)
	// HandleMessage обрабатывает обычный текст: ввод роли или сообщение модели.
	HandleMessage(ctx context.Context, chatID int64, text string) (string, error)
}
