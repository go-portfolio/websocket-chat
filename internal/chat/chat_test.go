package chat_test

import (
	"errors"
	"testing"
	"time"

	"github.com/go-portfolio/websocket-chat/internal/chat"
	"github.com/stretchr/testify/assert"
)


// mockClient — минимальная фейковая реализация интерфейса chat.UserClient.
type mockClient struct {
	username string
	room     string
	messages []chat.ChatMessage
	ch       chan chat.ChatMessage
	closed   bool
}

// newMockClient — удобный конструктор для тестов.
// Буфер канала = 10, чтобы избежать лишних блокировок в гонках.
func newMockClient(username, room string) *mockClient {
	return &mockClient{
		username: username,
		room:     room,
		ch:       make(chan chat.ChatMessage, 10),
	}
}

// Ниже — реализация методов интерфейса chat.UserClient.
// Каждый метод максимально прост и предсказуем, чтобы фокус тестов
// оставался на проверяемой логике Room/Hub/Client.

func (m *mockClient) GetUsername() string { return m.username }

func (m *mockClient) GetRoomName() string { return m.room }

// SendMessage имитирует асинхронную доставку сообщения в клиент.
// В реальности chat.Client кладёт сообщение в собственный приватный канал.
// Здесь мы просто накапливаем сообщения в слайсе для последующей проверки.
func (m *mockClient) SendMessage(msg chat.ChatMessage) error {
	m.messages = append(m.messages, msg)
	return nil
}

// ReceivePrivateChan возвращает канал для чтения входящих сообщений.
// В реальном клиенте это — read-only представление privateChan.
func (m *mockClient) ReceivePrivateChan() <-chan chat.ChatMessage { return m.ch }

// Close помечает клиента как закрытого. Это позволяет тесту проверить,
// что Hub корректно вызывает Close() при UnregisterClient().
func (m *mockClient) Close() error {
	m.closed = true
	return nil
}

// PrivateChan возвращает "внутренний" канал клиента (куда Room/Hub пишут).
func (m *mockClient) PrivateChan() chan chat.ChatMessage { return m.ch }

// --- Тесты Room --------------------------------------------------------------

// TestRoom_AddRemoveClient
// Цель: убедиться, что комната корректно обновляет список онлайн-пользователей
// при добавлении и удалении клиента.
func TestRoom_AddRemoveClient(t *testing.T) {
	room := chat.NewRoom("test")
	client := newMockClient("alice", "test")

	// Добавляем клиента и проверяем, что он появился в списке online.
	room.AddClient(client)
	assert.Len(t, room.OnlineUsers(), 1, "после AddClient должен быть 1 пользователь")

	// Удаляем клиента и проверяем, что список снова пуст.
	room.RemoveClient(client)
	assert.Len(t, room.OnlineUsers(), 0, "после RemoveClient список должен опустеть")
}

// TestRoom_BroadcastMessage
// Цель: проверить, что широковещательная рассылка через room.BroadcastMessage
// реально кладёт сообщения в приватные каналы подключённых клиентов.
//
// Детали синхронизации:
//   - Room.Run() должен выполняться в отдельной горутине: он читает из
//     room.Broadcast и доставляет сообщения клиентам + пишет в History.
//   - В конце теста закрываем room.Broadcast, чтобы корректно завершить Run().
func TestRoom_BroadcastMessage(t *testing.T) {
	room := chat.NewRoom("test")
	client := newMockClient("alice", "test")
	room.AddClient(client)

	// Запускаем "сервис" комнаты, который будет слушать канал Broadcast.
	go room.Run()
	// Закрываем канал в конце, чтобы Run() завершился (иначе горутина "повиснет").
	defer close(room.Broadcast)

	msg := chat.ChatMessage{From: "system", Text: "hello", Room: "test"}
	room.BroadcastMessage(msg)

	// Поскольку доставка асинхронная, используем select с таймаутом, чтобы
	// тест никогда не завис насмерть (хорошая практика для каналов).
	select {
	case got := <-client.ch:
		assert.Equal(t, msg.Text, got.Text, "клиент должен получить отправленный текст")
	case <-time.After(time.Second):
		t.Fatal("не пришло сообщение в клиент: вероятна проблема в Room.Run/BroadcastMessage")
	}
}

// TestRoom_HistoryLimit
// Цель: убедиться, что история сообщений комнаты "обрезается" и хранит
// не более 50 последних сообщений (по коду — len>50 => берутся последние 50).
//
// Подход:
//   - Запускаем Room.Run();
//   - Отправляем 60 сообщений (превышая лимит);
//   - Даём краткую паузу, чтобы горутина успела обработать записи;
//   - Проверяем размер History.
func TestRoom_HistoryLimit(t *testing.T) {
	room := chat.NewRoom("test")

	go room.Run()
	defer close(room.Broadcast)

	for i := 0; i < 60; i++ {
		room.BroadcastMessage(chat.ChatMessage{Text: "msg"})
	}
	// Маленькая пауза, чтобы Room.Run() обработал все 60 сообщений.
	// В юнит-тестах такие ожидания должны быть минимальными, чтобы не
	// замедлять прогон. При желании можно заменить на "ожидание условия".
	time.Sleep(100 * time.Millisecond)

	assert.LessOrEqual(t, len(room.History), 50, "история должна ограничиваться 50 сообщениями")
	// (дополнительно можно проверить, что именно последние 50 попали,
	// но для целей данного теста достаточно проверки размера)
}

// --- Тесты Hub ---------------------------------------------------------------

// TestHub_RegisterAndUnregisterClient
// Цель: проверить, что Hub корректно регистрирует клиента (через канал
// RegisterCh с обработкой в Hub.Run), отдает его в GetClients(), а также,
// что UnregisterClient удаляет клиента и вызывает Close().
//
// Важно: Hub.Run() запускается в отдельной горутине — он слушает свои каналы.
// Небольшие слипы после операций нужны, чтобы горутина успела обработать сигнал.
// Альтернативой было бы синхронно вызывать hub.RegisterClient(), но здесь
// специально проверяем канал-ориентированный путь.
func TestHub_RegisterAndUnregisterClient(t *testing.T) {
	hub := chat.NewHub()
	go hub.Run()

	client := newMockClient("bob", "room1")

	// Регистрируем клиента через канал — это эмулирует "боевую" работу Hub.
	hub.RegisterCh <- client
	time.Sleep(100 * time.Millisecond) // даём времени на обработку в Hub.Run

	// Проверяем, что клиент попал в текущий список клиентов Hub.
	assert.Contains(t, hub.GetClients(), client, "клиент должен быть зарегистрирован в Hub")

	// Теперь удаляем клиента (вызываем синхронно — метод сам всё сделает).
	hub.UnregisterClient(client)
	time.Sleep(100 * time.Millisecond) // время на удаление/Close()/уведомления

	// Клиента больше нет в списке; по мок-флажку видно, что его закрыли.
	assert.NotContains(t, hub.GetClients(), client, "клиент должен быть удалён из Hub")
	assert.True(t, client.closed, "при удалении Hub обязан вызвать Close() у клиента")
}



// TestHub_BroadcastPrivate
// Цель: проверить приватную рассылку — когда в сообщении указан получатель (To).
// Ожидание: сообщение уйдёт только целевому клиенту (и отправителю, по коду
// Hub.Broadcast, который рассылает обоим — To и From).
//
// Здесь мы вызываем hub.Broadcast(msg) напрямую (без канала), чтобы обойти
// влияние Hub.Run и протестировать чистую логику маршрутизации приватных сообщений.
func TestHub_BroadcastPrivate(t *testing.T) {
	hub := chat.NewHub()
	go hub.Run()

	alice := newMockClient("alice", "room1")
	bob := newMockClient("bob", "room1")
	hub.RegisterClient(alice)
	hub.RegisterClient(bob)

	// Приватное сообщение: адресат — bob.
	msg := chat.ChatMessage{From: "alice", To: "bob", Text: "secret"}
	hub.Broadcast(msg)

	// Небольшая пауза, чтобы goroutines доставки успели положить сообщение.
	time.Sleep(100 * time.Millisecond)

	// Проверяем, что bob действительно получил "secret".
	// В реальной системе также можно было бы проверить "получил ли отправитель".
	select {
	case got := <-bob.ch:
		assert.Equal(t, "secret", got.Text, "адресат приватного сообщения должен получить его в свой канал")
	case <-time.After(time.Second):
		t.Fatal("приватное сообщение не пришло адресату")
	}
}

// --- Тесты Client ------------------------------------------------------------

// mockConn — мок реализации интерфейса chat.WebSocketConn.
// Нужен для тестирования chat.Client без реального сетевого соединения.
// Мы фиксируем факты записи (WriteJSON/WriteMessage) и закрытия (Close).
type mockConn struct {
	writeJSONCalls []chat.ChatMessage // список объектов, переданных через WriteJSON
	writeMsgCalls  int                // сколько раз вызывался WriteMessage (PING и т.п.)
	closed         bool               // флаг, что соединение закрыто
}

// ReadJSON в тестах нам не нужен — возвращаем ошибку, как будто соединение
// закончилось (EOF). Это удобно для тестов WriteSocket/ReadSocket (если бы были).
func (m *mockConn) ReadJSON(v interface{}) error { return errors.New("eof") }

// WriteJSON просто запоминает, что именно пытались отправить по сокету.
func (m *mockConn) WriteJSON(v interface{}) error {
	m.writeJSONCalls = append(m.writeJSONCalls, v.(chat.ChatMessage))
	return nil
}

// Остальные методы нужны лишь для соответствия интерфейсу; они — ноопы.
func (m *mockConn) SetReadLimit(limit int64)            {}
func (m *mockConn) SetReadDeadline(t time.Time) error   { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetPongHandler(h func(string) error) { return }
func (m *mockConn) WriteMessage(mt int, data []byte) error {
	m.writeMsgCalls++
	return nil
}
func (m *mockConn) Close() error { m.closed = true; return nil }

// TestClient_SendAndReceive
// Цель: проверить, что метод Client.SendMessage неблокирующе кладёт сообщение
// в приватный канал клиента и его можно прочитать через ReceivePrivateChan().
//
// Тест НЕ проверяет сетевую отправку (WriteSocket) — здесь нас интересует
// только локальная логика буферизированного privateChan.
func TestClient_SendAndReceive(t *testing.T) {
	hub := chat.NewHub()
	room := hub.GetRoom("room1") // GetRoom создаёт комнату при отсутствии и запускает её Run.
	conn := &mockConn{}
	client := chat.NewClient(hub, room, conn, "alice")

	// Отправляем в "свою" приватку сообщение (как будто от системы/хаба).
	err := client.SendMessage(chat.ChatMessage{Text: "hello"})
	assert.NoError(t, err, "SendMessage не должен возвращать ошибку при свободном буфере")

	// Читаем из канала, который доступен только на чтение внешнему коду.
	select {
	case msg := <-client.ReceivePrivateChan():
		assert.Equal(t, "hello", msg.Text, "сообщение должно быть доставлено в privateChan клиента")
	case <-time.After(time.Second):
		t.Fatal("не пришло сообщение в privateChan — проверьте реализацию SendMessage/PrivateChan")
	}
}

// TestClient_Close
// Цель: проверить, что Close() у клиента закрывает базовое соединение (WebSocketConn).
// Мы смотрим на флаг mockConn.closed, выставляемый в Close().
func TestClient_Close(t *testing.T) {
	hub := chat.NewHub()
	room := hub.GetRoom("room1")
	conn := &mockConn{}
	client := chat.NewClient(hub, room, conn, "alice")

	err := client.Close()
	assert.NoError(t, err, "Close() клиента не должен возвращать ошибку")
	assert.True(t, conn.closed, "Close() клиента должен закрывать и соединение (WebSocketConn.Close)")
}

// ============================================================================
// Примечания по стабильности тестов:
//   • В местах, где задействованы горутины (Room.Run/Hub.Run), используются
//     короткие time.Sleep/таймауты в select, чтобы избежать зависаний.
//     Это компромисс между скоростью и детерминированностью.
//
//   • Каналы в моках буферизированные — это снижает вероятность блокировок,
//     если получение чуть запаздывает.
//
//   • Для максимальной надёжности можно запускать тесты с флагом -race,
//     чтобы выявить возможные гонки данных.
//
//   • Если потребуется ещё более строгая синхронизация, можно заменить
//     time.Sleep на условное ожидание (например, через дополнительный канал-сигнал
//     "сообщение доставлено") — но для данного уровня юнит-тестов текущий
//     подход обычно достаточен.
// ============================================================================
