package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/config"
	"github.com/yourusername/kor-assetforge/middleware"
	"github.com/yourusername/kor-assetforge/utils"
	"go.uber.org/zap"
)

// EventType classifies a real-time WebSocket event.
type EventType string

const (
	EventAssetCreated     EventType = "asset.created"
	EventAssetUpdated     EventType = "asset.updated"
	EventTransactionNew   EventType = "transaction.new"
	EventListingCreated   EventType = "listing.created"
	EventMarketplaceTrade EventType = "marketplace.trade"
	EventBatchProgress    EventType = "batch.progress"
	EventPing             EventType = "ping"
	EventAuthExpiring     EventType = "auth.expiring"
)

// Topic is a named channel that subscribers can filter on.
type Topic string

const (
	TopicAssets       Topic = "assets"
	TopicTransactions Topic = "transactions"
	TopicMarketplace  Topic = "marketplace"
	TopicAll          Topic = "all"
)

// WSEvent is the envelope sent to every client.
//
// Seq is a monotonically increasing, per-hub sequence number assigned at
// broadcast time. Clients use it to detect gaps (dropped/out-of-order
// delivery) and reorder buffered messages; see frontend/src/lib/websocket.ts.
type WSEvent struct {
	Type      EventType   `json:"type"`
	Payload   interface{} `json:"payload"`
	Timestamp time.Time   `json:"timestamp"`
	Seq       int64       `json:"seq"`
}

// WSStats exposes live metrics about the hub.
type WSStats struct {
	ConnectedClients int64     `json:"connected_clients"`
	TotalBroadcasts  int64     `json:"total_broadcasts"`
	Uptime           time.Time `json:"uptime"`
}

// ---- subscriber -------------------------------------------------------------

type subscriber struct {
	id     string
	conn   *utils.WSConn
	topics map[Topic]bool
	send   chan []byte
	done   chan struct{}
}

func newSubscriber(id string, conn *utils.WSConn, topics []string) *subscriber {
	t := make(map[Topic]bool)
	if len(topics) == 0 {
		t[TopicAll] = true
	}
	for _, name := range topics {
		t[Topic(name)] = true
	}
	return &subscriber{id: id, conn: conn, topics: t, send: make(chan []byte, 128), done: make(chan struct{})}
}

func (s *subscriber) matches(topic Topic) bool {
	return s.topics[TopicAll] || s.topics[topic]
}

// ---- singleflight (stdlib-only stampede guard used by broadcast) ------------

// ---- Hub --------------------------------------------------------------------

type hubMsg struct {
	topic   Topic
	payload []byte
}

// Hub manages all active WebSocket connections and fan-out broadcasting.
// Registration and removal are serialized through channels so broadcasts never
// hold a connection's read or write loop hostage.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[string]*subscriber

	register   chan *subscriber
	unregister chan string
	broadcast  chan *hubMsg

	totalBroadcasts atomic.Int64
	seq             atomic.Int64
	startedAt       time.Time
}

var (
	globalHub *Hub
	hubOnce   sync.Once
)

// GetHub returns the process-wide singleton Hub, starting its run loop once.
func GetHub() *Hub {
	hubOnce.Do(func() {
		globalHub = &Hub{
			subscribers: make(map[string]*subscriber),
			register:    make(chan *subscriber, 64),
			unregister:  make(chan string, 64),
			broadcast:   make(chan *hubMsg, 512),
			startedAt:   time.Now(),
		}
		go globalHub.run()
	})
	return globalHub
}

func (h *Hub) run() {
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case sub := <-h.register:
			if sub.done == nil {
				sub.done = make(chan struct{})
			}
			h.mu.Lock()
			h.subscribers[sub.id] = sub
			h.mu.Unlock()
			Logger.Info("WebSocket client connected",
				zap.String("id", sub.id),
				zap.Int("total", h.connectedCount()))
			// Start the writer goroutine for this subscriber
			go h.writerLoop(sub)

		case id := <-h.unregister:
			h.mu.Lock()
			if sub, ok := h.subscribers[id]; ok {
				if sub.conn != nil {
					sub.conn.Close()
				}
				select {
				case <-sub.done:
				default:
					close(sub.done)
				}
				close(sub.send)
				delete(h.subscribers, id)
			}
			h.mu.Unlock()
			Logger.Info("WebSocket client disconnected",
				zap.String("id", id),
				zap.Int("total", h.connectedCount()))

		case msg := <-h.broadcast:
			h.totalBroadcasts.Add(1)
			h.fanOut(msg)

		case <-heartbeat.C:
			h.sendHeartbeat()
		}
	}
}

func (h *Hub) fanOut(msg *hubMsg) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for id, sub := range h.subscribers {
		if !sub.matches(msg.topic) {
			continue
		}
		select {
		case sub.send <- msg.payload:
		default:
			// Slow client: schedule disconnect to avoid blocking the hub
			go func(cid string) { h.unregister <- cid }(id)
		}
	}
}

func (h *Hub) sendHeartbeat() {
	evt := WSEvent{Type: EventPing, Payload: nil, Timestamp: time.Now(), Seq: h.seq.Add(1)}
	data, _ := json.Marshal(evt)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, sub := range h.subscribers {
		select {
		case sub.send <- data:
		default:
		}
	}
}

func (h *Hub) writerLoop(sub *subscriber) {
	if sub.conn == nil {
		return
	}
	for msg := range sub.send {
		if err := sub.conn.WriteMessage(msg); err != nil {
			h.unregister <- sub.id
			return
		}
	}
}

func (h *Hub) connectedCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers)
}

// Broadcast dispatches an event to all subscribers interested in topic.
// It assigns the next monotonic sequence number before sending (see WSEvent.Seq).
func (h *Hub) Broadcast(topic Topic, event WSEvent) {
	event.Seq = h.seq.Add(1)
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	select {
	case h.broadcast <- &hubMsg{topic: topic, payload: data}:
	default:
		Logger.Warn("WebSocket broadcast channel full, dropping message")
	}
}

// BroadcastEvent is the package-level convenience helper.
func BroadcastEvent(topic Topic, evType EventType, payload interface{}) {
	GetHub().Broadcast(topic, WSEvent{
		Type:      evType,
		Payload:   payload,
		Timestamp: time.Now(),
	})
}

// ---- HTTP handler -----------------------------------------------------------

// WebSocketHandler wires WebSocket endpoints into Gin.
type WebSocketHandler struct {
	auth      *middleware.WSAuthValidator
	rateConfig config.WSRateLimitConfig
}

func NewWebSocketHandler(jwtSecret string, rateConfig config.WSRateLimitConfig) *WebSocketHandler {
	return &WebSocketHandler{
		auth:      middleware.NewWSAuthValidator(jwtSecret),
		rateConfig: rateConfig,
	}
}

// HandleWS upgrades GET /ws to a WebSocket connection. The access token is
// checked before the HTTP upgrade; after upgrading, expiry and message-rate
// enforcement are handled by the connection's lifecycle goroutines.
//
// Query parameters:
//
//	token  – bearer token for authentication (validated below)
//	topics – repeated; one of: assets, transactions, marketplace, all (default)
func (wsh *WebSocketHandler) HandleWS(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		token = c.GetHeader("Authorization")
	}
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication token required"})
		return
	}
	claims, err := wsh.auth.Validate(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
		return
	}

	topicParams := c.QueryArray("topics")

	wsConn, err := utils.UpgradeHTTP(c.Writer, c.Request)
	if err != nil {
		// UpgradeHTTP already hijacked the socket on success; on failure the
		// HTTP response is still writable.
		c.JSON(http.StatusBadRequest, gin.H{"error": "WebSocket upgrade failed: " + err.Error()})
		return
	}

	reqID := c.GetString("request_id")
	if reqID == "" {
		reqID = fmt.Sprintf("ws-%d", time.Now().UnixNano())
	}

	sub := newSubscriber(reqID, wsConn, topicParams)
	rateLimit := middleware.NewWebSocketRateLimiter(
		wsh.rateConfig.MessagesPerSecond,
		wsh.rateConfig.Burst,
	)
	hub := GetHub()
	hub.register <- sub
	go wsh.expiryLoop(sub, claims.ExpiresAt)

	// Reader loop: handles ping/pong, close frames, and client commands.
	for {
		op, msg, err := wsConn.ReadMessage()
		if err != nil {
			break
		}
		switch op {
		case utils.OpClose:
			hub.unregister <- sub.id
			return
		case utils.OpPing:
			_ = wsConn.WritePong(msg)
		case utils.OpText:
			// Client can send {"action":"subscribe","topic":"assets"} etc.
			if !rateLimit.Allow() {
				wsConn.CloseWithCode(1008, "message rate limit exceeded")
				hub.unregister <- sub.id
				return
			}
			wsh.handleClientMessage(sub, msg)
		}
	}

	hub.unregister <- sub.id
}

func (wsh *WebSocketHandler) expiryLoop(sub *subscriber, expiresAt time.Time) {
	warningAt := expiresAt.Add(-time.Minute)
	if delay := time.Until(warningAt); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
			data, _ := json.Marshal(WSEvent{Type: EventAuthExpiring, Payload: nil, Timestamp: time.Now()})
			select {
			case sub.send <- data:
			case <-sub.done:
				return
			}
		case <-sub.done:
			return
		}
	}

	if delay := time.Until(expiresAt); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
			sub.conn.CloseWithCode(4001, "token expired")
		case <-sub.done:
		}
	}
}

// handleClientMessage processes JSON commands sent by the client.
func (wsh *WebSocketHandler) handleClientMessage(sub *subscriber, raw []byte) {
	var cmd struct {
		Action string `json:"action"`
		Topic  string `json:"topic"`
	}
	if err := json.Unmarshal(raw, &cmd); err != nil {
		return
	}
	switch cmd.Action {
	case "subscribe":
		sub.topics[Topic(cmd.Topic)] = true
	case "unsubscribe":
		delete(sub.topics, Topic(cmd.Topic))
	}
}

// HandleWSStats returns live hub metrics.
// @Summary Get WebSocket stats
// @Description Returns live metrics about the WebSocket hub.
// @Tags websocket
// @Success 200 {object} WSStats
// @Router /ws/stats [get]
func (wsh *WebSocketHandler) HandleWSStats(c *gin.Context) {
	hub := GetHub()
	c.JSON(http.StatusOK, WSStats{
		ConnectedClients: int64(hub.connectedCount()),
		TotalBroadcasts:  hub.totalBroadcasts.Load(),
		Uptime:           hub.startedAt,
	})
}
