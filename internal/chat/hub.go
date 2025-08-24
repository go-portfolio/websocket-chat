package chat

import (
	"sync"
	"time"
)

// ChatMessage представляет одно сообщение в чате
type ChatMessage struct {
	Type      string `json:"type"`      // Тип сообщения: "system" или "message"
	From      string `json:"from"`      // Отправитель: имя пользователя или "system"
	Text      string `json:"text"`      // Текст сообщения
	Timestamp int64  `json:"timestamp"` // Временная метка Unix
}

// Hub управляет подключениями, рассылкой сообщений и хранением истории
type Hub struct {
	clients    map[*Client]bool // Все активные клиенты
	broadcast  chan ChatMessage // Канал для отправки сообщений всем клиентам
	register   chan *Client     // Канал для регистрации нового клиента
	unregister chan *Client     // Канал для удаления клиента
	mu         sync.RWMutex     // Мьютекс для защиты данных от гонок

	history    []ChatMessage // История последних сообщений
	maxHistory int           // Максимальный размер истории
}

// NewHub создаёт и возвращает новый Hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan ChatMessage, 128), // Буфер канала для сообщений
		register:   make(chan *Client),
		unregister: make(chan *Client),
		history:    make([]ChatMessage, 0, 50), // Начальная емкость истории
		maxHistory: 50,                          // Максимум последних сообщений
	}
}

// Run запускает главный цикл Hub, который обрабатывает регистрацию,
// удаление клиентов и рассылку сообщений
func (h *Hub) Run() {
	for {
		select {
		// Новый клиент подключился
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = true
			h.mu.Unlock()

			// Отправляем историю сообщений новому клиенту
			for _, m := range h.history {
				c.send <- m
			}

			// Сообщаем остальным, что клиент присоединился
			h.broadcast <- ChatMessage{
				Type:      "system",
				From:      c.username,
				Text:      "joined the chat",
				Timestamp: time.Now().Unix(),
			}

		// Клиент отключился
		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.closeCh) // Закрываем канал клиента
			}
			h.mu.Unlock()

			// Сообщаем остальным, что клиент вышел
			h.broadcast <- ChatMessage{
				Type:      "system",
				From:      c.username,
				Text:      "left the chat",
				Timestamp: time.Now().Unix(),
			}

		// Получено новое сообщение для рассылки
		case msg := <-h.broadcast:
			h.mu.RLock()
			// Рассылаем сообщение всем активным клиентам
			for client := range h.clients {
				select {
				case client.send <- msg:
				default:
					// Если канал клиента заблокирован, пропускаем
				}
			}
			h.mu.RUnlock()

			// Добавляем сообщение в историю
			h.history = append(h.history, msg)
			if len(h.history) > h.maxHistory {
				// Обрезаем историю, если превысили maxHistory
				h.history = h.history[len(h.history)-h.maxHistory:]
			}
		}
	}
}
