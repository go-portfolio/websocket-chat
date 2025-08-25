package chat

import "time"

// ChatMessage представляет одно сообщение
type ChatMessage struct {
	Type      string              `json:"type"`
	From      string              `json:"from"`
	To        string              `json:"to,omitempty"`
	Text      string              `json:"text"`
	Timestamp int64               `json:"timestamp"`
	Room      string              `json:"room"`
	Users     map[string]UserClient
}

// Интерфейс для клиента чата
type UserClient interface {
	GetUsername() string
	GetRoomName() string
	SendMessage(msg ChatMessage) error
	ReceivePrivateChan() <-chan ChatMessage
	Close() error
	PrivateChan() chan ChatMessage
}

// Минимальные методы websocket.Conn, которые нужны клиенту
type WebSocketConn interface {
	ReadJSON(v interface{}) error
	WriteJSON(v interface{}) error
	SetReadLimit(limit int64)
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	SetPongHandler(h func(string) error)
	WriteMessage(messageType int, data []byte) error
	Close() error
}

// Интерфейс для комнаты
type RoomManager interface {
	Run()
	OnlineUsers() []string
	AddClient(c UserClient)
	RemoveClient(c UserClient)
	BroadcastMessage(msg ChatMessage)
	GetName() string
}

// Интерфейс для Hub
type HubManager interface {
	Run()
	RegisterClient(c UserClient)
	UnregisterClient(c UserClient)
	Broadcast(msg ChatMessage)
	GetRoom(name string) RoomManager
	GetClients() []UserClient
}
