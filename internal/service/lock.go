package service

import (
	"context"
	"sync"
)

// contextLock has a usable zero value and lets waiting callers leave promptly.
type contextLock struct {
	init sync.Once
	gate chan struct{}
}

func (lock *contextLock) Lock(ctx context.Context) error {
	lock.init.Do(func() { lock.gate = make(chan struct{}, 1) })
	select {
	case lock.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			lock.Unlock()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (lock *contextLock) TryLock() bool {
	lock.init.Do(func() { lock.gate = make(chan struct{}, 1) })
	select {
	case lock.gate <- struct{}{}:
		return true
	default:
		return false
	}
}

func (lock *contextLock) Unlock() { <-lock.gate }
