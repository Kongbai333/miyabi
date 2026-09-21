package tasks

import "sync"

// Revisions are monotonic change counters broadcast to SSE clients so the
// frontend can decide which queries to refetch.
type TaskRevisions struct {
	Library uint64 `json:"library"`
	Offline uint64 `json:"offline"`
	History uint64 `json:"history"`
	Monitor uint64 `json:"monitor"`
}

// Change is a bitmask naming which revision counters an event bumps.
type Change uint8

const (
	ChangeLibrary Change = 1 << iota
	ChangeOffline
	ChangeHistory
	ChangeMonitor
)

// Bus fans task notifications out to the worker pool and SSE subscribers.
type Bus struct {
	mu          sync.Mutex
	revisions   TaskRevisions
	wake        chan struct{}
	subscribers map[chan struct{}]struct{}
}

func NewBus() *Bus {
	return &Bus{wake: make(chan struct{}, 1), subscribers: make(map[chan struct{}]struct{})}
}

// Pending signals the pool that queued work may exist.
func (b *Bus) Pending() <-chan struct{} { return b.wake }

// Subscribe registers an SSE listener. Notifications are coalesced.
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

// Notify wakes the pool and subscribers without bumping any revision.
func (b *Bus) Notify() { b.publish(0) }

// Changed bumps the named revisions and notifies subscribers. The pool is
// woken only for library or offline changes, which may have queued work;
// history and monitor changes never do.
func (b *Bus) Changed(change Change) { b.publish(change) }

func (b *Bus) NotifyLibraryChanged()      { b.publish(ChangeLibrary | ChangeHistory) }
func (b *Bus) NotifyOfflineChanged()      { b.publish(ChangeOffline) }
func (b *Bus) NotifyWatchHistoryChanged() { b.publish(ChangeHistory) }
func (b *Bus) NotifyMonitorChanged()      { b.publish(ChangeMonitor) }

func (b *Bus) Revisions() TaskRevisions {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.revisions
}

func (b *Bus) publish(change Change) {
	if change == 0 || change&(ChangeLibrary|ChangeOffline) != 0 {
		select {
		case b.wake <- struct{}{}:
		default:
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if change&ChangeLibrary != 0 {
		b.revisions.Library++
	}
	if change&ChangeOffline != 0 {
		b.revisions.Offline++
	}
	if change&ChangeHistory != 0 {
		b.revisions.History++
	}
	if change&ChangeMonitor != 0 {
		b.revisions.Monitor++
	}
	for subscriber := range b.subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}
