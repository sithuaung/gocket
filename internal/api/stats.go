package api

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/sithuaung/gocket/internal/app"
	"github.com/sithuaung/gocket/internal/channel"
	"github.com/sithuaung/gocket/internal/state"
)

var startTime = time.Now()

type AppStats struct {
	ID          string         `json:"id"`
	Connections int            `json:"connections"`
	Channels    int            `json:"channels"`
	TopChannels []ChannelStats `json:"top_channels,omitempty"`
}

type ChannelStats struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Subscribers int    `json:"subscribers"`
}

type StatsResponse struct {
	Uptime         string     `json:"uptime"`
	GoRoutines     int        `json:"goroutines"`
	MemAllocMB     float64    `json:"mem_alloc_mb"`
	MemSysMB       float64    `json:"mem_sys_mb"`
	TotalConns     int        `json:"total_connections"`
	TotalChannels  int        `json:"total_channels"`
	Apps           []AppStats `json:"apps"`
}

type StatsHandler struct {
	Apps        *app.Registry
	Managers    map[string]*channel.Manager
	Connections map[string]*state.ConnectionManager
}

func (h *StatsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	totalConns := 0
	totalChannels := 0
	var apps []AppStats

	for _, a := range h.Apps.All() {
		connCount := 0
		if cm, ok := h.Connections[a.ID]; ok {
			connCount = cm.Count()
		}
		totalConns += connCount

		var channelCount int
		var topChannels []ChannelStats
		if mgr, ok := h.Managers[a.ID]; ok {
			occupied := mgr.GetOccupiedChannels("")
			channelCount = len(occupied)
			for _, ch := range occupied {
				topChannels = append(topChannels, ChannelStats{
					Name:        ch.Name,
					Type:        channelTypeName(ch.Type),
					Subscribers: ch.SubscriberCount(),
				})
			}
		}
		totalChannels += channelCount

		apps = append(apps, AppStats{
			ID:          a.ID,
			Connections: connCount,
			Channels:    channelCount,
			TopChannels: topChannels,
		})
	}

	resp := StatsResponse{
		Uptime:        time.Since(startTime).Round(time.Second).String(),
		GoRoutines:    runtime.NumGoroutine(),
		MemAllocMB:    float64(memStats.Alloc) / 1024 / 1024,
		MemSysMB:      float64(memStats.Sys) / 1024 / 1024,
		TotalConns:    totalConns,
		TotalChannels: totalChannels,
		Apps:          apps,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func channelTypeName(t channel.Type) string {
	switch t {
	case channel.TypePrivate:
		return "private"
	case channel.TypePresence:
		return "presence"
	case channel.TypeEncrypted:
		return "encrypted"
	default:
		return "public"
	}
}
