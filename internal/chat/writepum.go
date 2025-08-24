package chat

import (
	"github.com/gorilla/websocket"
	"time"
)

// writePump отправляет сообщения из Hub клиенту и поддерживает heartbeat (PING)
func (c *Client) WritePum() {
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
