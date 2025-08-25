package chat

import (
	"log"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Client представляет подключенного пользователя
type Client struct {
	Hub         *Hub
	Room        RoomManager
	Conn        WebSocketConn
	privateChan chan ChatMessage
	CloseCh     chan struct{}
	Username    string
}

// NewClient создаёт нового клиента
func NewClient(hub *Hub, room RoomManager, conn WebSocketConn, username string) *Client {
	return &Client{
		Hub:         hub,
		Room:        room,
		Conn:        conn,
		privateChan: make(chan ChatMessage, 16),
		CloseCh:     make(chan struct{}),
		Username:    username,
	}
}

// GetUsername возвращает имя пользователя
func (c *Client) GetUsername() string {
	return c.Username
}

// GetRoomName возвращает имя комнаты через интерфейс RoomManager
func (c *Client) GetRoomName() string {
	if c.Room != nil {
		return c.Room.GetName()
	}
	return ""
}

// SendMessage отправляет сообщение в приватный канал
func (c *Client) SendMessage(msg ChatMessage) error {
	select {
	case c.privateChan <- msg:
		return nil
	default:
		return nil // или можно вернуть ошибку "канал заполнен"
	}
}

// ReceivePrivateChan возвращает канал входящих сообщений
func (c *Client) ReceivePrivateChan() <-chan ChatMessage {
	return c.PrivateChan()
}

// PrivateChan возвращает внутренний канал сообщений
func (c *Client) PrivateChan() chan ChatMessage {
	return c.privateChan
}

// Close закрывает соединение и каналы
func (c *Client) Close() error {
	close(c.CloseCh)
	return c.Conn.Close()
}

// ReadSocket читает сообщения из WebSocket
func (c *Client) ReadSocket() {
	defer func() {
		c.Hub.unregisterCh <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(512)
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		var incoming struct {
			Text string `json:"text"`
			To   string `json:"to"`
			Type string `json:"type"`
		}

		if err := c.Conn.ReadJSON(&incoming); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("read error: %v", err)
			}
			break
		}

		msg := ChatMessage{
			Type:      strings.TrimSpace(incoming.Type),
			From:      c.Username,
			Text:      strings.TrimSpace(incoming.Text),
			To:        strings.TrimSpace(incoming.To),
			Room:      c.GetRoomName(),
			Timestamp: time.Now().Unix(),
		}

		if msg.Text == "" {
			continue
		}

		if msg.To != "" {
			msg.Type = "private"
			c.Hub.mu.RLock()
			for client := range c.Hub.Clients {
				if client.GetUsername() == msg.To || client.GetUsername() == msg.From {
					_ = client.SendMessage(msg)
				}
			}
			c.Hub.mu.RUnlock()
		} else {
			c.Room.BroadcastMessage(msg)
		}
	}
}

// WriteSocket пишет сообщения из канала клиенту и отправляет PING
func (c *Client) WriteSocket() {
	ticker := time.NewTicker(45 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case msg := <-c.privateChan:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteJSON(msg); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-c.CloseCh:
			return
		}
	}
}
