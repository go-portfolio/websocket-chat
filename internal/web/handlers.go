package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

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
	  // Парсим multipart/form-data
    err := r.ParseMultipartForm(10 << 20) // 10MB лимит
    if err != nil {
        http.Error(w, "invalid form data", http.StatusBadRequest)
        return
    }

	var cred user.Credentials
    cred.Username = r.FormValue("username")
    cred.Password = r.FormValue("password")

    file, handler, err := r.FormFile("avatar")
    if err == nil {
        defer file.Close()
        log.Printf("Uploaded File: %+v, Size: %d", handler.Filename, handler.Size)
        // тут можно сохранить файл
    }

	var avatarURL string

	if err == nil { // файл передан
		defer file.Close()

		// создаём папку, если нет
		os.MkdirAll("../../uploads", os.ModePerm)

		// уникальное имя
		filename := fmt.Sprintf("../../uploads/%d_%s", time.Now().Unix(), handler.Filename)
		dst, err := os.Create(filename)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "cannot save file"})
			return
		}
		defer dst.Close()

		if _, err = io.Copy(dst, file); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "cannot write file"})
			return
		}

		avatarURL = fmt.Sprintf("/uploads/%d_%s", time.Now().Unix(), handler.Filename)
	}

	// Регистрируем пользователя в Users
	if err := Users.Register(cred.Username, cred.Password, avatarURL); err != nil {
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

	var avatar = Users.GetAvatar(cred.Username)


	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "avatar": avatar})
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

// ChatConnectionHandler Обработка сообщений сокета
func ChatConnectionHandler(w http.ResponseWriter, r *http.Request) {
	// Получаем username из контекста
	username, _ := r.Context().Value(ctxUserKey).(string)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	roomName := r.URL.Query().Get("room")
	if roomName == "" {
		roomName = "default"
	}

	// Обновляем HTTP-соединение до WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// Создаем клиента
	room := ChatHub.GetRoom(roomName)
	client := &chat.Client{
		Hub:         ChatHub, //Ссылка на центральный объект Hub
		Room:        room,
		Conn:        conn,                            //WebSocket-соединение между браузером и сервером
		PrivateChan: make(chan chat.ChatMessage, 16), //Буферизированный канал для отправки сообщений клиенту
		CloseCh:     make(chan struct{}),             //Канал для закрытия клиента
		Username:    username,                        //Имя пользователя, которое пришло из JWT
	}
	room.Mu.Lock()
	room.Clients[client] = true
	room.Mu.Unlock()

	// Регистрируем клиента в Hub
	ChatHub.RegisterCh <- client

	// Запускаем горутины для чтения и записи
	go client.WriteSocket()
	client.ReadSocket()
}

// IndexHandler читает HTML из файла и отдаёт клиенту
func IndexHandler(w http.ResponseWriter, r *http.Request) {
	// Определяем путь к index.html
	path := filepath.Join("..", "..", "internal", "web", "index.html")
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "index.html not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}
