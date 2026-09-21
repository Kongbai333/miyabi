package drive

import (
	"context"
	"sync"

	"github.com/ppxb/miyabi/internal/domain"
)

// MountEvent signals that a media directory has been mounted or cleared.
type MountEvent struct {
	Source domain.LibrarySource
}

// MountListener receives mount notifications.
type MountListener func(ctx context.Context, event MountEvent) error

type eventBus struct {
	mu        sync.RWMutex
	nextID    uint64
	listeners map[uint64]MountListener
}

func newEventBus() *eventBus {
	return &eventBus{listeners: make(map[uint64]MountListener)}
}

func (b *eventBus) subscribe(listener MountListener) func() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	id := b.nextID
	b.listeners[id] = listener
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.listeners, id)
	}
}

func (b *eventBus) publishMount(ctx context.Context, source domain.LibrarySource) error {
	b.mu.RLock()
	active := make([]MountListener, 0, len(b.listeners))
	for _, l := range b.listeners {
		active = append(active, l)
	}
	b.mu.RUnlock()

	event := MountEvent{Source: source}
	for _, listener := range active {
		if err := listener(ctx, event); err != nil {
			return err
		}
	}
	return nil
}
