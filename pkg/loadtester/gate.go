package loadtester

import "sync"

type GateStorage struct {
	backend string
	data    *sync.Map
}

func NewGateStorage(backend string) *GateStorage {
	return &GateStorage{
		backend: backend,
		data:    new(sync.Map),
	}
}

func (gs *GateStorage) open(key string) {
	gs.data.Store(key, true)
}

func (gs *GateStorage) close(key string) {
	gs.data.Store(key, false)
}

func (gs *GateStorage) isOpen(key string) (locked bool) {
	val, ok := gs.data.LoadOrStore(key, false)
	if ok {
		return val.(bool)
	}
	return
}
