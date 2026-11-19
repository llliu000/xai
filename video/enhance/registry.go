package enhance

import (
	"errors"
	"fmt"
	"sync"
)

var (
	ErrChannelNotFound          = errors.New("enhancement channel not found")
	ErrChannelAlreadyRegistered = errors.New("enhancement channel already registered")
)

// Registry stores configured adapters by channel name. Multiple channels can use
// the same provider with different configurations. Its zero value is ready to use.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]Adapter)}
}

// Register adds an adapter without replacing an existing channel.
func (r *Registry) Register(name string, adapter Adapter) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.adapters[name]; exists {
		return fmt.Errorf("%w: %q", ErrChannelAlreadyRegistered, name)
	}
	if r.adapters == nil {
		r.adapters = make(map[string]Adapter)
	}
	r.adapters[name] = adapter
	return nil
}

func (r *Registry) GetAdapter(name string) (Adapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	adapter, exists := r.adapters[name]
	if !exists {
		return nil, fmt.Errorf("%w: %q", ErrChannelNotFound, name)
	}
	return adapter, nil
}
