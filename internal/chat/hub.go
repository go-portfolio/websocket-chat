package chat

import (
	"fmt"
	"sync"
	"time"
)

// ----------------------------
// Сообщение чата
// ----------------------------
type ChatMessage struct {
	Type      string              `json:"type"`
	From      string              `json:"from"`
	To        string              `json:"to,omitempty"`
	Text      string              `json:"text"`
	Timestamp int64               `json:"timestamp"`
	Room      string              `json:"room"`
	Users     map[string]UserClient
}

// ----------------------------
// Интерфейс для комнаты
// ----------------------------
type RoomManager interface {
	Run()
	OnlineUsers() []string
	AddClient(c UserClient)
	RemoveClient(c UserClient)
	BroadcastMessage(msg ChatMessage)
	GetName() string
}

// ----------------------------
// Интерфейс для Hub
// ----------------------------
type HubManager interface {
	Run()
	RegisterClient(c UserClient)
	UnregisterClient(c UserClient)
	Broadcast(msg ChatMessage)
	GetRoom(name string) RoomManager
	GetClients() []UserClient
}

// ----------------------------
// Реализация комнаты
// ----------------------------
type Room struct {
	Name      string
	Clients   map[UserClient]bool
	Broadcast chan ChatMessage
	History   []ChatMessage
	Mu        sync.RWMutex
}

func NewRoom(name string) *Room {
	return &Room{
		Name:      name,
		Clients:   make(map[UserClient]bool),
		Broadcast: make(chan ChatMessage, 128),
		History:   make([]ChatMessage, 0, 50),
	}
}

func (r *Room) Run() {
	for msg := range r.Broadcast {
		r.Mu.RLock()
		for c := range r.Clients {
			select {
			case c.PrivateChan() <- msg:
			default:
			}
		}
		r.Mu.RUnlock()

		r.History = append(r.History, msg)
		if len(r.History) > 50 {
			r.History = r.History[len(r.History)-50:]
		}
	}
}

func (r *Room) OnlineUsers() []string {
	r.Mu.RLock()
	defer r.Mu.RUnlock()
	users := make([]string, 0, len(r.Clients))
	for c := range r.Clients {
		users = append(users, c.GetUsername())
	}
	return users
}

func (r *Room) AddClient(c UserClient) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	r.Clients[c] = true
}

func (r *Room) RemoveClient(c UserClient) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	delete(r.Clients, c)
}

func (r *Room) BroadcastMessage(msg ChatMessage) {
	r.Broadcast <- msg
}

func (r *Room) GetName() string {
	return r.Name
}

// ----------------------------
// Hub (менеджер чатов)
// ----------------------------
type Hub struct {
	Clients      map[UserClient]bool
	RegisterCh   chan UserClient
	unregisterCh chan UserClient
	Rooms        map[string]RoomManager
	mu           sync.RWMutex
	BroadcastCh chan ChatMessage 
}

func NewHub() *Hub {
	return &Hub{
		Clients:      make(map[UserClient]bool),
		Rooms:        make(map[string]RoomManager),
		BroadcastCh:    make(chan ChatMessage, 128),
		RegisterCh:   make(chan UserClient),
		unregisterCh: make(chan UserClient),
	}
}

// Реализация HubManager
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.RegisterCh:
			h.RegisterClient(client)
		case client := <-h.unregisterCh:
			h.UnregisterClient(client)
		case msg := <-h.BroadcastCh:
			h.Broadcast(msg)
		}
	}
}

func (h *Hub) RegisterClient(client UserClient) {
	h.mu.Lock()
	h.Clients[client] = true
	h.mu.Unlock()

	room := h.GetRoom(client.GetRoomName())

	// Отправка истории
	if r, ok := room.(*Room); ok {
		r.Mu.RLock()
		for _, msg := range r.History {
			client.SendMessage(msg)
		}
		r.Mu.RUnlock()
	}

	room.AddClient(client)

	room.BroadcastMessage(ChatMessage{
		Type:      "system",
		From:      client.GetUsername(),
		Room:      room.GetName(),
		Text:      fmt.Sprintf("присоединился к комнате %s", room.GetName()),
		Timestamp: time.Now().Unix(),
	})
}

func (h *Hub) UnregisterClient(client UserClient) {
	h.mu.Lock()
	if _, ok := h.Clients[client]; ok {
		delete(h.Clients, client)
		client.Close()
	}
	h.mu.Unlock()

	room := h.GetRoom(client.GetRoomName())
	room.RemoveClient(client)

	room.BroadcastMessage(ChatMessage{
		Type:      "system",
		From:      client.GetUsername(),
		Room:      room.GetName(),
		Text:      fmt.Sprintf("покинул комнату %s", room.GetName()),
		Timestamp: time.Now().Unix(),
	})
}

func (h *Hub) Broadcast(msg ChatMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if msg.To != "" {
		for client := range h.Clients {
			if client.GetUsername() == msg.To || client.GetUsername() == msg.From {
				select {
				case client.PrivateChan() <- msg:
				default:
				}
			}
		}
		return
	}

	if room, ok := h.Rooms[msg.Room]; ok {
		room.BroadcastMessage(msg)
	}
}

func (h *Hub) GetRoom(name string) RoomManager {
	h.mu.Lock()
	defer h.mu.Unlock()
	if room, ok := h.Rooms[name]; ok {
		return room
	}
	room := NewRoom(name)
	h.Rooms[name] = room
	go room.Run()
	return room
}

func (h *Hub) GetClients() []UserClient {
	h.mu.RLock()
	defer h.mu.RUnlock()
	clients := make([]UserClient, 0, len(h.Clients))
	for c := range h.Clients {
		clients = append(clients, c)
	}
	return clients
}
