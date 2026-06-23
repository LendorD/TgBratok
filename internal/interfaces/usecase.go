package interfaces

import "context"

// ChatUsecase — бизнес-логика бота, на которую опирается транспортный слой.
type ChatUsecase interface {
	Start(ctx context.Context, chatID int64) (string, error)
	RequestRole(ctx context.Context, chatID int64) (string, error)
	HandleMessage(ctx context.Context, chatID int64, text string) (string, error)
}
