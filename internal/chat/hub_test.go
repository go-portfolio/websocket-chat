package chat

import (
	"testing"
	"time"
)

type fakeConn interface {
    ReadMessage() (messageType int, p []byte, err error)
    WriteMessage(messageType int, data []byte) error
    Close() error
}

type fakeClient struct {
	Username     string
	Room         *Room
	PrivateChan  chan ChatMessage
	CloseCh      chan struct{}
	Conn     *fakeConn 
}

// helper: создать поддельного клиента
func newFakeClient(username string, room *Room) *fakeClient {
	return &fakeClient{
		Username:    username,
		Room:        room,
		PrivateChan: make(chan ChatMessage, 10),
		CloseCh:     make(chan struct{}),
	}
}

// Test: клиент успешно регистрируется и получает историю комнаты
func TestHub_RegisterClientAndReceiveHistory(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	room := hub.GetRoom("test-room")
	room.History = append(room.History, ChatMessage{
		From: "system", Text: "Welcome!", Timestamp: time.Now().Unix(),
	})

	client := newFakeClient("Alice", room)

	hub.RegisterCh <- (*Client)(client)

	select {
	case msg := <-client.PrivateChan:
		if msg.Text != "Welcome!" {
			t.Errorf("Ожидалось сообщение 'Welcome!', получено: %s", msg.Text)
		}
	case <-time.After(time.Second):
		t.Error("История не была получена клиентом")
	}
}

// Test: клиент успешно удаляется и его канал закрывается
func TestHub_UnregisterClient(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	room := hub.GetRoom("test-room")
	client := newFakeClient("Bob", room)

	hub.RegisterCh <- (*Client)(client)
	time.Sleep(100 * time.Millisecond) // Дать хабу время обработать

	hub.Unregister(client)

	select {
	case <-client.CloseCh:
		// OK
	case <-time.After(time.Second):
		t.Error("Канал клиента не был закрыт после удаления")
	}
}

// helper: вручную вызвать Unregister
func (h *Hub) Unregister(c *fakeClient) {
	h.UnregisterCh <- c
}

// Test: сообщение отправляется в комнату и принимается клиентами
func TestHub_BroadcastMessageToRoom(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	room := hub.GetRoom("test-room")
	client := newFakeClient("Charlie", room)
	hub.RegisterCh <- (*Client)(client)
	time.Sleep(100 * time.Millisecond)

	hub.Broadcast <- ChatMessage{
		From: "Charlie",
		Text: "Hello, room!",
		Room: "test-room",
	}

	select {
	case msg := <-client.PrivateChan:
		if msg.Text != "Hello, room!" {
			t.Errorf("Ожидалось сообщение 'Hello, room!', получено: %s", msg.Text)
		}
	case <-time.After(time.Second):
		t.Error("Сообщение не получено клиентом")
	}
}

// Test: приватное сообщение доставляется только отправителю и получателю
func TestHub_PrivateMessage(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	room := hub.GetRoom("test-room")

	sender := newFakeClient("Sender", room)
	receiver := newFakeClient("Receiver", room)
	other := newFakeClient("Other", room)

	hub.RegisterCh <- (*Client)(sender)
	hub.RegisterCh <- (*Client)(receiver)
	hub.RegisterCh <- (*Client)(other)

	time.Sleep(100 * time.Millisecond)

	hub.Broadcast <- ChatMessage{
		From: "Sender",
		To:   "Receiver",
		Text: "Private message",
	}

	checkReceived := func(c *fakeClient, expect bool) {
		select {
		case msg := <-c.PrivateChan:
			if !expect {
				t.Errorf("Клиент %s не должен был получить сообщение", c.Username)
			}
			if msg.Text != "Private message" {
				t.Errorf("Неверный текст сообщения: %s", msg.Text)
			}
		case <-time.After(300 * time.Millisecond):
			if expect {
				t.Errorf("Клиент %s должен был получить сообщение", c.Username)
			}
		}
	}

	checkReceived(sender, true)
	checkReceived(receiver, true)
	checkReceived(other, false)
}

// Test: создание новой комнаты и получение списка пользователей
func TestRoom_OnlineUsers(t *testing.T) {
	room := &Room{
		Name:    "test",
		Clients: make(map[*Client]bool),
	}

	c1 := &Client{Username: "user1"}
	c2 := &Client{Username: "user2"}

	room.Clients[c1] = true
	room.Clients[c2] = true

	users := room.OnlineUsers()

	if len(users) != 2 {
		t.Fatalf("Ожидалось 2 пользователя, получено: %d", len(users))
	}
	found := map[string]bool{}
	for _, u := range users {
		found[u] = true
	}

	if !found["user1"] || !found["user2"] {
		t.Error("Ожидались имена user1 и user2 в списке")
	}
}
