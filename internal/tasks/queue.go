package tasks

import (
	"context"
	"fmt"
	"sync"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
)

// contextLock has a usable zero value and lets waiting callers leave promptly.
type contextLock struct {
	once sync.Once
	gate chan struct{}
}

func (lock *contextLock) Lock(ctx context.Context) error {
	lock.once.Do(func() { lock.gate = make(chan struct{}, 1) })
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

func (lock *contextLock) Unlock() { <-lock.gate }

func (lock *contextLock) TryLock() bool {
	lock.once.Do(func() { lock.gate = make(chan struct{}, 1) })
	select {
	case lock.gate <- struct{}{}:
		return true
	default:
		return false
	}
}

// Queue coordinates task persistence, claiming, and completion transitions.
type Queue struct {
	database *ent.Client
	registry *Registry
	bus      *Bus
	lock     contextLock
}

// NewQueue creates a new task Queue.
func NewQueue(database *ent.Client, registry *Registry, bus *Bus) *Queue {
	return &Queue{
		database: database,
		registry: registry,
		bus:      bus,
	}
}

// Lock acquires the queue gate.
func (q *Queue) Lock(ctx context.Context) error {
	return q.lock.Lock(ctx)
}

// Unlock releases the queue gate.
func (q *Queue) Unlock() {
	q.lock.Unlock()
}

// Recover marks any interrupted running tasks of the given kinds back to queued.
func (q *Queue) Recover(ctx context.Context, kinds []Kind) error {
	types := make([]string, len(kinds))
	for i, k := range kinds {
		types[i] = string(k)
	}
	if _, err := q.database.Task.Update().Where(
		task.TypeIn(types...), task.StatusEQ(task.StatusRunning),
	).SetStatus(task.StatusQueued).SetProgress(0).ClearError().Save(ctx); err != nil {
		return fmt.Errorf("recover interrupted tasks: %w", err)
	}
	q.bus.Notify()
	return nil
}

// Claim claims the next queued task matching the requested kinds and marks it running.
func (q *Queue) Claim(ctx context.Context, kinds []Kind) (*Job, error) {
	if err := q.lock.Lock(ctx); err != nil {
		return nil, err
	}
	defer q.lock.Unlock()
	types := make([]string, len(kinds))
	for i, k := range kinds {
		types[i] = string(k)
	}
	for {
		record, err := q.database.Task.Query().Where(
			task.TypeIn(types...), task.StatusEQ(task.StatusQueued),
		).Order(ent.Asc(task.FieldID)).First(ctx)
		if ent.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("find queued task: %w", err)
		}
		claimed, err := q.database.Task.Update().Where(
			task.IDEQ(record.ID), task.StatusEQ(task.StatusQueued),
		).SetStatus(task.StatusRunning).SetProgress(0).ClearError().Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("claim task %d: %w", record.ID, err)
		}
		if claimed == 0 {
			continue
		}
		q.bus.Notify()
		return &Job{ID: record.ID, Type: Kind(record.Type), Payload: record.Payload}, nil
	}
}

// Finish completes a task record inside a transaction, invoking the Finished hook if registered.
func (q *Queue) Finish(ctx context.Context, id int, runError error) error {
	var job Job
	if err := ent.WithTx(ctx, q.database, func(tx *ent.Tx) error {
		record, err := tx.Task.Get(ctx, id)
		if err != nil {
			return err
		}
		job = Job{ID: record.ID, Type: Kind(record.Type), Payload: record.Payload}
		update := tx.Task.UpdateOneID(id)
		if runError != nil {
			update.SetStatus(task.StatusFailed).SetError(runError.Error())
		} else {
			update.SetStatus(task.StatusDone).SetProgress(100).ClearError()
		}
		if err := update.Exec(ctx); err != nil {
			return err
		}
		if q.registry != nil {
			if handler, ok := q.registry.Get(job.Type); ok {
				if hook, ok := handler.(FinishedHook); ok {
					if err := hook.Finished(ctx, tx, job, runError); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("finish task %d: %w", id, err)
	}
	if (job.Type == KindScrape || job.Type == KindCover) && runError != nil {
		q.bus.NotifyLibraryChanged()
	} else {
		q.bus.NotifyOfflineChanged()
	}
	return nil
}
