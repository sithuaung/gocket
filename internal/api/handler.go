package api

import (
	"net/http"

	"github.com/sithuaung/gocket/internal/app"
	"github.com/sithuaung/gocket/internal/channel"
	"github.com/sithuaung/gocket/internal/ratelimit"
)

func NewRouter(apps *app.Registry, managers map[string]*channel.Manager, allowedOrigins []string) http.Handler {
	backendLimits := make(map[string]*ratelimit.Limiter)
	for _, a := range apps.All() {
		backendLimits[a.ID] = ratelimit.NewLimiter(a.MaxBackendEventsPerSec)
	}

	events := &EventsHandler{Apps: apps, Managers: managers, BackendRateLimits: backendLimits}
	channels := &ChannelsHandler{Apps: apps, Managers: managers}

	mux := http.NewServeMux()

	authMw := AuthMiddleware(apps)
	corsMw := CORSMiddleware(allowedOrigins)

	mux.Handle("POST /apps/{appId}/events", corsMw(authMw(http.HandlerFunc(events.TriggerEvent))))
	mux.Handle("POST /apps/{appId}/batch_events", corsMw(authMw(http.HandlerFunc(events.BatchTrigger))))
	mux.Handle("GET /apps/{appId}/channels", corsMw(authMw(http.HandlerFunc(channels.ListChannels))))
	mux.Handle("GET /apps/{appId}/channels/{channelName}", corsMw(authMw(http.HandlerFunc(channels.GetChannel))))
	mux.Handle("GET /apps/{appId}/channels/{channelName}/users", corsMw(authMw(http.HandlerFunc(channels.GetUsers))))

	return mux
}
