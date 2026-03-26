package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sithuaung/gocket/internal/app"
	"github.com/sithuaung/gocket/internal/channel"
)

type ChannelsHandler struct {
	Apps     *app.Registry
	Managers map[string]*channel.Manager
}

func (h *ChannelsHandler) ListChannels(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	a, ok := h.Apps.FindByID(appID)
	if !ok {
		http.Error(w, `{"error":"app not found"}`, http.StatusNotFound)
		return
	}

	mgr, ok := h.Managers[a.ID]
	if !ok {
		http.Error(w, `{"error":"app not initialized"}`, http.StatusInternalServerError)
		return
	}

	prefix := r.URL.Query().Get("filter_by_prefix")
	info := r.URL.Query().Get("info")

	channels := mgr.GetOccupiedChannels(prefix)
	result := make(map[string]any, len(channels))

	for _, ch := range channels {
		chInfo := make(map[string]any)
		if strings.Contains(info, "user_count") && ch.Type == channel.TypePresence {
			chInfo["user_count"] = ch.UserCount()
		}
		if strings.Contains(info, "subscription_count") {
			chInfo["subscription_count"] = ch.SubscriberCount()
		}
		result[ch.Name] = chInfo
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"channels": result})
}

func (h *ChannelsHandler) GetChannel(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	a, ok := h.Apps.FindByID(appID)
	if !ok {
		http.Error(w, `{"error":"app not found"}`, http.StatusNotFound)
		return
	}

	mgr, ok := h.Managers[a.ID]
	if !ok {
		http.Error(w, `{"error":"app not initialized"}`, http.StatusInternalServerError)
		return
	}

	channelName := r.PathValue("channelName")
	ch, exists := mgr.GetChannel(channelName)
	if !exists {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"occupied": false})
		return
	}

	info := r.URL.Query().Get("info")
	result := map[string]any{"occupied": true}

	if strings.Contains(info, "subscription_count") {
		result["subscription_count"] = ch.SubscriberCount()
	}
	if strings.Contains(info, "user_count") && ch.Type == channel.TypePresence {
		result["user_count"] = ch.UserCount()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *ChannelsHandler) GetUsers(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	a, ok := h.Apps.FindByID(appID)
	if !ok {
		http.Error(w, `{"error":"app not found"}`, http.StatusNotFound)
		return
	}

	mgr, ok := h.Managers[a.ID]
	if !ok {
		http.Error(w, `{"error":"app not initialized"}`, http.StatusInternalServerError)
		return
	}

	channelName := r.PathValue("channelName")
	ch, exists := mgr.GetChannel(channelName)
	if !exists {
		http.Error(w, `{"error":"channel not found"}`, http.StatusNotFound)
		return
	}

	if ch.Type != channel.TypePresence {
		http.Error(w, `{"error":"not a presence channel"}`, http.StatusBadRequest)
		return
	}

	members := ch.PresenceMembers()
	users := make([]map[string]string, len(members))
	for i, m := range members {
		users[i] = map[string]string{"id": m.UserID}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"users": users})
}
