package channel

import (
	"encoding/json"
	"log/slog"
	"strings"
	"sync"

	"github.com/sithuaung/gocket/internal/proto"
)

type Type int

const (
	TypePublic Type = iota
	TypePrivate
	TypePresence
	TypeEncrypted
)

func ParseType(name string) Type {
	switch {
	case strings.HasPrefix(name, "private-encrypted-"):
		return TypeEncrypted
	case strings.HasPrefix(name, "presence-"):
		return TypePresence
	case strings.HasPrefix(name, "private-"):
		return TypePrivate
	default:
		return TypePublic
	}
}

type PresenceMember struct {
	UserID   string          `json:"user_id"`
	UserInfo json.RawMessage `json:"user_info"`
}

type Channel struct {
	mu          sync.RWMutex
	Name        string
	Type        Type
	subscribers map[string]proto.Subscriber // socketID -> subscriber
	members     map[string]PresenceMember   // userID -> member (presence only)
	memberConn  map[string]string           // socketID -> userID
}

func New(name string) *Channel {
	return &Channel{
		Name:        name,
		Type:        ParseType(name),
		subscribers: make(map[string]proto.Subscriber),
		members:     make(map[string]PresenceMember),
		memberConn:  make(map[string]string),
	}
}

func (ch *Channel) Subscribe(conn proto.Subscriber, member *PresenceMember) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	socketID := conn.SocketID()
	ch.subscribers[socketID] = conn
	conn.AddChannel(ch.Name)

	if ch.Type == TypePresence && member != nil {
		ch.members[member.UserID] = *member
		ch.memberConn[socketID] = member.UserID

		event, err := proto.NewEvent("pusher_internal:member_added", ch.Name, member)
		if err == nil {
			ch.broadcastLocked(event, socketID)
		}
	}
}

func (ch *Channel) Unsubscribe(socketID string) *PresenceMember {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	conn, ok := ch.subscribers[socketID]
	if !ok {
		return nil
	}

	conn.RemoveChannel(ch.Name)
	delete(ch.subscribers, socketID)

	if ch.Type == TypePresence {
		if userID, ok := ch.memberConn[socketID]; ok {
			delete(ch.memberConn, socketID)
			member := ch.members[userID]
			delete(ch.members, userID)

			event, err := proto.NewEvent("pusher_internal:member_removed", ch.Name, map[string]string{"user_id": userID})
			if err == nil {
				ch.broadcastLocked(event, "")
			}
			return &member
		}
	}

	return nil
}

func (ch *Channel) Broadcast(event proto.Event, excludeSocketID string) {
	ch.mu.RLock()
	targets := make([]proto.Subscriber, 0, len(ch.subscribers))
	for socketID, conn := range ch.subscribers {
		if socketID != excludeSocketID {
			targets = append(targets, conn)
		}
	}
	ch.mu.RUnlock()

	for _, conn := range targets {
		go func(c proto.Subscriber) {
			if err := c.SendEvent(event); err != nil {
				slog.Warn("failed to send to subscriber", "error", err)
			}
		}(conn)
	}
}

func (ch *Channel) broadcastLocked(event proto.Event, excludeSocketID string) {
	for socketID, conn := range ch.subscribers {
		if socketID == excludeSocketID {
			continue
		}
		if err := conn.SendEvent(event); err != nil {
			slog.Warn("failed to send to subscriber", "socket_id", socketID, "error", err)
		}
	}
}

func (ch *Channel) SubscriberCount() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return len(ch.subscribers)
}

func (ch *Channel) PresenceMembers() []PresenceMember {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	members := make([]PresenceMember, 0, len(ch.members))
	for _, m := range ch.members {
		members = append(members, m)
	}
	return members
}

func (ch *Channel) UserCount() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return len(ch.members)
}

func (ch *Channel) IsEmpty() bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return len(ch.subscribers) == 0
}

func (ch *Channel) SubscriptionSucceededEvent() (proto.Event, error) {
	if ch.Type == TypePresence {
		ch.mu.RLock()
		ids := make([]string, 0, len(ch.members))
		hash := make(map[string]json.RawMessage, len(ch.members))
		for userID, m := range ch.members {
			ids = append(ids, userID)
			hash[userID] = m.UserInfo
		}
		count := len(ch.members)
		ch.mu.RUnlock()

		data := map[string]any{
			"presence": map[string]any{
				"ids":   ids,
				"hash":  hash,
				"count": count,
			},
		}
		return proto.NewEvent("pusher_internal:subscription_succeeded", ch.Name, data)
	}

	return proto.NewEvent("pusher_internal:subscription_succeeded", ch.Name, struct{}{})
}
