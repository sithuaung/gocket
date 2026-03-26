package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/sithuaung/gocket/internal/app"
	"github.com/sithuaung/gocket/internal/auth"
)

type EventType string

const (
	ChannelOccupied EventType = "channel_occupied"
	ChannelVacated  EventType = "channel_vacated"
	MemberAdded     EventType = "member_added"
	MemberRemoved   EventType = "member_removed"
	ClientEvent     EventType = "client_event"
)

type HookEvent struct {
	Name    EventType `json:"name"`
	Channel string    `json:"channel"`
	UserID  string    `json:"user_id,omitempty"`
	Event   string    `json:"event,omitempty"`
	Data    string    `json:"data,omitempty"`
}

type Payload struct {
	TimeMS int64       `json:"time_ms"`
	Events []HookEvent `json:"events"`
}

type Dispatcher struct {
	app    *app.App
	client *http.Client
	queue  chan HookEvent
}

func NewDispatcher(a *app.App) *Dispatcher {
	return &Dispatcher{
		app: a,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		queue: make(chan HookEvent, 256),
	}
}

func (d *Dispatcher) Enqueue(event HookEvent) {
	if d.app.WebhookURL == "" {
		return
	}
	select {
	case d.queue <- event:
	default:
		slog.Warn("webhook queue full, dropping event", "event", event.Name, "app_id", d.app.ID)
	}
}

func (d *Dispatcher) Start(ctx context.Context) {
	if d.app.WebhookURL == "" {
		return
	}

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		var batch []HookEvent

		for {
			select {
			case <-ctx.Done():
				// Flush remaining
				if len(batch) > 0 {
					d.send(batch)
				}
				return
			case event := <-d.queue:
				batch = append(batch, event)
				if len(batch) >= 10 {
					d.send(batch)
					batch = nil
				}
			case <-ticker.C:
				if len(batch) > 0 {
					d.send(batch)
					batch = nil
				}
			}
		}
	}()
}

func (d *Dispatcher) send(events []HookEvent) {
	payload := Payload{
		TimeMS: time.Now().UnixMilli(),
		Events: events,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal webhook payload", "error", err)
		return
	}

	signature := auth.HmacSHA256Hex(d.app.Secret, string(body))

	req, err := http.NewRequest("POST", d.app.WebhookURL, bytes.NewReader(body))
	if err != nil {
		slog.Error("failed to create webhook request", "error", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pusher-Key", d.app.Key)
	req.Header.Set("X-Pusher-Signature", signature)

	resp, err := d.client.Do(req)
	if err != nil {
		slog.Warn("webhook delivery failed", "error", err, "url", d.app.WebhookURL)
		return
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		slog.Warn("webhook returned error", "status", resp.StatusCode, "url", d.app.WebhookURL)
	}
}
