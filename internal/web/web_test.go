package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/go-portfolio/websocket-chat/internal/auth"

	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
)

/*
	Тестовый мок для user.UserStore.

	Почему нужен мок:
	- handlers используют глобальную переменную Users
	- для unit-тестов не хотим дергать реальную БД
	- поэтому создаём лёгкий in-memory store
	- имитируем поведение реального Store: Register, Authenticate, GetAvatar, Close

	Важно:
	- мок повторяет видимую логику ошибок реального Store
	  (например, "username already exists"), чтобы тесты были релевантными
	  и handlers корректно передавали ошибки дальше.
*/

// mockUser хранит bcrypt-хэш пароля и имя файла аватара
type mockUser struct {
	hash   []byte // bcrypt-хэш пароля
	avatar string // путь/имя аватара
}

// mockUserStore — in-memory хранилище пользователей с защитой от конкурентного доступа
type mockUserStore struct {
	mu    sync.Mutex            // защита от параллельного доступа
	users map[string]mockUser   // ключ: username, значение: mockUser
}

// newMockUserStore создаёт новый in-memory store
func newMockUserStore() *mockUserStore {
	return &mockUserStore{
		users: make(map[string]mockUser),
	}
}

// Register эмулирует регистрацию пользователя
// - проверяет пустые поля и длину username
// - возвращает "username already exists", если пользователь уже есть
// - хеширует пароль через bcrypt для последующей проверки в Authenticate
func (m *mockUserStore) Register(username, password, avatar string) error {
	username = strings.TrimSpace(username) // убираем пробелы с краёв
	if username == "" || password == "" {
		return fmt.Errorf("username and password are required")
	}
	if len(username) > 24 {
		return fmt.Errorf("username too long (max 24)")
	}

	m.mu.Lock()         // блокировка для безопасного доступа к карте users
	defer m.mu.Unlock() // разблокировка в конце функции

	if _, ok := m.users[username]; ok {
		return fmt.Errorf("username already exists")
	}

	// генерируем bcrypt-хэш пароля
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// сохраняем нового пользователя в карту
	m.users[username] = mockUser{
		hash:   h,
		avatar: avatar,
	}
	return nil
}

// Authenticate проверяет соответствие введённого пароля сохранённому bcrypt-хэшу
// Возвращает true только при корректном username+password
func (m *mockUserStore) Authenticate(username, password string) bool {
	m.mu.Lock()
	u, ok := m.users[username]
	m.mu.Unlock()
	if !ok { // пользователь не найден
		return false
	}
	return bcrypt.CompareHashAndPassword(u.hash, []byte(password)) == nil
}

// GetAvatar возвращает avatar пользователя или пустую строку, если пользователь не найден
func (m *mockUserStore) GetAvatar(username string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.users[username]; ok {
		return u.avatar
	}
	return ""
}

// Close — пустая реализация для совместимости с интерфейсом
func (m *mockUserStore) Close() error { return nil }

/*
---------------------------------------------------------
	Вспомогательная функция для формирования multipart/form-data
	(используется для тестов RegisterHandler)
	Возвращает body + contentType (boundary)
	Если fileName == "", multipart состоит только из текстовых полей
---------------------------------------------------------
*/
func createMultipartForm(t *testing.T, fields map[string]string, fileField, fileName string, fileContent []byte) (*bytes.Buffer, string) {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)

	// Добавляем текстовые поля
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("write field: %v", err) // прерываем тест при ошибке
		}
	}

	// Если есть файл, добавляем его
	if fileField != "" && fileName != "" {
		part, err := w.CreateFormFile(fileField, fileName)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := part.Write(fileContent); err != nil {
			t.Fatalf("write file content: %v", err)
		}
	}

	// Закрываем writer и получаем boundary
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return body, w.FormDataContentType()
}

/* ==========================
   ТЕСТЫ RegisterHandler
   ========================== */

// Успешная регистрация без аватара
func TestRegisterHandler_Success_NoAvatar(t *testing.T) {
	Users = newMockUserStore() // подставляем мок вместо реальной БД

	// Формируем multipart без файла (только username и password)
	body, contentType := createMultipartForm(t,
		map[string]string{"username": "alice", "password": "12345"},
		"", "", nil,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/register", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()

	RegisterHandler(rr, req) // вызов тестируемого handler-а

	// Проверяем HTTP статус и JSON ответ
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]string
	err := json.NewDecoder(rr.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.Equal(t, "registered", resp["status"])

	// Проверяем, что пользователь действительно добавлен в мок
	assert.True(t, Users.Authenticate("alice", "12345"))
}

// Некорректный form-data (ошибка ParseMultipartForm)
func TestRegisterHandler_InvalidForm(t *testing.T) {
	Users = newMockUserStore() // Users не нужен для этого теста, но не должен паниковать

	req := httptest.NewRequest(http.MethodPost, "/api/register", strings.NewReader("not a multipart"))
	rr := httptest.NewRecorder()

	RegisterHandler(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "invalid form data")
}

// Повторная регистрация (duplicate username)
func TestRegisterHandler_DuplicateUser(t *testing.T) {
	Users = newMockUserStore()
	err := Users.Register("bob", "pass", "")
	assert.NoError(t, err) // пользователь успешно зарегистрирован

	// Попытка зарегистрировать того же пользователя снова
	body, contentType := createMultipartForm(t,
		map[string]string{"username": "bob", "password": "pass"},
		"", "", nil,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/register", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()

	RegisterHandler(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "username already exists")
}

/* ==========================
   ТЕСТЫ LoginHandler
   ========================== */

// Успешный логин с корректными данными
func TestLoginHandler_Success(t *testing.T) {
	Users = newMockUserStore()
	_ = Users.Register("john", "secret", "avatar.png") // создаём тестового пользователя

	body := `{"username":"john","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	rr := httptest.NewRecorder()

	LoginHandler(rr, req) // вызываем handler

	// Проверяем статус и JSON ответ
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]string
	err := json.NewDecoder(rr.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.Equal(t, "ok", resp["status"])
	assert.Equal(t, "avatar.png", resp["avatar"]) // проверяем аватар

	// Проверяем установку cookie авторизации
	cookies := rr.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == CookieName {
			found = true
			break
		}
	}
	assert.True(t, found, "должна быть установлена cookie авторизации")
}

// Некорректный JSON
func TestLoginHandler_InvalidJSON(t *testing.T) {
	Users = newMockUserStore() // неважно
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader("bad json"))
	rr := httptest.NewRecorder()

	LoginHandler(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "invalid json")
}

// Логин с неверным паролем
func TestLoginHandler_BadCredentials(t *testing.T) {
	Users = newMockUserStore()
	_ = Users.Register("john", "secret", "")

	body := `{"username":"john","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	rr := httptest.NewRecorder()

	LoginHandler(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "invalid credentials")
}

/* ==========================
   ТЕСТЫ IndexHandler
   ========================== */

// Проверка успешного чтения index.html
func TestIndexHandler_Success(t *testing.T) {
	// Создаём директорию и файл, которые читает handler
	_ = os.MkdirAll("../../internal/web", 0o755)
	const idxPath = "../../internal/web/index.html"
	_ = os.WriteFile(idxPath, []byte("<html>OK</html>"), 0o644)
	defer os.Remove(idxPath)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	IndexHandler(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "OK")
	assert.Equal(t, "text/html; charset=utf-8", rr.Header().Get("Content-Type"))
}

// Если index.html нет — handler возвращает 500 и сообщение об ошибке
func TestIndexHandler_NotFound(t *testing.T) {
	_ = os.Remove("../../internal/web/index.html") // удаляем файл, если есть

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	IndexHandler(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "index.html not found")
}

/* ==========================
   ТЕСТЫ AuthMiddleware
   ========================== */

// Корректная cookie авторизации
func TestAuthMiddleware_Success(t *testing.T) {
	token, err := auth.IssueJWT("alice")
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	rr := httptest.NewRecorder()

	// Downstream handler считывает username из контекста
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		un, _ := r.Context().Value(ctxUserKey).(string)
		_, _ = w.Write([]byte(un))
	}))

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "alice", rr.Body.String())
}

// Отсутствие cookie авторизации
func TestAuthMiddleware_MissingCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rr := httptest.NewRecorder()

	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "missing auth cookie")
}

// Некорректный JWT токен
func TestAuthMiddleware_InvalidToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "badtoken"})
	rr := httptest.NewRecorder()

	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "invalid token")
}
