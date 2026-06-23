// Package usecases содержит бизнес-логику бота, не зависящую от транспорта,
// HTTP и конкретного хранилища.
package usecases

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"bratok/internal/domain/entities"
	"bratok/internal/interfaces"
)

const (
	greetingText = "Привет! Я бот, который общается с нейросетью в выбранной тобой роли.\n\n" +
		"Сначала выбери роль командой /role — например «Психолог», «Мотиватор», " +
		"«Программист» или «Друг». Можно придумать и свою.\n\n" +
		"После этого просто пиши мне сообщения, и я отвечу, помня контекст нашего диалога."

	roleSetTemplate = "Готово! Теперь я — «%s». История диалога очищена, начинаем с чистого листа. " +
		"Напиши что-нибудь, и поехали 🙂"

	roleEmptyText = "Роль не может быть пустой. Напиши название роли (например, «Друг») или опиши свою."

	needRoleText = "Сначала выбери роль командой /role, а потом будем общаться."
)

// проверка, что ChatUseCase реализует порт ChatUsecase.
var _ interfaces.ChatUsecase = (*ChatUseCase)(nil)

// ChatUseCase реализует поведение бота: приветствие, выбор роли и диалог с ИИ.
type ChatUseCase struct {
	users        interfaces.UserRepository
	history      interfaces.ChatHistoryRepository
	ai           interfaces.AIClient
	historyLimit int
	log          *slog.Logger
}

// NewChatUseCase связывает use case с зависимостями (внедряются как интерфейсы).
func NewChatUseCase(
	users interfaces.UserRepository,
	history interfaces.ChatHistoryRepository,
	ai interfaces.AIClient,
	historyLimit int,
	log *slog.Logger,
) *ChatUseCase {
	return &ChatUseCase{
		users:        users,
		history:      history,
		ai:           ai,
		historyLimit: historyLimit,
		log:          log,
	}
}

// Start обрабатывает /start: возвращает приветствие, не трогая роль и историю.
func (uc *ChatUseCase) Start(ctx context.Context, chatID int64) (string, error) {
	u, err := uc.users.Get(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("start: get user: %w", err)
	}
	if err := uc.users.Save(ctx, u); err != nil {
		return "", fmt.Errorf("start: save user: %w", err)
	}
	uc.log.Info("user started", "chat_id", chatID)
	return greetingText, nil
}

// RequestRole обрабатывает /role: помечает чат как ожидающий роль и отдаёт меню.
func (uc *ChatUseCase) RequestRole(ctx context.Context, chatID int64) (string, error) {
	u, err := uc.users.Get(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("request role: get user: %w", err)
	}
	u.AwaitingRole = true
	if err := uc.users.Save(ctx, u); err != nil {
		return "", fmt.Errorf("request role: save user: %w", err)
	}
	return roleMenuText(), nil
}

// HandleMessage маршрутизирует текст: если роль ещё не задана или ожидается —
// это ввод роли, иначе обычное сообщение модели.
func (uc *ChatUseCase) HandleMessage(ctx context.Context, chatID int64, text string) (string, error) {
	u, err := uc.users.Get(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("handle message: get user: %w", err)
	}
	if u.AwaitingRole || u.Role == "" {
		return uc.SetRole(ctx, chatID, text)
	}
	return uc.SendMessage(ctx, chatID, text)
}

// SetRole сохраняет роль как системный промпт и очищает историю для нового
// контекста. Пустой ввод отклоняется без изменений.
func (uc *ChatUseCase) SetRole(ctx context.Context, chatID int64, input string) (string, error) {
	prompt := entities.ResolveRolePrompt(input)
	if prompt == "" {
		return roleEmptyText, nil
	}

	u, err := uc.users.Get(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("set role: get user: %w", err)
	}
	u.Role = prompt
	u.AwaitingRole = false
	if err := uc.users.Save(ctx, u); err != nil {
		return "", fmt.Errorf("set role: save user: %w", err)
	}
	if err := uc.history.Clear(ctx, chatID); err != nil {
		return "", fmt.Errorf("set role: clear history: %w", err)
	}

	uc.log.Info("role set", "chat_id", chatID)
	return fmt.Sprintf(roleSetTemplate, roleLabel(input, prompt)), nil
}

// SendMessage собирает [система + история + текущая реплика], вызывает модель и
// сохраняет в историю и вопрос, и ответ.
func (uc *ChatUseCase) SendMessage(ctx context.Context, chatID int64, text string) (string, error) {
	u, err := uc.users.Get(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("send message: get user: %w", err)
	}
	if u.Role == "" {
		return needRoleText, nil
	}

	history, err := uc.history.LastN(ctx, chatID, uc.historyLimit)
	if err != nil {
		return "", fmt.Errorf("send message: load history: %w", err)
	}

	messages := make([]entities.Message, 0, len(history)+2)
	messages = append(messages, entities.Message{Role: entities.RoleSystem, Content: entities.BuildSystemPrompt(u.Role)})
	messages = append(messages, history...)
	userMsg := entities.Message{Role: entities.RoleUser, Content: text}
	messages = append(messages, userMsg)

	answer, err := uc.ai.Complete(ctx, messages)
	if err != nil {
		// Фильтр контента провайдера — это не сбой бота, отвечаем понятно.
		if errors.Is(err, interfaces.ErrContentFiltered) {
			return "Запрос отклонён фильтром контента провайдера. Попробуй переформулировать сообщение.", nil
		}
		return "", fmt.Errorf("send message: ai complete: %w", err)
	}
	assistantMsg := entities.Message{Role: entities.RoleAssistant, Content: answer}

	if err := uc.history.Append(ctx, chatID, userMsg); err != nil {
		return "", fmt.Errorf("send message: append user turn: %w", err)
	}
	if err := uc.history.Append(ctx, chatID, assistantMsg); err != nil {
		return "", fmt.Errorf("send message: append assistant turn: %w", err)
	}

	return answer, nil
}

// roleMenuText — подсказка для команды /role (сами роли показываются кнопками).
func roleMenuText() string {
	return "Выбери роль кнопкой ниже 👇 или опиши свою текстом " +
		"(например: «Ты — строгий редактор, который правит тексты»).\n\n" +
		"При смене роли история диалога очищается."
}

// roleLabel возвращает короткую метку роли: имя предустановленной либо обрезанный
// фрагмент кастомного промпта (режем по рунам, чтобы не разрезать символ).
func roleLabel(input, prompt string) string {
	trimmed := strings.TrimSpace(input)
	for _, r := range entities.PredefinedRoles {
		if strings.EqualFold(trimmed, r.Name) {
			return r.Name
		}
	}
	const max = 40
	runes := []rune(prompt)
	if len(runes) > max {
		return strings.TrimSpace(string(runes[:max])) + "…"
	}
	return prompt
}
