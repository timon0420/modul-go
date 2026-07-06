package logger

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const historyLimit = 200

type Hub struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
	history [][]byte
}

func New() (*slog.Logger, *Hub) {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	hub := &Hub{clients: make(map[chan []byte]struct{})}
	base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(&broadcastHandler{base: base, hub: hub}), hub
}

type broadcastHandler struct {
	base   slog.Handler
	hub    *Hub
	attrs  []slog.Attr
	groups []string
}

func (h *broadcastHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}
func (h *broadcastHandler) Handle(ctx context.Context, record slog.Record) error {
	if err := h.base.Handle(ctx, record); err != nil {
		return err
	}
	entry := map[string]any{"time": record.Time.Format(time.RFC3339Nano), "level": record.Level.String(), "msg": record.Message}
	for _, attr := range h.attrs {
		entry[attr.Key] = attr.Value.Any()
	}
	record.Attrs(func(attr slog.Attr) bool { entry[attr.Key] = attr.Value.Any(); return true })
	payload, err := json.Marshal(entry)
	if err == nil {
		h.hub.publish(payload)
	}
	return err
}
func (h *broadcastHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	copyAttrs := append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &broadcastHandler{base: h.base.WithAttrs(attrs), hub: h.hub, attrs: copyAttrs, groups: h.groups}
}
func (h *broadcastHandler) WithGroup(name string) slog.Handler {
	return &broadcastHandler{base: h.base.WithGroup(name), hub: h.hub, attrs: h.attrs, groups: append(h.groups, name)}
}

func (h *Hub) publish(payload []byte) {
	h.mu.Lock()
	h.history = append(h.history, append([]byte(nil), payload...))
	if len(h.history) > historyLimit {
		h.history = h.history[len(h.history)-historyLimit:]
	}
	for client := range h.clients {
		select {
		case client <- payload:
		default:
		}
	}
	h.mu.Unlock()
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if token := os.Getenv("LOG_WS_TOKEN"); token != "" && r.URL.Query().Get("token") != token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	connection, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer connection.Close()
	client := make(chan []byte, 64)
	h.mu.Lock()
	for _, item := range h.history {
		if err := connection.WriteMessage(websocket.TextMessage, item); err != nil {
			h.mu.Unlock()
			return
		}
	}
	h.clients[client] = struct{}{}
	h.mu.Unlock()
	defer func() { h.mu.Lock(); delete(h.clients, client); h.mu.Unlock() }()
	for payload := range client {
		if err := connection.WriteMessage(websocket.TextMessage, payload); err != nil {
			return
		}
	}
}
