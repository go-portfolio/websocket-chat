package main

import (
	"log"
	"net/http"
	"os"

	"github.com/go-portfolio/websocket-chat/internal/auth"
	"github.com/go-portfolio/websocket-chat/internal/chat"
	"github.com/go-portfolio/websocket-chat/internal/user"
	"github.com/go-portfolio/websocket-chat/internal/web"
	"github.com/go-portfolio/websocket-chat/config"

	"github.com/joho/godotenv"
)

func main() {
		// Локально читаем .env, в продакшене переменные берутся из окружения
	_ = godotenv.Load("../../.env")
	// Загружаем конфигурацию
	cfg := config.Load()

	// Создаём user store (Postgres)
	store, err := user.NewStore(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to init user store: %v", err)
	}
	defer store.Close() // не забудь закрыть соединение с БД

	// тест: регистрация
	if err := store.Register("alex", "12345"); err != nil {
		log.Println("register error:", err)
	} else {
		log.Println("user alex registered")
	}

	// тест: авторизация
	ok := store.Authenticate("alex", "12345")
	log.Println("auth success?", ok)
	
	// =========================
	// Инициализация глобальных сервисов
	// =========================
	web.ChatHub = chat.NewHub() // Создаем Hub чата для управления подключениями и рассылкой
	web.Users = store // Создаем хранилище пользователей

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
	mux.Handle("/ws", web.AuthMiddleware(http.HandlerFunc(web.ChatConnectionHandler)))

	// =========================
	// Запуск сервера
	// =========================
	addr := ":8080"
	log.Printf("Server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err) // Завершаем при ошибке запуска сервера
	}
}
