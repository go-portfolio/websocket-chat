package web

import (
	"log"
	"net/http"

	"github.com/go-portfolio/websocket-chat/internal/chat"
	"github.com/go-portfolio/websocket-chat/internal/user"
	"github.com/gorilla/websocket"
)

// =========================
// Глобальные переменные (можно инжектировать в main.go)
// =========================
var (
	ChatHub    *chat.Hub   // Ссылка на Hub чата
	Users      user.UserStore // Хранилище пользователей
	CookieName = "auth"    // Имя cookie для хранения JWT
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// =========================
// ChatConnectionHandler
// =========================
func ChatConnectionHandler(w http.ResponseWriter, r *http.Request) {
	username, _ := r.Context().Value(ctxUserKey).(string)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	roomName := r.URL.Query().Get("room")
	if roomName == "" {
		roomName = "default"
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	room := ChatHub.GetRoom(roomName)
	client := chat.NewClient(ChatHub, room, conn, username)
	room.AddClient(client)
	ChatHub.RegisterCh <- client

	go client.WriteSocket()
	client.ReadSocket()
}
