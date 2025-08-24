package chat

import (
	"log"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Client представляет подключенного пользователя
type Client struct {
	Hub      *Hub            // Ссылка на Hub для регистрации/рассылки сообщений
	Conn     *websocket.Conn // WebSocket-соединение клиента
	Send     chan ChatMessage // Канал для отправки сообщений клиенту
	CloseCh  chan struct{}    // Канал для безопасного закрытия клиента
	Username string           // Имя пользователя
}

// readPump читает входящие сообщения от клиента и отправляет их в Hub
func (c *Client) ReadPump() {
	defer func() {
		// При завершении чтения удаляем клиента из Hub и закрываем соединение
		c.Hub.unregister <- c
		c.Conn.Close()
	}()

	// Настройка лимита и времени ожидания чтения
	c.Conn.SetReadLimit(512) // Максимальный размер сообщения
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error { // Обновление таймаута при получении PONG
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		var incoming struct {
			Text string `json:"text"` // Структура ожидаемого JSON-сообщения
		}
		// Читаем JSON-сообщение
		if err := c.Conn.ReadJSON(&incoming); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("read error: %v", err)
			}
			break // Завершаем цикл при ошибке
		}

		// Создаем ChatMessage для Hub
		msg := ChatMessage{
			Type:      "message",
			From:      c.Username,
			Text:      strings.TrimSpace(incoming.Text),
			Timestamp: time.Now().Unix(),
		}
		if msg.Text == "" {
			continue // Игнорируем пустые сообщения
		}

		// Отправляем сообщение в Hub для рассылки всем клиентам
		c.Hub.Broadcast <- msg
	}
}

// writePump отправляет сообщения из Hub клиенту и поддерживает heartbeat (PING)
func (c *Client) WritePump() {
	ticker := time.NewTicker(45 * time.Second) // Периодический PING для проверки соединения
	defer func() {
		ticker.Stop()
		c.Conn.Close() // Закрываем соединение при завершении
	}()

	for {
		select {
		case msg := <-c.Send:
			// Отправляем сообщение клиенту
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteJSON(msg); err != nil {
				return // Завершаем при ошибке записи
			}

		case <-ticker.C:
			// Отправляем PING каждые 45 секунд
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return // Завершаем при ошибке PING
			}

		case <-c.CloseCh:
			// Завершение работы при закрытии клиента
			return
		}
	}
}
