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
	Room     *Room
	Conn     *websocket.Conn // WebSocket-соединение клиента
	Send     chan ChatMessage // Канал для отправки сообщений клиенту
	CloseCh  chan struct{}    // Канал для безопасного закрытия клиента
	Username string           // Имя пользователя
}

// ReadSocket читает входящие сообщения от клиента и отправляет их в Hub
func (client *Client) ReadSocket() {
	defer func() {
		// При завершении чтения удаляем клиента из Hub и закрываем соединение
		client.Hub.unregisterCh <- client
		client.Conn.Close()
	}()

	// Настройка лимита и времени ожидания чтения
	client.Conn.SetReadLimit(512) // Максимальный размер сообщения
	client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	client.Conn.SetPongHandler(func(string) error { // Обновление таймаута при получении PONG
		client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		var incoming struct {
			Text string `json:"text"` // Структура ожидаемого JSON-сообщения
		}
		// Читаем JSON-сообщение
		if err := client.Conn.ReadJSON(&incoming); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("read error: %v", err)
			}
			break // Завершаем цикл при ошибке
		}

		// Создаем ChatMessage для Hub
		msg := ChatMessage{
			Type:      "message",
			From:      client.Username,
			Text:      strings.TrimSpace(incoming.Text),
			Room:      client.Room.Name,
			Timestamp: time.Now().Unix(),
		}
		
		if msg.Text == "" {
			continue // Игнорируем пустые сообщения
		}

		// Отправляем сообщение в Hub для рассылки в комнату
		client.Room.Broadcast <- msg
	}
}

// WriteSocket отправляет сообщения из Hub клиенту и поддерживает heartbeat (PING)
func (client *Client) WriteSocket() {
	ticker := time.NewTicker(45 * time.Second) // Периодический PING для проверки соединения
	defer func() {
		ticker.Stop()
		client.Conn.Close() // Закрываем соединение при завершении
	}()

	for {
		select {
		case msg := <-client.Send:
			// Отправляем сообщение клиенту
			client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.Conn.WriteJSON(msg); err != nil {
				return // Завершаем при ошибке записи
			}

		case <-ticker.C:
			// Отправляем PING каждые 45 секунд
			client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return // Завершаем при ошибке PING
			}

		case <-client.CloseCh:
			// Завершение работы при закрытии клиента
			return
		}
	}
}
