package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/sithuaung/gocket/internal/app"
	"github.com/sithuaung/gocket/internal/auth"
	"github.com/sithuaung/gocket/internal/channel"
	"github.com/sithuaung/gocket/internal/proto"
	"github.com/sithuaung/gocket/internal/state"
	"github.com/sithuaung/gocket/internal/ws"
)

const (
	activityTimeout        = 120 * time.Second
	activityGrace          = 30 * time.Second
	maxChannelsPerConn     = 100
)

type WSHandler struct {
	Apps           *app.Registry
	Managers       map[string]*channel.Manager
	Connections    map[string]*state.ConnectionManager
	MaxMessageSize int64
	AllowedOrigins []string
}

func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	appKey := r.PathValue("appKey")
	if appKey == "" {
		http.Error(w, "missing app key", http.StatusBadRequest)
		return
	}

	a, ok := h.Apps.FindByKey(appKey)
	if !ok {
		http.Error(w, "invalid app key", http.StatusNotFound)
		return
	}

	acceptOpts := &websocket.AcceptOptions{}
	if len(h.AllowedOrigins) == 0 {
		acceptOpts.InsecureSkipVerify = true
	} else {
		acceptOpts.OriginPatterns = h.AllowedOrigins
	}
	wsConn, err := websocket.Accept(w, r, acceptOpts)
	if err != nil {
		slog.Error("websocket accept failed", "error", err)
		return
	}

	conn := ws.NewConn(r.Context(), wsConn, a.ID, a.Key, h.MaxMessageSize, a.MaxClientEventsPerSec)

	if cm, ok := h.Connections[a.ID]; ok {
		if !cm.Add(conn) {
			slog.Warn("max connections reached", "security_event", "connection_limit", "app_id", a.ID)
			conn.Close(websocket.StatusCode(4100), "max connections reached")
			return
		}
	}

	slog.Info("client connected", "socket_id", conn.SocketID(), "app_id", a.ID)

	// Send connection established
	data, _ := json.Marshal(map[string]any{
		"socket_id":        conn.SocketID(),
		"activity_timeout": 120,
	})
	established := proto.Event{
		Event: "pusher:connection_established",
		Data:  string(data),
	}
	if err := conn.Send(conn.Context(), established); err != nil {
		slog.Error("failed to send connection_established", "error", err)
		conn.Close(websocket.StatusInternalError, "")
		return
	}

	h.readLoop(conn, a)
}

func (h *WSHandler) readLoop(conn *ws.Conn, a *app.App) {
	defer func() {
		if mgr, ok := h.Managers[a.ID]; ok {
			mgr.RemoveConnection(conn.SocketID(), conn.ChannelNames())
		}
		if cm, ok := h.Connections[a.ID]; ok {
			cm.Remove(conn.SocketID())
		}
		conn.Close(websocket.StatusNormalClosure, "")
		slog.Info("client disconnected", "socket_id", conn.SocketID())
	}()

	// Activity timeout: disconnect if no message received within timeout + grace period.
	// The client is expected to send pusher:ping before the activity_timeout (120s).
	// We add 30s grace, so the actual deadline is 150s of silence.
	idleTimer := time.NewTimer(activityTimeout + activityGrace)
	defer idleTimer.Stop()

	// Close the connection when the idle timer fires
	go func() {
		select {
		case <-idleTimer.C:
			slog.Info("activity timeout", "socket_id", conn.SocketID())
			conn.Close(websocket.StatusCode(4201), "activity timeout")
		case <-conn.Context().Done():
		}
	}()

	for {
		event, err := conn.Read(conn.Context())
		if err != nil {
			if websocket.CloseStatus(err) != -1 {
				slog.Info("websocket closed", "socket_id", conn.SocketID(), "status", websocket.CloseStatus(err))
			} else if conn.Context().Err() == nil {
				slog.Warn("read error", "socket_id", conn.SocketID(), "error", err)
			}
			return
		}

		// Reset idle timer on any received message
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(activityTimeout + activityGrace)

		switch {
		case event.Event == "pusher:ping":
			pong := proto.Event{Event: "pusher:pong", Data: "{}"}
			if err := conn.Send(conn.Context(), pong); err != nil {
				slog.Warn("failed to send pong", "error", err)
				return
			}

		case event.Event == "pusher:subscribe":
			h.handleSubscribe(conn, a, event)

		case event.Event == "pusher:unsubscribe":
			h.handleUnsubscribe(conn, a, event)

		case strings.HasPrefix(event.Event, "pusher:"):
			h.sendError(conn, 4009, "unknown protocol event")

		case strings.HasPrefix(event.Event, "client-"):
			h.handleClientEvent(conn, a, event)

		default:
			h.sendError(conn, 4009, "unsupported event prefix")
		}
	}
}

func (h *WSHandler) handleSubscribe(conn *ws.Conn, a *app.App, event proto.Event) {
	sub, err := proto.ParseSubscribeData(event.Data)
	if err != nil {
		h.sendError(conn, 4009, "invalid subscribe data")
		return
	}

	if err := proto.ValidateChannelName(sub.Channel); err != nil {
		h.sendError(conn, 4009, err.Error())
		return
	}

	chType := channel.ParseType(sub.Channel)

	switch chType {
	case channel.TypePrivate, channel.TypeEncrypted:
		if !auth.ValidatePrivateAuth(a.Key, a.Secret, conn.SocketID(), sub.Channel, sub.Auth) {
			slog.Warn("channel auth failed", "security_event", "auth_failed", "reason", "invalid_token", "channel", sub.Channel, "socket_id", conn.SocketID(), "app_id", a.ID)
			h.sendError(conn, 4009, "authorization failed")
			return
		}
	case channel.TypePresence:
		if !auth.ValidatePresenceAuth(a.Key, a.Secret, conn.SocketID(), sub.Channel, sub.ChannelData, sub.Auth) {
			slog.Warn("channel auth failed", "security_event", "auth_failed", "reason", "invalid_token", "channel", sub.Channel, "socket_id", conn.SocketID(), "app_id", a.ID)
			h.sendError(conn, 4009, "authorization failed")
			return
		}
	}

	if conn.ChannelCount() >= maxChannelsPerConn {
		slog.Warn("max channels per connection exceeded", "security_event", "channel_limit", "socket_id", conn.SocketID(), "app_id", a.ID)
		h.sendError(conn, 4009, "max channels per connection exceeded")
		return
	}

	mgr, ok := h.Managers[a.ID]
	if !ok {
		return
	}

	var member *channel.PresenceMember
	if chType == channel.TypePresence && sub.ChannelData != "" {
		var m channel.PresenceMember
		if err := json.Unmarshal([]byte(sub.ChannelData), &m); err == nil {
			member = &m
		}
	}

	ch := mgr.Subscribe(conn, sub.Channel, member)

	successEvent, err := ch.SubscriptionSucceededEvent()
	if err != nil {
		slog.Error("failed to build subscription_succeeded", "error", err)
		return
	}
	if err := conn.Send(conn.Context(), successEvent); err != nil {
		slog.Warn("failed to send subscription_succeeded", "error", err)
	}

	slog.Info("subscribed", "socket_id", conn.SocketID(), "channel", sub.Channel)
}

func (h *WSHandler) handleUnsubscribe(conn *ws.Conn, a *app.App, event proto.Event) {
	sub, err := proto.ParseSubscribeData(event.Data)
	if err != nil {
		return
	}

	if err := proto.ValidateChannelName(sub.Channel); err != nil {
		return
	}

	if mgr, ok := h.Managers[a.ID]; ok {
		mgr.Unsubscribe(conn.SocketID(), sub.Channel)
	}

	slog.Info("unsubscribed", "socket_id", conn.SocketID(), "channel", sub.Channel)
}

func (h *WSHandler) handleClientEvent(conn *ws.Conn, a *app.App, event proto.Event) {
	if !a.ClientEvents {
		h.sendError(conn, 4301, "client events not enabled")
		return
	}

	if !conn.ClientEventRate.Allow() {
		slog.Warn("client event rate limit exceeded", "security_event", "rate_limited", "socket_id", conn.SocketID(), "app_id", a.ID)
		h.sendError(conn, 4301, "rate limit exceeded")
		return
	}

	if err := proto.ValidateEventName(event.Event); err != nil {
		h.sendError(conn, 4009, err.Error())
		return
	}
	if err := proto.ValidateChannelName(event.Channel); err != nil {
		h.sendError(conn, 4009, err.Error())
		return
	}

	chType := channel.ParseType(event.Channel)
	if chType != channel.TypePrivate && chType != channel.TypePresence && chType != channel.TypeEncrypted {
		h.sendError(conn, 4301, "client events only on private/presence channels")
		return
	}

	if !conn.HasChannel(event.Channel) {
		h.sendError(conn, 4301, "not subscribed to channel")
		return
	}

	mgr, ok := h.Managers[a.ID]
	if !ok {
		return
	}

	ch, ok := mgr.GetChannel(event.Channel)
	if !ok {
		return
	}

	ch.Broadcast(event, conn.SocketID())
}

func (h *WSHandler) sendError(conn *ws.Conn, code int, message string) {
	data, _ := json.Marshal(map[string]any{
		"code":    code,
		"message": message,
	})
	errEvent := proto.Event{
		Event: "pusher:error",
		Data:  string(data),
	}
	conn.Send(conn.Context(), errEvent)
}
