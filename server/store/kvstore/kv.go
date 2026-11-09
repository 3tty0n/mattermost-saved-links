// Package kvstore persists tasks and bookmarks in the Mattermost plugin KV store and maintains
// compact per-scope indexes for list views. It never scans keys: every query reads a bounded set
// of index shards for the requested team, channel or user.
package kvstore

import (
	"errors"
	"sync"

	"github.com/mattermost/mattermost/server/public/pluginapi"
)

// ErrNoChange tells Update to skip the write. Compare-and-set implementations treat it as a
// successful no-op, which avoids rewriting unchanged index records.
var ErrNoChange = errors.New("no change")

// ErrNotFound is returned when a full record does not exist.
var ErrNotFound = errors.New("record not found")

// ErrExists is returned when a create would overwrite an existing record.
var ErrExists = errors.New("record already exists")

// KV is the narrow slice of the plugin KV store this package needs. Keeping it an interface lets
// tests use an in-memory implementation without mocking the whole Mattermost API.
type KV interface {
	Get(key string, out any) error
	Set(key string, value any) (bool, error)
	CompareAndSet(key string, oldValue []byte, newValue any) (bool, error)
	Update(key string, fn func(oldValue []byte) (newValue any, err error)) error
	Delete(key string) error
}

// pluginKV adapts pluginapi.KVService to KV.
type pluginKV struct {
	client *pluginapi.Client
}

func (p pluginKV) Get(key string, out any) error {
	return p.client.KV.Get(key, out)
}

func (p pluginKV) Set(key string, value any) (bool, error) {
	return p.client.KV.Set(key, value)
}

func (p pluginKV) CompareAndSet(key string, oldValue []byte, newValue any) (bool, error) {
	return p.client.KV.Set(key, newValue, pluginapi.SetAtomic(oldValue))
}

func (p pluginKV) Update(key string, fn func(oldValue []byte) (any, error)) error {
	err := p.client.KV.SetAtomicWithRetries(key, func(oldValue []byte) (any, error) {
		return fn(oldValue)
	})
	if errors.Is(err, ErrNoChange) {
		return nil
	}
	return err
}

func (p pluginKV) Delete(key string) error {
	return p.client.KV.Delete(key)
}

// memoryKV is a small in-memory KV implementation for unit tests and benchmarks.
type memoryKV struct {
	mu   sync.RWMutex
	data map[string][]byte
}

// NewMemoryKV returns an empty in-memory KV.
func NewMemoryKV() KV {
	return &memoryKV{data: make(map[string][]byte)}
}

func (m *memoryKV) Get(key string, out any) error {
	m.mu.RLock()
	raw, ok := m.data[key]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return unmarshalValue(raw, out)
}

func (m *memoryKV) Set(key string, value any) (bool, error) {
	raw, err := marshalValue(value)
	if err != nil {
		return false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if raw == nil {
		delete(m.data, key)
	} else {
		m.data[key] = raw
	}
	return true, nil
}

func (m *memoryKV) CompareAndSet(key string, oldValue []byte, newValue any) (bool, error) {
	raw, err := marshalValue(newValue)
	if err != nil {
		return false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.data[key]
	if !bytesEqual(current, oldValue) || (!ok && len(oldValue) > 0) {
		return false, nil
	}
	if raw == nil {
		delete(m.data, key)
	} else {
		m.data[key] = raw
	}
	return true, nil
}

func (m *memoryKV) Update(key string, fn func(oldValue []byte) (any, error)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	oldValue := m.data[key]
	newValue, err := fn(append([]byte(nil), oldValue...))
	if errors.Is(err, ErrNoChange) {
		return nil
	}
	if err != nil {
		return err
	}
	raw, err := marshalValue(newValue)
	if err != nil {
		return err
	}
	if raw == nil {
		delete(m.data, key)
	} else {
		m.data[key] = raw
	}
	return nil
}

func (m *memoryKV) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
