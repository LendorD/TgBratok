package interfaces

import (
	"context"
	"errors"

	"bratok/internal/domain/entities"
)

// ErrContentFiltered — провайдер отклонил запрос из-за фильтра контента.
var ErrContentFiltered = errors.New("ответ отклонён фильтром контента провайдера")

// AIClient — внешняя нейросеть (OpenRouter).
type AIClient interface {
	// Complete отправляет сообщения модели и возвращает ответ ассистента.
	Complete(ctx context.Context, messages []entities.Message) (string, error)
}
