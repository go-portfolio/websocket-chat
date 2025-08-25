package chat

import (
	"log"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketConn описывает минимальный набор методов websocket.Conn, которые нужны клиенту
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

// UserClient описывает поведение клиента в чате
type UserClient interface {
	GetUsername() string
	GetRoomName() string
	SendMessage(msg ChatMessage) error
	ReceivePrivateChan() <-chan ChatMessage
	Close() error
	PrivateChan() chan ChatMessage
}

// Client представляет подключенного пользователя
type Client struct {
	Hub         *Hub
	Room        *Room
	Conn        WebSocketConn    // Интерфейс вместо конкретного типа
	privateChan chan ChatMessage
	CloseCh     chan struct{}
	Username    string
}

func NewClient(hub *Hub, room *Room, conn WebSocketConn, username string) *Client {
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

// GetRoomName возвращает имя комнаты, к которой подключен клиент
func (c *Client) GetRoomName() string {
	if c.Room != nil {
		return c.Room.Name
	}
	return ""
}

// SendMessage отправляет сообщение клиенту (через PrivateChan)
func (c *Client) SendMessage(msg ChatMessage) error {
	select {
	case c.privateChan <- msg:
		return nil
	default:
		return nil // или ошибка "канал заполнен"
	}
}

// ReceivePrivateChan возвращает канал входящих сообщений
func (c *Client) ReceivePrivateChan() <-chan ChatMessage {
	return c.PrivateChan()
}

func (c *Client) PrivateChan() chan ChatMessage {
	return c.privateChan
}


// Close закрывает соединение и каналы
func (c *Client) Close() error {
	close(c.CloseCh)
	return c.Conn.Close()
}

// ReadSocket читает сообщения из WebSocket
func (client *Client) ReadSocket() {
	defer func() {
		client.Hub.unregisterCh <- client
		client.Conn.Close()
	}()

	client.Conn.SetReadLimit(512)
	client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	client.Conn.SetPongHandler(func(string) error {
		client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		var incoming struct {
			Text string `json:"text"`
			To   string `json:"to"`
			Type string `json:"type"`
		}

		if err := client.Conn.ReadJSON(&incoming); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("read error: %v", err)
			}
			break
		}

		msg := ChatMessage{
			Type:      strings.TrimSpace(incoming.Type),
			From:      client.Username,
			Text:      strings.TrimSpace(incoming.Text),
			To:        strings.TrimSpace(incoming.To),
			Room:      client.GetRoomName(),
			Timestamp: time.Now().Unix(),
		}

		if msg.Text == "" {
			continue
		}

		if msg.To != "" {
			msg.Type = "private"
			client.Hub.mu.RLock()
			for c := range client.Hub.Clients {
				if c.GetUsername() == msg.To || c.GetUsername() == msg.From {
					_ = c.SendMessage(msg)
				}
			}
			client.Hub.mu.RUnlock()
		} else {
			client.Room.Broadcast <- msg
		}
	}
}

// WriteSocket пишет сообщения из канала клиенту и посылает PING
func (client *Client) WriteSocket() {
	ticker := time.NewTicker(45 * time.Second)
	defer func() {
		ticker.Stop()
		client.Conn.Close()
	}()

	for {
		select {
		case msg := <-client.privateChan:
			client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.Conn.WriteJSON(msg); err != nil {
				return
			}

		case <-ticker.C:
			client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-client.CloseCh:
			return
		}
	}
}
