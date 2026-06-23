package usecases

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"bratok/internal/domain/entities"
)

// --- Моки портов ---

type mockUsers struct{ mock.Mock }

func (m *mockUsers) Get(ctx context.Context, chatID int64) (entities.User, error) {
	args := m.Called(ctx, chatID)
	return args.Get(0).(entities.User), args.Error(1)
}

func (m *mockUsers) Save(ctx context.Context, user entities.User) error {
	return m.Called(ctx, user).Error(0)
}

type mockHistory struct{ mock.Mock }

func (m *mockHistory) Append(ctx context.Context, chatID int64, msg entities.Message) error {
	return m.Called(ctx, chatID, msg).Error(0)
}

func (m *mockHistory) LastN(ctx context.Context, chatID int64, n int) ([]entities.Message, error) {
	args := m.Called(ctx, chatID, n)
	var msgs []entities.Message
	if v := args.Get(0); v != nil {
		msgs = v.([]entities.Message)
	}
	return msgs, args.Error(1)
}

func (m *mockHistory) Clear(ctx context.Context, chatID int64) error {
	return m.Called(ctx, chatID).Error(0)
}

type mockAI struct{ mock.Mock }

func (m *mockAI) Complete(ctx context.Context, messages []entities.Message) (string, error) {
	args := m.Called(ctx, messages)
	return args.String(0), args.Error(1)
}

// newUC собирает use case с моками и логгером в /dev/null.
func newUC(u *mockUsers, h *mockHistory, ai *mockAI, limit int) *ChatUseCase {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewChatUseCase(u, h, ai, limit, logger)
}

// SendMessage собирает [system + история + текущее] и сохраняет обе реплики.
func TestSendMessage_BuildsContextAndStoresTurns(t *testing.T) {
	ctx := context.Background()
	const chatID = int64(42)

	users := new(mockUsers)
	history := new(mockHistory)
	ai := new(mockAI)

	users.On("Get", ctx, chatID).Return(entities.User{ChatID: chatID, Role: "SYS"}, nil)

	prev := []entities.Message{
		{Role: entities.RoleUser, Content: "hi"},
		{Role: entities.RoleAssistant, Content: "hello"},
	}
	history.On("LastN", ctx, chatID, 20).Return(prev, nil)

	expected := []entities.Message{
		{Role: entities.RoleSystem, Content: entities.BuildSystemPrompt("SYS")},
		{Role: entities.RoleUser, Content: "hi"},
		{Role: entities.RoleAssistant, Content: "hello"},
		{Role: entities.RoleUser, Content: "how are you"},
	}
	ai.On("Complete", ctx, expected).Return("good", nil)

	history.On("Append", ctx, chatID, entities.Message{Role: entities.RoleUser, Content: "how are you"}).Return(nil)
	history.On("Append", ctx, chatID, entities.Message{Role: entities.RoleAssistant, Content: "good"}).Return(nil)

	uc := newUC(users, history, ai, 20)
	got, err := uc.SendMessage(ctx, chatID, "how are you")

	require.NoError(t, err)
	require.Equal(t, "good", got)
	users.AssertExpectations(t)
	history.AssertExpectations(t)
	ai.AssertExpectations(t)
}

// SendMessage не вызывает модель, если роль не задана.
func TestSendMessage_NoRole(t *testing.T) {
	ctx := context.Background()
	const chatID = int64(1)

	users := new(mockUsers)
	history := new(mockHistory)
	ai := new(mockAI)

	users.On("Get", ctx, chatID).Return(entities.User{ChatID: chatID}, nil)

	uc := newUC(users, history, ai, 20)
	got, err := uc.SendMessage(ctx, chatID, "hey")

	require.NoError(t, err)
	require.Equal(t, needRoleText, got)
	ai.AssertNotCalled(t, "Complete", mock.Anything, mock.Anything)
}

// Ошибка AI пробрасывается, история не меняется.
func TestSendMessage_AIError(t *testing.T) {
	ctx := context.Background()
	const chatID = int64(5)

	users := new(mockUsers)
	history := new(mockHistory)
	ai := new(mockAI)

	users.On("Get", ctx, chatID).Return(entities.User{ChatID: chatID, Role: "SYS"}, nil)
	history.On("LastN", ctx, chatID, 20).Return([]entities.Message(nil), nil)
	ai.On("Complete", ctx, mock.Anything).Return("", errors.New("boom"))

	uc := newUC(users, history, ai, 20)
	_, err := uc.SendMessage(ctx, chatID, "hey")

	require.Error(t, err)
	history.AssertNotCalled(t, "Append", mock.Anything, mock.Anything, mock.Anything)
}

// SetRole распознаёт предустановленную роль, сохраняет её и чистит историю.
func TestSetRole_ResolvesAndClearsHistory(t *testing.T) {
	ctx := context.Background()
	const chatID = int64(7)

	users := new(mockUsers)
	history := new(mockHistory)
	ai := new(mockAI)

	wantPrompt := entities.ResolveRolePrompt("Программист")
	require.NotEmpty(t, wantPrompt)

	users.On("Get", ctx, chatID).Return(entities.User{ChatID: chatID, AwaitingRole: true}, nil)
	users.On("Save", ctx, mock.MatchedBy(func(u entities.User) bool {
		return u.ChatID == chatID && u.Role == wantPrompt && !u.AwaitingRole
	})).Return(nil)
	history.On("Clear", ctx, chatID).Return(nil)

	uc := newUC(users, history, ai, 20)
	reply, err := uc.SetRole(ctx, chatID, "Программист")

	require.NoError(t, err)
	require.Contains(t, reply, "Программист")
	users.AssertExpectations(t)
	history.AssertExpectations(t)
}

// Пустой ввод роли отклоняется без изменения состояния.
func TestSetRole_EmptyInput(t *testing.T) {
	ctx := context.Background()
	const chatID = int64(8)

	users := new(mockUsers)
	history := new(mockHistory)
	ai := new(mockAI)

	uc := newUC(users, history, ai, 20)
	reply, err := uc.SetRole(ctx, chatID, "   ")

	require.NoError(t, err)
	require.Equal(t, roleEmptyText, reply)
	users.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	history.AssertNotCalled(t, "Clear", mock.Anything, mock.Anything)
}

// HandleMessage уводит в SetRole, когда роль не задана.
func TestHandleMessage_RoutesToSetRole(t *testing.T) {
	ctx := context.Background()
	const chatID = int64(9)

	users := new(mockUsers)
	history := new(mockHistory)
	ai := new(mockAI)

	users.On("Get", ctx, chatID).Return(entities.User{ChatID: chatID}, nil)
	users.On("Save", ctx, mock.Anything).Return(nil)
	history.On("Clear", ctx, chatID).Return(nil)

	uc := newUC(users, history, ai, 20)
	_, err := uc.HandleMessage(ctx, chatID, "Друг")

	require.NoError(t, err)
	history.AssertExpectations(t)
	ai.AssertNotCalled(t, "Complete", mock.Anything, mock.Anything)
}

// HandleMessage уводит в SendMessage, когда роль уже задана.
func TestHandleMessage_RoutesToSendMessage(t *testing.T) {
	ctx := context.Background()
	const chatID = int64(10)

	users := new(mockUsers)
	history := new(mockHistory)
	ai := new(mockAI)

	users.On("Get", ctx, chatID).Return(entities.User{ChatID: chatID, Role: "SYS"}, nil)
	history.On("LastN", ctx, chatID, 20).Return([]entities.Message(nil), nil)
	ai.On("Complete", ctx, mock.Anything).Return("answer", nil)
	history.On("Append", ctx, chatID, mock.Anything).Return(nil)

	uc := newUC(users, history, ai, 20)
	got, err := uc.HandleMessage(ctx, chatID, "hello there")

	require.NoError(t, err)
	require.Equal(t, "answer", got)
	ai.AssertExpectations(t)
}
