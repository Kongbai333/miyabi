package tasks

import (
	"sync"
)

// TaskRevisions tracks monotonic change counters broadcasted to clients.
type TaskRevisions struct {
	Library uint64 `json:"library"`
	Offline uint64 `json:"offline"`
	History uint64 `json:"history"`
	Monitor uint64 `json:"monitor"`
}

// Bus distributes task state notifications and revision counts to workers and SSE clients.
type Bus struct {
	revisions   TaskRevisions
	wake        chan struct{}
	mu          sync.Mutex
	subscribers map[chan struct{}]struct{}
}

// NewBus creates a new Bus instance.
func NewBus() *Bus {
	return &Bus{
		wake:        make(chan struct{}, 1),
		subscribers: make(map[chan struct{}]struct{}),
	}
}

// Pending returns a wake channel indicating that pending tasks are available.
func (b *Bus) Pending() <-chan struct{} {
	return b.wake
}

// Subscribe registers a channel to receive notifications. Returns the channel and an unsubscribe func.
func (b *Bus) Subscribe() (<-chan struct{}, func()) {
	updates := make(chan struct{}, 1)
	b.mu.Lock()
	b.subscribers[updates] = struct{}{}
	b.mu.Unlock()
	return updates, func() {
		b.mu.Lock()
		delete(b.subscribers, updates)
		b.mu.Unlock()
	}
}

// Notify triggers a general wake notification.
func (b *Bus) Notify() {
	b.notify(false, false, false)
}

// NotifyLibraryChanged triggers a notification with library revision increment.
func (b *Bus) NotifyLibraryChanged() {
	b.notify(true, false, true)
}

// NotifyOfflineChanged triggers a notification with offline revision increment.
func (b *Bus) NotifyOfflineChanged() {
	b.notify(false, true, false)
}

// NotifyWatchHistoryChanged triggers a notification with history revision increment.
func (b *Bus) NotifyWatchHistoryChanged() {
	b.notify(false, false, true)
}

// NotifyMonitorChanged triggers a notification with monitor revision increment without waking the pool.
func (b *Bus) NotifyMonitorChanged() {
	b.mu.Lock()
	b.revisions.Monitor++
	subscribers := make([]chan struct{}, 0, len(b.subscribers))
	for subscriber := range b.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	b.mu.Unlock()
	for _, subscriber := range subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

// Revisions returns the current revision counters.
func (b *Bus) Revisions() TaskRevisions {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.revisions
}

func (b *Bus) notify(library, offline, history bool) {
	if !history || library || offline {
		select {
		case b.wake <- struct{}{}:
		default:
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if library {
		b.revisions.Library++
	}
	if offline {
		b.revisions.Offline++
	}
	if history {
		b.revisions.History++
	}
	for subscriber := range b.subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}
