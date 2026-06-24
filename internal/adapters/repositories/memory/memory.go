package memory

import (
	"context"
	"sync"

	"bratok/internal/domain/entities"
	"bratok/internal/interfaces"
)

var (
	_ interfaces.UserRepository        = (*Store)(nil)
	_ interfaces.ChatHistoryRepository = (*Store)(nil)
)

// userState — данные одного чата: роль, флаг ожидания роли и история.
type userState struct {
	role         string
	awaitingRole bool
	messages     []entities.Message
}

// Store реализует оба репозитория поверх map под RWMutex.
type Store struct {
	mu    sync.RWMutex
	keep  int
	users map[int64]*userState
}

// New создаёт хранилище; keep ограничивает число хранимых сообщений на чат.
func New(keep int) *Store {
	return &Store{keep: keep, users: make(map[int64]*userState)}
}

// stateLocked возвращает или создаёт состояние чата (вызывать под write-локом).
func (s *Store) stateLocked(chatID int64) *userState {
	st, ok := s.users[chatID]
	if !ok {
		st = &userState{}
		s.users[chatID] = st
	}
	return st
}

// Get возвращает состояние чата; для неизвестного — User с пустой ролью.
func (s *Store) Get(_ context.Context, chatID int64) (entities.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	st, ok := s.users[chatID]
	if !ok {
		return entities.User{ChatID: chatID}, nil
	}
	return entities.User{ChatID: chatID, Role: st.role, AwaitingRole: st.awaitingRole}, nil
}

// Save обновляет роль и флаг ожидания, не трогая историю чата.
func (s *Store) Save(_ context.Context, user entities.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.stateLocked(user.ChatID)
	st.role = user.Role
	st.awaitingRole = user.AwaitingRole
	return nil
}

// Append добавляет сообщение и обрезает историю до лимита keep.
func (s *Store) Append(_ context.Context, chatID int64, msg entities.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.stateLocked(chatID)
	st.messages = append(st.messages, msg)
	if s.keep > 0 && len(st.messages) > s.keep {
		trimmed := make([]entities.Message, s.keep)
		copy(trimmed, st.messages[len(st.messages)-s.keep:])
		st.messages = trimmed
	}
	return nil
}

// LastN возвращает копию до n последних сообщений (копия — против гонок).
func (s *Store) LastN(_ context.Context, chatID int64, n int) ([]entities.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	st, ok := s.users[chatID]
	if !ok || len(st.messages) == 0 {
		return nil, nil
	}
	msgs := st.messages
	if n > 0 && len(msgs) > n {
		msgs = msgs[len(msgs)-n:]
	}
	out := make([]entities.Message, len(msgs))
	copy(out, msgs)
	return out, nil
}

// Clear удаляет историю чата, сохраняя роль.
func (s *Store) Clear(_ context.Context, chatID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if st, ok := s.users[chatID]; ok {
		st.messages = nil
	}
	return nil
}
