package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/sithuaung/gocket/internal/app"
	"github.com/sithuaung/gocket/internal/channel"
	"github.com/sithuaung/gocket/internal/proto"
	"github.com/sithuaung/gocket/internal/ratelimit"
)

type TriggerRequest struct {
	Name     string   `json:"name"`
	Data     string   `json:"data"`
	Channels []string `json:"channels"`
	Channel  string   `json:"channel"`
	SocketID string   `json:"socket_id"`
	Info     string   `json:"info"`
}

type BatchRequest struct {
	Batch []TriggerRequest `json:"batch"`
}

type EventsHandler struct {
	Apps             *app.Registry
	Managers         map[string]*channel.Manager
	BackendRateLimits map[string]*ratelimit.Limiter
}

func (h *EventsHandler) TriggerEvent(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	a, ok := h.Apps.FindByID(appID)
	if !ok {
		http.Error(w, `{"error":"app not found"}`, http.StatusNotFound)
		return
	}

	if rl, ok := h.BackendRateLimits[a.ID]; ok && !rl.Allow() {
		slog.Warn("backend rate limit exceeded", "security_event", "rate_limited", "app_id", a.ID, "endpoint", "trigger", "remote_addr", r.RemoteAddr)
		http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
		return
	}

	var req TriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if err := proto.ValidateEventName(req.Name); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	channels := req.Channels
	if len(channels) == 0 && req.Channel != "" {
		channels = []string{req.Channel}
	}

	if len(channels) == 0 {
		http.Error(w, `{"error":"no channels specified"}`, http.StatusBadRequest)
		return
	}
	if len(channels) > 100 {
		http.Error(w, `{"error":"max 100 channels per request"}`, http.StatusBadRequest)
		return
	}

	for _, chName := range channels {
		if err := proto.ValidateChannelName(chName); err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
	}

	mgr, ok := h.Managers[a.ID]
	if !ok {
		http.Error(w, `{"error":"app not initialized"}`, http.StatusInternalServerError)
		return
	}

	response := make(map[string]any)
	channelsInfo := make(map[string]any)

	for _, chName := range channels {
		event := proto.NewRawEvent(req.Name, chName, req.Data)

		ch, exists := mgr.GetChannel(chName)
		if exists {
			ch.Broadcast(event, req.SocketID)
		}

		if req.Info != "" {
			info := make(map[string]any)
			if exists {
				info["subscription_count"] = ch.SubscriberCount()
				if ch.Type == channel.TypePresence {
					info["user_count"] = ch.UserCount()
				}
			}
			channelsInfo[chName] = info
		}
	}

	if req.Info != "" {
		response["channels"] = channelsInfo
	}

	slog.Info("event triggered", "event", req.Name, "channels", channels, "app_id", a.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *EventsHandler) BatchTrigger(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	a, ok := h.Apps.FindByID(appID)
	if !ok {
		http.Error(w, `{"error":"app not found"}`, http.StatusNotFound)
		return
	}

	if rl, ok := h.BackendRateLimits[a.ID]; ok && !rl.Allow() {
		slog.Warn("backend rate limit exceeded", "security_event", "rate_limited", "app_id", a.ID, "endpoint", "batch_trigger", "remote_addr", r.RemoteAddr)
		http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
		return
	}

	var req BatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if len(req.Batch) > 10 {
		http.Error(w, `{"error":"max 10 events per batch"}`, http.StatusBadRequest)
		return
	}

	mgr, ok := h.Managers[a.ID]
	if !ok {
		http.Error(w, `{"error":"app not initialized"}`, http.StatusInternalServerError)
		return
	}

	for _, trigger := range req.Batch {
		if err := proto.ValidateEventName(trigger.Name); err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

		channels := trigger.Channels
		if len(channels) == 0 && trigger.Channel != "" {
			channels = []string{trigger.Channel}
		}
		for _, chName := range channels {
			if err := proto.ValidateChannelName(chName); err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
				return
			}
			event := proto.NewRawEvent(trigger.Name, chName, trigger.Data)
			if ch, exists := mgr.GetChannel(chName); exists {
				ch.Broadcast(event, trigger.SocketID)
			}
		}
	}

	slog.Info("batch events triggered", "count", len(req.Batch), "app_id", a.ID)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{}"))
}
