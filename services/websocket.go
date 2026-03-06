package services

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

const (
	MessageTypeReview = "review"
	MessageTypeError  = "review_error"
)

type WSMessage struct {
	Type    string      `json:"type"`
	MrID    int         `json:"mr_id,omitempty"`
	Review  interface{} `json:"review,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

type WSHub struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan []byte
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mu         sync.RWMutex
}

func NewWSHub() *WSHub {
	return &WSHub{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
}

func (h *WSHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			logrus.Infof("[WS] Client connected, total: %d", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.Close()
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				if err := client.WriteMessage(websocket.TextMessage, message); err != nil {
					logrus.Warnf("[WS] Write error: %v", err)
					h.unregister <- client
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *WSHub) BroadcastReview(mrID int, review interface{}) {
	msg := WSMessage{Type: MessageTypeReview, MrID: mrID, Review: review}
	data, err := json.Marshal(msg)
	if err != nil {
		logrus.Errorf("[WS] Marshal error: %v", err)
		return
	}
	h.broadcast <- data
	logrus.Infof("[WS] Broadcast review for MR %d to %d clients", mrID, len(h.clients))
}

func (h *WSHub) HandleWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logrus.Warnf("[WS] Upgrade error: %v", err)
		return
	}
	h.register <- conn
	defer func() { h.unregister <- conn }()

	// Read loop - keep connection alive, handle pings
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (h *WSHub) BroadcastReviewError(mrID int, errMsg string) {
	msg := WSMessage{Type: MessageTypeError, MrID: mrID, Error: errMsg, Message: errMsg}
	data, err := json.Marshal(msg)
	if err != nil {
		logrus.Errorf("[WS] Marshal error: %v", err)
		return
	}
	h.broadcast <- data
}
