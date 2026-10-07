package ws

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/edhases/kadrbox-server/internal/domain"
	"github.com/gorilla/websocket"
)

// TestRealUserWatchPartySimulation моделює життєвий цикл реальної Watch Party:
// Послідовне підключення від 1 до 5 реальних користувачів по живому WebSocket,
// спільний перегляд (Play, Seek, Pause, Speed, Sync), обмін повідомленнями в чаті,
// перевірку прав доступу (гість не може керувати плеєром) та коректне відключення.
// Жодних моків - реальний сервер Go Hub + повноцінні WebSocket з'єднання.
func TestRealUserWatchPartySimulation(t *testing.T) {
	// 1. Запуск реального Watch Party Hub
	h := startHub(t, func(opts *Options) {
		opts.MaxClientsPerRoom = 10
		opts.JoinLimiter = JoinLimiterConfig{Disabled: true}
	})
	srv := hubServer(t, h)

	const roomCode = "REALWP1"

	type UserClient struct {
		id       string
		name     string
		isHost   bool
		conn     *websocket.Conn
		received chan domain.WatchPartyEvent
		done     chan struct{}
	}

	startUser := func(id, name string, isHost bool) *UserClient {
		var token string
		var err error
		if isHost {
			token, err = h.IssueHostWatchPartyTicket(id, name, roomCode, time.Minute)
		} else {
			token, err = h.IssueWatchPartyTicket(id, name, roomCode, time.Minute)
		}
		if err != nil {
			t.Fatalf("Failed to issue ticket for %s: %v", name, err)
		}

		conn, resp, err := dialWS(srv, "/?room="+roomCode+"&ticket="+token, nil)
		if err != nil {
			t.Fatalf("User %s failed to connect via WebSocket: %v", name, err)
		}
		if resp.StatusCode != 101 {
			t.Fatalf("User %s unexpected status code: %d", name, resp.StatusCode)
		}

		u := &UserClient{
			id:       id,
			name:     name,
			isHost:   isHost,
			conn:     conn,
			received: make(chan domain.WatchPartyEvent, 100),
			done:     make(chan struct{}),
		}

		// Фоновий слухач отримання реальних повідомлень клієнтом
		go func() {
			defer close(u.done)
			for {
				_, data, err := conn.ReadMessage()
				if err != nil {
					return
				}
				var ev domain.WatchPartyEvent
				if err := json.Unmarshal(data, &ev); err == nil {
					u.received <- ev
				}
			}
		}()

		return u
	}

	sendAction := func(u *UserClient, action string, payload any) {
		t.Helper()
		msg := map[string]any{
			"action": action,
		}
		if payload != nil {
			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal payload failed: %v", err)
			}
			msg["payload"] = json.RawMessage(payloadBytes)
		}
		data, _ := json.Marshal(msg)
		if err := u.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			t.Fatalf("User %s failed to send %s: %v", u.name, action, err)
		}
	}

	awaitEvent := func(u *UserClient, wantAction string, timeout time.Duration) domain.WatchPartyEvent {
		t.Helper()
		deadline := time.After(timeout)
		for {
			select {
			case ev := <-u.received:
				if ev.Action == wantAction {
					return ev
				}
			case <-deadline:
				t.Fatalf("Timeout: User %s did not receive action %q in time", u.name, wantAction)
			}
		}
	}

	awaitEventOnAll := func(users []*UserClient, wantAction string, timeout time.Duration) {
		t.Helper()
		var wg sync.WaitGroup
		for _, u := range users {
			wg.Add(1)
			go func(usr *UserClient) {
				defer wg.Done()
				deadline := time.After(timeout)
				for {
					select {
					case ev := <-usr.received:
						if ev.Action == wantAction {
							return
						}
					case <-deadline:
						t.Errorf("Timeout: User %s did not receive action %q in time", usr.name, wantAction)
						return
					}
				}
			}(u)
		}
		wg.Wait()
	}

	// =========================================================================
	// СЦЕНАРІЙ 1: Користувач 1 (Олександр - Хост) створює кімнату
	// =========================================================================
	t.Log("===> [1 КОРИСТУВАЧ]: Хост Олександр підключається до кімнати")
	host := startUser("user-1", "Олександр (Host)", true)
	defer host.conn.Close()

	// Хост отримує подію userJoined про себе
	evJoined := awaitEvent(host, ActionUserJoined, 2*time.Second)
	t.Logf("Хост підключився: %s (room: %s, user: %s)", evJoined.Action, evJoined.RoomCode, evJoined.SenderName)

	// =========================================================================
	// СЦЕНАРІЙ 2: Користувач 2 (Марія) приєднується
	// =========================================================================
	t.Log("===> [2 КОРИСТУВАЧІ]: Марія приєднується до кімнати")
	user2 := startUser("user-2", "Марія", false)
	defer user2.conn.Close()

	// І Хост, і Марія отримують повідомлення про вхід Марії
	awaitEventOnAll([]*UserClient{host, user2}, ActionUserJoined, 2*time.Second)
	t.Log("Марія успішно зареєстрована в кімнаті і відобразилася у хоста")

	// Хост запускає відтворення (Play)
	t.Log("Хост натискає Play (старт фільму)")
	sendAction(host, ActionPlay, nil)
	// Завдяки echo filter Марія отримує подію Play
	awaitEvent(user2, ActionPlay, 2*time.Second)
	t.Log("Марія синхронізувала старт відтворення (Play)")

	// Марія пише в чат: "Всім привіт!"
	t.Log("Марія пише в чат: 'Всім привіт!'")
	sendAction(user2, ActionChat, "Всім привіт!")
	// Хост отримує чат-повідомлення
	evChat := awaitEvent(host, ActionChat, 2*time.Second)
	t.Logf("Хост отримав повідомлення з чату від %s: %v", evChat.SenderName, evChat.Payload)

	// Хост ставить на паузу (хтось підійшов)
	t.Log("Хост ставить на паузу (Pause)")
	sendAction(host, ActionPause, nil)
	awaitEvent(user2, ActionPause, 2*time.Second)
	t.Log("Марія отримала подію Pause")

	// =========================================================================
	// СЦЕНАРІЙ 3: Користувач 3 (Дмитро) приєднується
	// =========================================================================
	t.Log("===> [3 КОРИСТУВАЧІ]: Дмитро приєднується")
	user3 := startUser("user-3", "Дмитро", false)
	defer user3.conn.Close()

	// Усі троє бачать вхід Дмитра
	awaitEventOnAll([]*UserClient{host, user2, user3}, ActionUserJoined, 2*time.Second)
	t.Log("Всі учасники бачать підключення Дмитра")

	// Дмитро просить синхронізувати таймкод (requestSync)
	t.Log("Дмитро надсилає запит синхронізації до хоста (requestSync)")
	sendAction(user3, ActionRequestSync, nil)

	// Хост отримує requestSync і шле актуальний таймкод sync
	evSyncReq := awaitEvent(host, ActionRequestSync, 2*time.Second)
	t.Logf("Хост отримав запит на синхронізацію від %s", evSyncReq.SenderName)

	sendAction(host, ActionSync, map[string]any{
		"position": 12000,
		"speed":    1.0,
	})
	// Гості (Марія та Дмитро) отримують sync
	awaitEventOnAll([]*UserClient{user2, user3}, ActionSync, 2*time.Second)
	t.Log("Усі гості успішно синхронізували позицію фільму")

	// =========================================================================
	// СЦЕНАРІЙ 4: Користувач 4 (Олена) приєднується + Перемотування (Seek)
	// =========================================================================
	t.Log("===> [4 КОРИСТУВАЧІ]: Олена приєднується")
	user4 := startUser("user-4", "Олена", false)
	defer user4.conn.Close()

	awaitEventOnAll([]*UserClient{host, user2, user3, user4}, ActionUserJoined, 2*time.Second)

	// Хост перемотує вперед на 15 хвилину (Seek 900 000 ms)
	t.Log("Хост робить Seek на 15-ту хвилину (900 000 ms)")
	sendAction(host, ActionSeek, 900000)
	awaitEventOnAll([]*UserClient{user2, user3, user4}, ActionSeek, 2*time.Second)
	t.Log("Усі 3 гості перемотали фільм на 15-ту хвилину")

	// Хост продовжує відтворення (Play)
	sendAction(host, ActionPlay, nil)
	awaitEventOnAll([]*UserClient{user2, user3, user4}, ActionPlay, 2*time.Second)

	// =========================================================================
	// СЦЕНАРІЙ 5: Користувач 5 (Тарас) приєднується + Зміна швидкості (Speed 1.25x)
	// =========================================================================
	t.Log("===> [5 КОРИСТУВАЧІВ]: Тарас приєднується")
	user5 := startUser("user-5", "Тарас", false)
	defer user5.conn.Close()

	activeUsers := []*UserClient{host, user2, user3, user4, user5}
	awaitEventOnAll(activeUsers, ActionUserJoined, 2*time.Second)

	// Хост змінює швидкість на 1.25x
	t.Log("Хост змінює швидкість відтворення на 1.25x (Speed)")
	sendAction(host, ActionSpeed, 1.25)
	awaitEventOnAll([]*UserClient{user2, user3, user4, user5}, ActionSpeed, 2*time.Second)
	t.Log("Усі 4 гості перемкнули швидкість на 1.25x")

	// ПЕРЕВІРКА БЕЗПЕКИ:
	// Гість Тарас намагається несанкціоновано поставити на паузу (має бути відхилено бекендом)
	t.Log("Гість Тарас намагається натиснути Pause (Бекенд повинен відхилити, бо він не хост)")
	sendAction(user5, ActionPause, nil)
	// Даємо паузу переконатися, що ніхто не отримав команду Pause від Тараса
	time.Sleep(300 * time.Millisecond)
	select {
	case ev := <-host.received:
		if ev.Action == ActionPause {
			t.Fatalf("Критична вразливість: хост отримав паузу від гостя Тараса!")
		}
	default:
		t.Log("Безпека підтверджена: несанкціоновану команду від гостя відхилено")
	}

	// КОНКУРЕНТНИЙ ЧАТ:
	// Усі 5 користувачів пишуть у чат одночасно
	t.Log("Усі 5 користувачів одночасно відправляють репліки в чат")
	for i, u := range activeUsers {
		sendAction(u, ActionChat, fmt.Sprintf("Коментар від %s (#%d)", u.name, i+1))
	}

	// Кожен клієнт повинен отримати повідомлення від інших 4 учасників
	for _, u := range activeUsers {
		gotChats := 0
		deadline := time.After(3 * time.Second)
	collectChats:
		for gotChats < 4 {
			select {
			case ev := <-u.received:
				if ev.Action == ActionChat {
					gotChats++
				}
			case <-deadline:
				t.Fatalf("User %s received only %d of 4 expected chat messages", u.name, gotChats)
				break collectChats
			}
		}
	}
	t.Log("Усі 5 користувачів успішно отримали всі повідомлення конкурентного чату!")

	// =========================================================================
	// ВІД'ЄДНАННЯ КОРИСТУВАЧІВ (UserLeft)
	// =========================================================================
	t.Log("Користувач 5 (Тарас) виходить з кімнати")
	_ = user5.conn.Close()
	remainingUsers := []*UserClient{host, user2, user3, user4}
	awaitEventOnAll(remainingUsers, ActionUserLeft, 3*time.Second)

	t.Log("Користувач 4 (Олена) виходить з кімнати")
	_ = user4.conn.Close()
	remainingUsers = []*UserClient{host, user2, user3}
	awaitEventOnAll(remainingUsers, ActionUserLeft, 3*time.Second)

	t.Log("Користувач 3 (Дмитро) виходить з кімнати")
	_ = user3.conn.Close()
	remainingUsers = []*UserClient{host, user2}
	awaitEventOnAll(remainingUsers, ActionUserLeft, 3*time.Second)

	t.Log("Користувач 2 (Марія) виходить з кімнати")
	_ = user2.conn.Close()
	remainingUsers = []*UserClient{host}
	awaitEventOnAll(remainingUsers, ActionUserLeft, 3*time.Second)

	t.Log("Хост залишається сам у кімнаті та успішно завершує сесію")
	t.Log("✅ Усі 5 етапів Watch Party (1 -> 2 -> 3 -> 4 -> 5 -> 1 користувач) пройшли успішно без жодних моків!")
}
