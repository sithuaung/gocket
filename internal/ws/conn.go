package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"sync"

	"github.com/coder/websocket"
	"github.com/sithuaung/gocket/internal/proto"
	"github.com/sithuaung/gocket/internal/ratelimit"
)

type Conn struct {
	wsMu            sync.Mutex
	ws              *websocket.Conn
	socketID        string
	AppID           string
	AppKey          string
	chMu            sync.RWMutex
	channels        map[string]struct{}
	ctx             context.Context
	cancel          context.CancelFunc
	ClientEventRate *ratelimit.Limiter
}

func NewConn(ctx context.Context, ws *websocket.Conn, appID, appKey string, maxMessageSize int64, maxClientEventsPerSec int) *Conn {
	if maxMessageSize > 0 {
		ws.SetReadLimit(maxMessageSize)
	}
	ctx, cancel := context.WithCancel(ctx)
	return &Conn{
		ws:              ws,
		socketID:        generateSocketID(),
		AppID:           appID,
		AppKey:          appKey,
		channels:        make(map[string]struct{}),
		ctx:             ctx,
		cancel:          cancel,
		ClientEventRate: ratelimit.NewLimiter(maxClientEventsPerSec),
	}
}

func (c *Conn) SocketID() string {
	return c.socketID
}

func (c *Conn) Context() context.Context {
	return c.ctx
}

func (c *Conn) Send(ctx context.Context, event proto.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	c.wsMu.Lock()
	defer c.wsMu.Unlock()
	return c.ws.Write(ctx, websocket.MessageText, data)
}

// SendEvent implements proto.Subscriber.
func (c *Conn) SendEvent(event proto.Event) error {
	return c.Send(c.ctx, event)
}

func (c *Conn) Read(ctx context.Context) (proto.Event, error) {
	_, data, err := c.ws.Read(ctx)
	if err != nil {
		return proto.Event{}, err
	}
	var event proto.Event
	if err := json.Unmarshal(data, &event); err != nil {
		return proto.Event{}, err
	}
	return event, nil
}

func (c *Conn) Close(code websocket.StatusCode, reason string) error {
	c.cancel()
	return c.ws.Close(code, reason)
}

// AddChannel marks this connection as subscribed to a channel.
func (c *Conn) AddChannel(name string) {
	c.chMu.Lock()
	defer c.chMu.Unlock()
	c.channels[name] = struct{}{}
}

// RemoveChannel removes a channel subscription from this connection.
func (c *Conn) RemoveChannel(name string) {
	c.chMu.Lock()
	defer c.chMu.Unlock()
	delete(c.channels, name)
}

// HasChannel reports whether this connection is subscribed to the named channel.
func (c *Conn) HasChannel(name string) bool {
	c.chMu.RLock()
	defer c.chMu.RUnlock()
	_, ok := c.channels[name]
	return ok
}

// ChannelCount returns the number of channels this connection is subscribed to.
func (c *Conn) ChannelCount() int {
	c.chMu.RLock()
	defer c.chMu.RUnlock()
	return len(c.channels)
}

// ChannelNames returns a snapshot of all subscribed channel names.
func (c *Conn) ChannelNames() []string {
	c.chMu.RLock()
	defer c.chMu.RUnlock()
	names := make([]string, 0, len(c.channels))
	for name := range c.channels {
		names = append(names, name)
	}
	return names
}

func generateSocketID() string {
	return fmt.Sprintf("%d.%d", rand.Int64N(1_000_000_000), rand.Int64N(1_000_000_000))
}
