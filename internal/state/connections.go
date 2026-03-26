package state

import (
	"sync"

	"github.com/sithuaung/gocket/internal/ws"
)

type ConnectionManager struct {
	mu       sync.RWMutex
	conns    map[string]*ws.Conn
	maxConns int
}

func NewConnectionManager(maxConns int) *ConnectionManager {
	return &ConnectionManager{
		conns:    make(map[string]*ws.Conn),
		maxConns: maxConns,
	}
}

// Add adds a connection. Returns false if the max connection limit has been reached.
func (cm *ConnectionManager) Add(conn *ws.Conn) bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.maxConns > 0 && len(cm.conns) >= cm.maxConns {
		return false
	}
	cm.conns[conn.SocketID()] = conn
	return true
}

func (cm *ConnectionManager) Remove(socketID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.conns, socketID)
}

func (cm *ConnectionManager) Count() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.conns)
}
