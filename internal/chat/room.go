package chat

import "sync"

// Room реализует RoomManager
type Room struct {
	Name      string
	Clients   map[UserClient]bool
	Broadcast chan ChatMessage
	History   []ChatMessage
	Mu        sync.RWMutex
}

func NewRoom(name string) *Room {
	return &Room{
		Name:      name,
		Clients:   make(map[UserClient]bool),
		Broadcast: make(chan ChatMessage, 128),
		History:   make([]ChatMessage, 0, 50),
	}
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

func (r *Room) AddClient(c UserClient) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	r.Clients[c] = true
}

func (r *Room) RemoveClient(c UserClient) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	delete(r.Clients, c)
}

func (r *Room) BroadcastMessage(msg ChatMessage) {
	r.Broadcast <- msg
}

func (r *Room) GetName() string {
	return r.Name
}
