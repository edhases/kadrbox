package ws

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	redisRepo "github.com/edhases/oxide-server/internal/repository/redis"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Відкритий CheckOrigin для легкого підключення десктопу/мобілок/вебу без CORS колізій
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	roomCode string
	userID   string
	userName string
}

type Hub struct {
	redisClient *redisRepo.RedisClient
	rooms       map[string]map[*Client]bool
	broadcast   chan *domain.WatchPartyEvent
	register    chan *Client
	unregister  chan *Client
	mu          sync.RWMutex
	stopChan    chan struct{}
}

func NewHub(redisClient *redisRepo.RedisClient) *Hub {
	return &Hub{
		redisClient: redisClient,
		rooms:       make(map[string]map[*Client]bool),
		broadcast:   make(chan *domain.WatchPartyEvent),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		stopChan:    make(chan struct{}),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case <-h.stopChan:
			h.mu.Lock()
			// Сповіщення всіх клієнтів про планове закриття сервера (1001 Going Away)
			for _, clients := range h.rooms {
				for client := range clients {
					_ = client.conn.WriteMessage(
						websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseGoingAway, "server restarting"),
					)
					close(client.send)
				}
			}
			h.mu.Unlock()
			return

		case client := <-h.register:
			h.mu.Lock()
			if h.rooms[client.roomCode] == nil {
				h.rooms[client.roomCode] = make(map[*Client]bool)
			}
			h.rooms[client.roomCode][client] = true
			h.mu.Unlock()

			// Повідомлення про підключення нового користувача
			event := &domain.WatchPartyEvent{
				Action:     "USER_JOINED",
				RoomCode:   client.roomCode,
				SenderID:   client.userID,
				SenderName: client.userName,
				Timestamp:  time.Now(),
			}
			_ = h.redisClient.PublishWatchPartyEvent(context.Background(), client.roomCode, event)

		case client := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.rooms[client.roomCode]; ok {
				if _, exists := clients[client]; exists {
					delete(clients, client)
					close(client.send)
					if len(clients) == 0 {
						delete(h.rooms, client.roomCode)
					}
				}
			}
			h.mu.Unlock()

			event := &domain.WatchPartyEvent{
				Action:     "USER_LEFT",
				RoomCode:   client.roomCode,
				SenderID:   client.userID,
				SenderName: client.userName,
				Timestamp:  time.Now(),
			}
			_ = h.redisClient.PublishWatchPartyEvent(context.Background(), client.roomCode, event)

		case event := <-h.broadcast:
			h.mu.RLock()
			clients := h.rooms[event.RoomCode]
			data, err := json.Marshal(event)
			if err == nil {
				for client := range clients {
					select {
					case client.send <- data:
					default:
						close(client.send)
						delete(clients, client)
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// GracefulStop плавно зупиняє Hub із закриттям сокетів
func (h *Hub) GracefulStop() {
	close(h.stopChan)
}

func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	roomCode := r.URL.Query().Get("room")
	userID := r.URL.Query().Get("user_id")
	userName := r.URL.Query().Get("user_name")

	if roomCode == "" || userID == "" {
		http.Error(w, "missing room or user_id query parameters", http.StatusBadRequest)
		return
	}
	if userName == "" {
		userName = "Гість"
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}

	client := &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 256),
		roomCode: roomCode,
		userID:   userID,
		userName: userName,
	}

	h.register <- client

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		var event domain.WatchPartyEvent
		if err := json.Unmarshal(message, &event); err == nil {
			event.RoomCode = c.roomCode
			event.SenderID = c.userID
			event.SenderName = c.userName
			event.Timestamp = time.Now()

			// Публікація в Redis Pub/Sub (і трансляція локально)
			_ = c.hub.redisClient.PublishWatchPartyEvent(context.Background(), c.roomCode, &event)
			c.hub.broadcast <- &event
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(20 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			_, _ = w.Write(message)
			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
