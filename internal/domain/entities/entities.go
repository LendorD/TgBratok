package entities

// Роли сообщений совпадают со значениями API OpenRouter.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message — одна реплика диалога.
type Message struct {
	Role    string
	Content string
}

// User — состояние чата: активная роль (пустая = не выбрана) и флаг ожидания
// ввода новой роли после команды /role.
type User struct {
	ChatID       int64
	Role         string
	AwaitingRole bool
}
