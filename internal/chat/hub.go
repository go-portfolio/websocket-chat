package chat

import (
	"fmt"
	"sync"
	"time"
)

type Hub struct {
	Clients      map[UserClient]bool
	RegisterCh   chan UserClient
	unregisterCh chan UserClient
	Rooms        map[string]RoomManager
	mu           sync.RWMutex
	BroadcastCh  chan ChatMessage
}

func NewHub() *Hub {
	return &Hub{
		Clients:      make(map[UserClient]bool),
		Rooms:        make(map[string]RoomManager),
		BroadcastCh:  make(chan ChatMessage, 128),
		RegisterCh:   make(chan UserClient),
		unregisterCh: make(chan UserClient),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.RegisterCh:
			h.RegisterClient(client)
		case client := <-h.unregisterCh:
			h.UnregisterClient(client)
		case msg := <-h.BroadcastCh:
			h.Broadcast(msg)
		}
	}
}

func (h *Hub) RegisterClient(client UserClient) {
	h.mu.Lock()
	h.Clients[client] = true
	h.mu.Unlock()

	room := h.GetRoom(client.GetRoomName())

	// Отправка истории
	if r, ok := room.(*Room); ok {
		r.Mu.RLock()
		for _, msg := range r.History {
			client.SendMessage(msg)
		}
		r.Mu.RUnlock()
	}

	room.AddClient(client)
	room.BroadcastMessage(ChatMessage{
		Type:      "system",
		From:      client.GetUsername(),
		Room:      room.GetName(),
		Text:      fmt.Sprintf("присоединился к комнате %s", room.GetName()),
		Timestamp: time.Now().Unix(),
	})
}

func (h *Hub) UnregisterClient(client UserClient) {
	h.mu.Lock()
	if _, ok := h.Clients[client]; ok {
		delete(h.Clients, client)
		client.Close()
	}
	h.mu.Unlock()

	room := h.GetRoom(client.GetRoomName())
	room.RemoveClient(client)

	room.BroadcastMessage(ChatMessage{
		Type:      "system",
		From:      client.GetUsername(),
		Room:      room.GetName(),
		Text:      fmt.Sprintf("покинул комнату %s", room.GetName()),
		Timestamp: time.Now().Unix(),
	})
}

func (h *Hub) Broadcast(msg ChatMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if msg.To != "" {
		for client := range h.Clients {
			if client.GetUsername() == msg.To || client.GetUsername() == msg.From {
				select {
				case client.PrivateChan() <- msg:
				default:
				}
			}
		}
		return
	}

	if room, ok := h.Rooms[msg.Room]; ok {
		room.BroadcastMessage(msg)
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

func (h *Hub) GetClients() []UserClient {
	h.mu.RLock()
	defer h.mu.RUnlock()
	clients := make([]UserClient, 0, len(h.Clients))
	for c := range h.Clients {
		clients = append(clients, c)
	}
	return clients
}
