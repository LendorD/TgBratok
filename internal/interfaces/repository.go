// Package interfaces содержит порты (интерфейсы), от которых зависит ядро
// приложения. Реализуются адаптерами — это инверсия зависимостей.
package interfaces

import (
	"context"

	"bratok/internal/domain/entities"
)

// UserRepository хранит роль и флаг ожидания роли по chatID.
type UserRepository interface {
	// Get возвращает состояние чата; для неизвестного чата — User с пустой ролью.
	Get(ctx context.Context, chatID int64) (entities.User, error)
	Save(ctx context.Context, user entities.User) error
}

// ChatHistoryRepository хранит историю диалога для поддержания контекста.
type ChatHistoryRepository interface {
	Append(ctx context.Context, chatID int64, msg entities.Message) error
	// LastN возвращает до n последних сообщений (от старых к новым).
	LastN(ctx context.Context, chatID int64, n int) ([]entities.Message, error)
	Clear(ctx context.Context, chatID int64) error
}
