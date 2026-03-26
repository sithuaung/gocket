package app

import "sync"

type Registry struct {
	mu    sync.RWMutex
	byKey map[string]*App
	byID  map[string]*App
}

func NewRegistry() *Registry {
	return &Registry{
		byKey: make(map[string]*App),
		byID:  make(map[string]*App),
	}
}

func (r *Registry) Add(a *App) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byKey[a.Key] = a
	r.byID[a.ID] = a
}

func (r *Registry) FindByKey(key string) (*App, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.byKey[key]
	return a, ok
}

func (r *Registry) FindByID(id string) (*App, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.byID[id]
	return a, ok
}

func (r *Registry) All() []*App {
	r.mu.RLock()
	defer r.mu.RUnlock()
	apps := make([]*App, 0, len(r.byID))
	for _, a := range r.byID {
		apps = append(apps, a)
	}
	return apps
}
