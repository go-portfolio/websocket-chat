package app

import (
	"log"
	"net/http"
	"os"

	"github.com/go-portfolio/websocket-chat/config"
	"github.com/go-portfolio/websocket-chat/internal/auth"
	"github.com/go-portfolio/websocket-chat/internal/chat"
	"github.com/go-portfolio/websocket-chat/internal/user"
	"github.com/go-portfolio/websocket-chat/internal/web"
)

type App struct {
	Mux *http.ServeMux
}

func New() *App {
	// Загружаем конфиг
	cfg := config.Load()

	// User store
	store, err := user.NewStore(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to init user store: %v", err)
	}

	// JWT secret
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-secret"
		log.Printf("[dev] JWT_SECRET not set, using default secret")
	}
	auth.InitSecret([]byte(secret))

	// ChatHub
	hub := chat.NewHub()
	go hub.Run()

	// Внутренние глобальные сервисы
	web.ChatHub = hub
	web.Users = store

	// Роуты
	mux := http.NewServeMux()
	mux.HandleFunc("/", web.IndexHandler)
	mux.HandleFunc("/api/register", web.RegisterHandler)
	mux.HandleFunc("/api/login", web.LoginHandler)
	mux.Handle("/ws", web.AuthMiddleware(http.HandlerFunc(web.ChatConnectionHandler)))
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("../../uploads"))))

	return &App{Mux: mux}
}
