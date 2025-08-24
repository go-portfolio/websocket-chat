package main

import (
	"log"
	"net/http"
	"os"

	"github.com/go-portfolio/websocket-chat/internal/auth"
	"github.com/go-portfolio/websocket-chat/internal/chat"
	"github.com/go-portfolio/websocket-chat/internal/user"
	"github.com/go-portfolio/websocket-chat/internal/web"
)

// Минимальный фронт (можно вынести в отдельный файл web/index.html)
const indexHTML = `<!doctype html>
<html lang="en">
<head><meta charset="UTF-8"><title>Go WebSocket Chat</title></head>
<body>
<h1>Go WebSocket Chat</h1>
<p>Use browser console or build a JS client to test /ws</p>
</body></html>`

func main() {
	// =========================
	// Инициализация глобальных сервисов
	// =========================
	web.ChatHub = chat.NewHub() // Создаем Hub чата для управления подключениями и рассылкой
	web.Users = user.NewStore() // Создаем хранилище пользователей (in-memory)

	// Инициализация JWT-секрета
	secret := os.Getenv("JWT_SECRET") // Берем из переменной окружения
	if secret == "" {
		secret = "dev-secret" // Для локальной разработки используем дефолт
		log.Printf("[dev] JWT_SECRET not set, using default secret")
	}
	auth.InitSecret([]byte(secret)) // Инициализируем секрет в модуле auth

	// =========================
	// Запуск Hub
	// =========================
	go web.ChatHub.Run() // Главный цикл Hub работает в отдельной горутине

	// =========================
	// Настройка HTTP маршрутов
	// =========================
	mux := http.NewServeMux()

	// Статический фронт (главная страница)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		web.IndexHandler(w, r)
	})

	// API маршруты
	mux.HandleFunc("/api/register", web.RegisterHandler) // Регистрация
	mux.HandleFunc("/api/login", web.LoginHandler)       // Логин

	// WebSocket маршрут с авторизацией через middleware
	mux.Handle("/ws", web.AuthMiddleware(http.HandlerFunc(web.WSHandler)))

	// =========================
	// Запуск сервера
	// =========================
	addr := ":8080"
	log.Printf("Server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err) // Завершаем при ошибке запуска сервера
	}
}
