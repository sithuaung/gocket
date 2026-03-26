package channel

import (
	"sync"

	"github.com/sithuaung/gocket/internal/proto"
)

type Manager struct {
	mu       sync.RWMutex
	channels map[string]*Channel
}

func NewManager() *Manager {
	return &Manager{
		channels: make(map[string]*Channel),
	}
}

func (m *Manager) Subscribe(conn proto.Subscriber, channelName string, member *PresenceMember) *Channel {
	m.mu.Lock()
	ch, exists := m.channels[channelName]
	if !exists {
		ch = New(channelName)
		m.channels[channelName] = ch
	}
	m.mu.Unlock()

	ch.Subscribe(conn, member)

	return ch
}

func (m *Manager) Unsubscribe(socketID string, channelName string) (channelVacated bool, removedMember *PresenceMember) {
	m.mu.RLock()
	ch, exists := m.channels[channelName]
	m.mu.RUnlock()

	if !exists {
		return false, nil
	}

	removedMember = ch.Unsubscribe(socketID)

	if ch.IsEmpty() {
		m.mu.Lock()
		if ch.IsEmpty() {
			delete(m.channels, channelName)
		}
		m.mu.Unlock()
		return true, removedMember
	}

	return false, removedMember
}

func (m *Manager) RemoveConnection(socketID string, channelNames []string) {
	for _, ch := range channelNames {
		m.Unsubscribe(socketID, ch)
	}
}

func (m *Manager) GetChannel(name string) (*Channel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ch, ok := m.channels[name]
	return ch, ok
}

func (m *Manager) GetOccupiedChannels(prefix string) []*Channel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*Channel
	for name, ch := range m.channels {
		if prefix == "" || len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			result = append(result, ch)
		}
	}
	return result
}
