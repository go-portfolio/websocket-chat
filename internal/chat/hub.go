package chat

import (
	"fmt"
	"sync"
	"time"
)

// ChatMessage представляет одно сообщение в чате
type ChatMessage struct {
	Type      string `json:"type"`      // Тип сообщения: "system" или "message"
	From      string `json:"from"`      // Отправитель: имя пользователя или "system"
	Text      string `json:"text"`      // Текст сообщения
	Timestamp int64  `json:"timestamp"` // Временная метка Unix
	Room      string `join:"room"`
}

type Room struct {
	Name      string
	Clients   map[*Client]bool
	Broadcast chan ChatMessage
	History   []ChatMessage
	Mu        sync.RWMutex
}

// Hub управляет подключениями, рассылкой сообщений и хранением истории
type Hub struct {
	Clients    map[*Client]bool // Все активные клиенты
	Broadcast  chan ChatMessage // Канал для отправки сообщений всем клиентам
	Register   chan *Client     // Канал для регистрации нового клиента
	unregister chan *Client     // Канал для удаления клиента
	Rooms      map[string]*Room
	mu         sync.RWMutex // Мьютекс для защиты данных от гонок

	history    []ChatMessage // История последних сообщений
	maxHistory int           // Максимальный размер истории
}

// NewHub создаёт и возвращает новый Hub
func NewHub() *Hub {
	return &Hub{
		Clients:    make(map[*Client]bool),
		Rooms:      make(map[string]*Room),
		Broadcast:  make(chan ChatMessage, 128), // Буфер канала для сообщений
		Register:   make(chan *Client),
		unregister: make(chan *Client),
		history:    make([]ChatMessage, 0, 50), // Начальная емкость истории
		maxHistory: 50,                         // Максимум последних сообщений
	}
}

// Run запускает главный цикл Hub, который обрабатывает регистрацию,
// удаление клиентов и рассылку сообщений
func (h *Hub) Run() {
	for {
		select {
		// Новый клиент подключился
		case c := <-h.Register:
			h.mu.Lock()
			h.Clients[c] = true
			h.mu.Unlock()

			room := h.GetRoom(c.Room.Name)

			// Отправляем историю комнаты новому клиенту
			room.Mu.RLock()
			// Отправляем историю сообщений новому клиенту
			for _, m := range room.History {
				c.Send <- m
			}
			room.Mu.RUnlock()

			room.Mu.Lock()
			room.Clients[c] = true
			room.Mu.Unlock()

			// Сообщаем остальным, что клиент присоединился
			room.Broadcast <- ChatMessage{
				Type:      "system",
				From:      c.Username,
				Room:      room.Name,
				Text:      fmt.Sprintf("присоединился к комнате %s", room.Name),
				Timestamp: time.Now().Unix(),
			}

		// Клиент отключился
		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.Clients[c]; ok {
				delete(h.Clients, c)
				close(c.CloseCh) // Закрываем канал клиента
			}
			h.mu.Unlock()

			room := h.GetRoom(c.Room.Name)

			room.Mu.Lock()
			delete(room.Clients, c) // удаляем клиента из комнаты
			room.Mu.Unlock()

			// Сообщаем остальным, что клиент вышел
			room.Broadcast <- ChatMessage{
				Type:      "system",
				From:      c.Username,
				Room:      room.Name,
				Text:      fmt.Sprintf("покинул комнату %s", room.Name),
				Timestamp: time.Now().Unix(),
			}

		// Получено новое сообщение для рассылки
		case msg := <-h.Broadcast:
			h.mu.Lock()	
			if room, ok := h.Rooms[msg.Room]; ok {
				room.Broadcast <- msg
			}
			h.mu.Unlock()
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
		Clients:   make(map[*Client]bool),
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
			case c.Send <- msg:
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
		users = append(users, c.Username)
	}
	return users
}
