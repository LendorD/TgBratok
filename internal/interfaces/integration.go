package interfaces

import (
	"context"

	"bratok/internal/domain/entities"
)

// AIClient — внешняя нейросеть (OpenRouter): отправляет сообщения и возвращает
// ответ ассистента.
type AIClient interface {
	Complete(ctx context.Context, messages []entities.Message) (string, error)
}
