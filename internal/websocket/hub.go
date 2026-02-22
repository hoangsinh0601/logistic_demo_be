package websocket

import (
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Allow all origins for dev simplicity
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Client represents a single connected WebSocket client
type Client struct {
	Hub  *Hub
	Conn *websocket.Conn
	Send chan []byte
}

// Hub maintains the set of active clients and broadcasts messages to the clients
type Hub struct {
	clients    map[*Client]bool
	Broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.Mutex // lock just in case if doing manual iter
}

// NewHub initializes a new WS Hub instance
func NewHub() *Hub {
	return &Hub{
		Broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

// Run starts the core dispatch loop for WebSocket events
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Println("New WebSocket client connected")
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
				log.Println("WebSocket client disconnected")
			}
			h.mu.Unlock()
		case message := <-h.Broadcast:
			h.mu.Lock()
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.clients, client)
				}
			}
			h.mu.Unlock()
		}
	}
}

// writePump handles writing messages from the Hub to the WebSocket connection
func (c *Client) writePump() {
	defer func() {
		_ = c.Conn.Close()
	}()
	for message := range c.Send {
		w, err := c.Conn.NextWriter(websocket.TextMessage)
		if err != nil {
			return
		}
		_, _ = w.Write(message)

		// Fast track writing queued messages
		n := len(c.Send)
		for i := 0; i < n; i++ {
			_, _ = w.Write([]byte{'\n'})
			_, _ = w.Write(<-c.Send)
		}

		if err := w.Close(); err != nil {
			return
		}
	}
	_ = c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
}

// readPump pumps messages from the WebSocket connection to the hub
func (c *Client) readPump() {
	defer func() {
		c.Hub.unregister <- c
		_ = c.Conn.Close()
	}()
	for {
		// Just reading to keep connection alive or handle client messages if necessary
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
	}
}

// ServeWs handles websocket requests from the peer
func ServeWs(hub *Hub, c *gin.Context, secret []byte) {
	// 1. Authenticate via token query param
	tokenString := c.Query("token")
	if tokenString == "" {
		log.Println("WebSocket connection rejected: missing token")
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return secret, nil
	})

	if err != nil || !token.Valid {
		log.Println("WebSocket connection rejected: invalid token:", err)
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	// Token is valid, ensure they have proper permissions if needed here
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		log.Println("WebSocket connection rejected: invalid claims")
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	role, _ := claims["role"].(string)
	if role != "admin" && role != "quản lý" && role != "nhân viên" {
		log.Println("WebSocket connection rejected: inadequate permissions")
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("WebSocket upgrade failed:", err)
		return
	}
	client := &Client{Hub: hub, Conn: conn, Send: make(chan []byte, 256)}
	client.Hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in new goroutines
	go client.writePump()
	go client.readPump()
}
