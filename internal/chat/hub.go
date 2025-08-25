package chat

import (
	"fmt"
	"sync"
	"time"
)

// ChatMessage представляет одно сообщение в чате
type ChatMessage struct {
	Type      string              `json:"type"`         // Тип: "system", "message", "private"
	From      string              `json:"from"`         // Отправитель
	To        string              `json:"to,omitempty"` // Адресат (для приватных)
	Text      string              `json:"text"`         // Текст
	Timestamp int64               `json:"timestamp"`    // Временная метка
	Room      string              `json:"room"`
	Users     map[string]UserClient // username → client
}

// ----------------------------
// Интерфейс для комнаты
// ----------------------------
type RoomManager interface {
	Run()                 // Запускает обработку сообщений
	OnlineUsers() []string // Возвращает список онлайн-юзеров
	AddClient(c UserClient)
	RemoveClient(c UserClient)
	BroadcastMessage(msg ChatMessage)
	GetName() string
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

		// сохраняем историю (не более 50)
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
	Clients      map[UserClient]bool // Все активные клиенты
	Broadcast    chan ChatMessage     // Канал общих сообщений
	RegisterCh   chan UserClient      // Регистрация клиента
	unregisterCh chan UserClient      // Удаление клиента
	Rooms        map[string]RoomManager
	mu           sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		Clients:      make(map[UserClient]bool),
		Rooms:        make(map[string]RoomManager),
		Broadcast:    make(chan ChatMessage, 128),
		RegisterCh:   make(chan UserClient),
		unregisterCh: make(chan UserClient),
	}
}

// Главный цикл Hub
func (chatHub *Hub) Run() {
	for {
		select {
		case client := <-chatHub.RegisterCh:
			chatHub.mu.Lock()
			chatHub.Clients[client] = true
			chatHub.mu.Unlock()

			room := chatHub.GetRoom(client.GetRoomName())

			// Отправляем историю новому клиенту
			if r, ok := room.(*Room); ok {
				r.Mu.RLock()
				for _, msg := range r.History {
					client.SendMessage(msg)
				}
				r.Mu.RUnlock()
			}

			room.AddClient(client)

			// Системное сообщение
			room.BroadcastMessage(ChatMessage{
				Type:      "system",
				From:      client.GetUsername(),
				Room:      room.GetName(),
				Text:      fmt.Sprintf("присоединился к комнате %s", room.GetName()),
				Timestamp: time.Now().Unix(),
			})

		case client := <-chatHub.unregisterCh:
			chatHub.mu.Lock()
			if _, ok := chatHub.Clients[client]; ok {
				delete(chatHub.Clients, client)
				client.Close()
			}
			chatHub.mu.Unlock()

			room := chatHub.GetRoom(client.GetRoomName())
			room.RemoveClient(client)

			room.BroadcastMessage(ChatMessage{
				Type:      "system",
				From:      client.GetUsername(),
				Room:      room.GetName(),
				Text:      fmt.Sprintf("покинул комнату %s", room.GetName()),
				Timestamp: time.Now().Unix(),
			})

		case msg := <-chatHub.Broadcast:
			chatHub.mu.Lock()
			if msg.To != "" {
				// приватное сообщение
				for client := range chatHub.Clients {
					if client.GetUsername() == msg.To || client.GetUsername() == msg.From {
						select {
						case client.PrivateChan() <- msg:
						default:
						}
					}
				}
				chatHub.mu.Unlock()
				continue
			}

			if room, ok := chatHub.Rooms[msg.Room]; ok {
				room.BroadcastMessage(msg)
			}
			chatHub.mu.Unlock()
		}
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

