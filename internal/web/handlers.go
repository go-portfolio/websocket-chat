package web

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-portfolio/websocket-chat/internal/auth"
	"github.com/go-portfolio/websocket-chat/internal/chat"
	"github.com/go-portfolio/websocket-chat/internal/user"

	"github.com/gorilla/websocket"
)

// =========================
// Контекст для хранения username
// =========================
type ctxKey string

const ctxUserKey ctxKey = "user" // Ключ для хранения имени пользователя в контексте запроса

// =========================
// Глобальные переменные (можно инжектировать в main.go)
// =========================
var (
	ChatHub    *chat.Hub   // Ссылка на Hub чата
	Users      *user.Store // Хранилище пользователей
	CookieName = "auth"    // Имя cookie для хранения JWT
)

// =========================
// Вспомогательная функция для JSON ответов
// =========================
func withJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
}

// =========================
// Регистрация пользователя
// POST /api/register
// тело JSON { "username": "...", "password": "..." }
// =========================
func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	withJSON(w)

	var cred user.Credentials
	// Декодируем JSON тело запроса
	if err := json.NewDecoder(r.Body).Decode(&cred); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid json"})
		return
	}

	// Регистрируем пользователя в Users
	if err := Users.Register(cred.Username, cred.Password); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

// =========================
// Логин пользователя
// POST /api/login
// тело JSON { "username": "...", "password": "..." }
// =========================
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	withJSON(w)

	var cred user.Credentials
	// Декодируем JSON тело запроса
	if err := json.NewDecoder(r.Body).Decode(&cred); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid json"})
		return
	}

	// Проверяем логин/пароль
	if !Users.Authenticate(cred.Username, cred.Password) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid credentials"})
		return
	}

	// Генерируем JWT
	token, err := auth.IssueJWT(cred.Username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to issue token"})
		return
	}

	// Устанавливаем cookie с токеном
	cookie := http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,  // недоступно JS
		Secure:   false, // на HTTPS ставить true
		SameSite: http.SameSiteLaxMode,
		MaxAge:   24 * 60 * 60, // 1 день
	}
	http.SetCookie(w, &cookie)

	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// =========================
// Middleware для проверки авторизации по cookie
// =========================
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Получаем cookie
		c, err := r.Cookie(CookieName)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing auth cookie"})
			return
		}

		// Парсим JWT
		userName, err := auth.ParseJWT(c.Value)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid token"})
			return
		}

		// Сохраняем username в контекст запроса
		ctx := context.WithValue(r.Context(), ctxUserKey, userName)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// =========================
// WebSocket обработчик
// /ws
// =========================
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Разрешаем соединения с любого источника (можно ограничить домен)
		return true
	},
}

func WSHandler(w http.ResponseWriter, r *http.Request) {
	// Получаем username из контекста
	username, _ := r.Context().Value(ctxUserKey).(string)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Обновляем HTTP-соединение до WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// Создаем клиента
	client := &chat.Client{
		Hub:      ChatHub,//Ссылка на центральный объект Hub
		Conn:     conn,//WebSocket-соединение между браузером и сервером
		Send:     make(chan chat.ChatMessage, 16),//Буферизированный канал для отправки сообщений клиенту
		CloseCh:  make(chan struct{}),//Канал для закрытия клиента
		Username: username,//Имя пользователя, которое пришло из JWT
	}

	// Регистрируем клиента в Hub
	ChatHub.Register <- client

	// Запускаем горутины для чтения и записи
	go client.WritePump()
	client.ReadPump()
}

// =========================
// Минимальный index handler
// =========================
func IndexHandler(w http.ResponseWriter, r *http.Request, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}
