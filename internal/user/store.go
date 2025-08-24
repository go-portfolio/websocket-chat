package user

import (
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// Credentials — структура для логина/регистрации
type Credentials struct {
    Username string `json:"username"`
    Password string `json:"password"`
}


// Store — простое in-memory хранилище пользователей.
// Использует карту username -> passwordHash и мьютекс для безопасного доступа из разных горутин.
// ⚠️ В реальных приложениях вместо этого лучше использовать полноценную базу данных (Postgres/MySQL).
type Store struct {
	mu   sync.RWMutex      // RWMutex для конкурентного доступа (чтение/запись из разных горутин)
	data map[string]string // username -> bcrypt hash пароля
}

// NewStore создаёт и возвращает новый пустой Store.
func NewStore() *Store {
	return &Store{data: make(map[string]string)}
}

// Register регистрирует нового пользователя.
// 1. Проверяет, что логин и пароль не пустые.
// 2. Ограничивает длину логина (макс. 24 символа).
// 3. Блокирует Store на запись, чтобы избежать гонок.
// 4. Проверяет, что пользователь с таким именем ещё не существует.
// 5. Хэширует пароль с помощью bcrypt и сохраняет его в Store.
// Возвращает ошибку, если имя занято, некорректное или при проблемах с хэшированием.
func (store *Store) Register(username, password string) error {
	// Убираем пробелы в начале/конце
	username = strings.TrimSpace(username)

	// Проверяем, что логин и пароль заданы
	if username == "" || password == "" {
		return fmt.Errorf("username and password are required")
	}

	// Ограничиваем максимальную длину логина
	if len(username) > 24 {
		return fmt.Errorf("username too long (max 24)")
	}

	// Блокируем Store для записи
	store.mu.Lock()
	defer store.mu.Unlock()

	// Проверяем, что пользователя с таким именем ещё нет
	if _, exists := store.data[username]; exists {
		return fmt.Errorf("username already exists")
	}

	// Хэшируем пароль
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Сохраняем хэш в хранилище
	store.data[username] = string(hash)
	return nil
}

// Authenticate проверяет логин и пароль пользователя.
// 1. Ищет пользователя в Store по имени.
// 2. Если не найден — возвращает false.
// 3. Сравнивает переданный пароль с сохранённым bcrypt-хэшем.
// Возвращает true, если пароль совпадает, иначе false.
func (store *Store) Authenticate(username, password string) bool {
	// Блокируем Store для чтения (несколько горутин могут читать параллельно)
	store.mu.RLock()
	hash, ok := store.data[username]
	store.mu.RUnlock()

	// Если пользователя нет
	if !ok {
		return false
	}

	// Сравниваем пароль с хэшем
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
