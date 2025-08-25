package main

import (
	"log"
	"net/http"

	"github.com/joho/godotenv"
	"github.com/go-portfolio/websocket-chat/internal/app"
)

func main() {
	// Загружаем .env
	_ = godotenv.Load("../../.env")

	// Создаём приложение
	a := app.New()

	// Запуск сервера
	addr := ":8080"
	log.Printf("Server listening on %s", addr)
	if err := http.ListenAndServe(addr, a.Mux); err != nil {
		log.Fatal(err)
	}
}
