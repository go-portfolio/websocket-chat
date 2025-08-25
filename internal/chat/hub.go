package chat

import (
	"fmt"
	"sync"
	"time"
)


// ChatMessage представляет одно сообщение в чате
type ChatMessage struct {
	Type      string             `json:"type"`         // Тип сообщения: "system" или "message" или "private"
	From      string             `json:"from"`         // Отправитель: имя пользователя или "system"
	To        string             `json:"to,omitempty"` // Для приватных сообщений
	Text      string             `json:"text"`         // Текст сообщения
	Timestamp int64              `json:"timestamp"`    // Временная метка Unix
	Room      string             `join:"room"`
	Users     map[string]UserClient // 🔑 username → client
}

type Room struct {
	Name      string
	Clients   map[UserClient]bool
	Broadcast chan ChatMessage
	History   []ChatMessage
	Mu        sync.RWMutex
}

// Hub управляет подключениями, рассылкой сообщений и хранением истории
type Hub struct {
	Clients      map[UserClient]bool // Все активные клиенты
	Broadcast    chan ChatMessage     // Канал для отправки сообщений всем клиентам
	RegisterCh   chan UserClient      // Канал для регистрации нового клиента
	unregisterCh chan UserClient      // Канал для удаления клиента
	Rooms        map[string]*Room
	mu           sync.RWMutex // Мьютекс для защиты данных от гонок
}

func NewHub() *Hub {
	return &Hub{
		Clients:      make(map[UserClient]bool),
		Rooms:        make(map[string]*Room),
		Broadcast:    make(chan ChatMessage, 128),
		RegisterCh:   make(chan UserClient),
		unregisterCh: make(chan UserClient),
	}
}

// Run запускает главный цикл Hub
func (chatHub *Hub) Run() {
	for {
		select {
		case client := <-chatHub.RegisterCh:
			chatHub.mu.Lock()
			chatHub.Clients[client] = true
			chatHub.mu.Unlock()

			room := chatHub.GetRoom(client.GetRoomName())

			// Отправляем историю комнаты новому клиенту
			room.Mu.RLock()
			for _, msg := range room.History {
				client.SendMessage(msg)
			}
			room.Mu.RUnlock()

			room.Mu.Lock()
			room.Clients[client] = true
			room.Mu.Unlock()

			// Сообщаем остальным, что клиент присоединился
			room.Broadcast <- ChatMessage{
				Type:      "system",
				From:      client.GetUsername(),
				Room:      room.Name,
				Text:      fmt.Sprintf("присоединился к комнате %s", room.Name),
				Timestamp: time.Now().Unix(),
			}

		case client := <-chatHub.unregisterCh:
			chatHub.mu.Lock()
			if _, ok := chatHub.Clients[client]; ok {
				delete(chatHub.Clients, client)
				client.Close()
			}
			chatHub.mu.Unlock()

			room := chatHub.GetRoom(client.GetRoomName())

			room.Mu.Lock()
			delete(room.Clients, client)
			room.Mu.Unlock()

			// Сообщаем остальным, что клиент вышел
			room.Broadcast <- ChatMessage{
				Type:      "system",
				From:      client.GetUsername(),
				Room:      room.Name,
				Text:      fmt.Sprintf("покинул комнату %s", room.Name),
				Timestamp: time.Now().Unix(),
			}

		case msg := <-chatHub.Broadcast:
			chatHub.mu.Lock()
			if msg.To != "" {
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
				room.Broadcast <- msg
			}
			chatHub.mu.Unlock()
		}
	}
}

func (h *Hub) GetRoom(name string) *Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	if room, ok := h.Rooms[name]; ok {
		return room
	}
	room := &Room{
		Name:      name,
		Clients:   make(map[UserClient]bool),
		Broadcast: make(chan ChatMessage, 128),
		History:   make([]ChatMessage, 0, 50),
	}
	h.Rooms[name] = room
	go room.Run()
	return room
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
