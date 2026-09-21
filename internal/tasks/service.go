package tasks

import (
	"context"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
)

// Service provides the unified task execution, queueing, and workflow inspection API.
type Service struct {
	database *ent.Client
	queue    *Queue
	bus      *Bus
	registry *Registry
}

// NewService creates a new tasks Service.
func NewService(database *ent.Client, registry *Registry) *Service {
	bus := NewBus()
	queue := NewQueue(database, registry, bus)
	return &Service{
		database: database,
		queue:    queue,
		bus:      bus,
		registry: registry,
	}
}

// Database returns the underlying database client.
func (s *Service) Database() *ent.Client {
	return s.database
}

// Queue returns the underlying queue coordinator.
func (s *Service) Queue() *Queue {
	return s.queue
}

// Bus returns the notification and revision bus.
func (s *Service) Bus() *Bus {
	return s.bus
}

// Registry returns the handler registry.
func (s *Service) Registry() *Registry {
	return s.registry
}

// List returns the active and recent workflows for API presentation.
func (s *Service) List(ctx context.Context) ([]TaskInfo, error) {
	return ListWorkflows(ctx, s.database)
}

// Info returns the status of a single scan workflow.
func (s *Service) Info(ctx context.Context, id int) (TaskInfo, error) {
	return WorkflowInfo(ctx, s.database, id)
}

// Workflows folds child tasks into their parent scan workflow summaries.
func (s *Service) Workflows(ctx context.Context, records []*ent.Task) ([]TaskInfo, error) {
	return WorkflowInfos(ctx, s.database, records)
}

// EnqueueScan enqueues or reuses a scan task for the given source while holding the queue lock.
func (s *Service) EnqueueScan(ctx context.Context, source domain.LibrarySource) (TaskInfo, error) {
	if err := s.queue.Lock(ctx); err != nil {
		return TaskInfo{}, err
	}
	defer s.queue.Unlock()
	record, err := EnsureScanTask(ctx, s.database.Task, source, task.StatusQueued, task.StatusRunning)
	if err != nil {
		return TaskInfo{}, err
	}
	s.bus.Notify()
	return ScanTaskInfo(record)
}

// Claim claims a task of the requested kinds from the queue.
func (s *Service) Claim(ctx context.Context, kinds []Kind) (*Job, error) {
	return s.queue.Claim(ctx, kinds)
}

// Finish marks a task completed or failed and triggers the finished hook.
func (s *Service) Finish(ctx context.Context, id int, runError error) error {
	return s.queue.Finish(ctx, id, runError)
}

// Recover resets interrupted running tasks back to queued.
func (s *Service) Recover(ctx context.Context, kinds []Kind) error {
	return s.queue.Recover(ctx, kinds)
}

// Subscribe returns an updates channel for SSE clients.
func (s *Service) Subscribe() (<-chan struct{}, func()) {
	return s.bus.Subscribe()
}

// Revisions returns the current task revisions.
func (s *Service) Revisions() TaskRevisions {
	return s.bus.Revisions()
}

// Notify triggers a general wake notification.
func (s *Service) Notify() {
	s.bus.Notify()
}

// NotifyLibraryChanged triggers a notification with library revision increment.
func (s *Service) NotifyLibraryChanged() {
	s.bus.NotifyLibraryChanged()
}

// NotifyOfflineChanged triggers a notification with offline revision increment.
func (s *Service) NotifyOfflineChanged() {
	s.bus.NotifyOfflineChanged()
}

// NotifyWatchHistoryChanged triggers a notification with history revision increment.
func (s *Service) NotifyWatchHistoryChanged() {
	s.bus.NotifyWatchHistoryChanged()
}

// NotifyMonitorChanged triggers a notification with monitor revision increment without waking the pool.
func (s *Service) NotifyMonitorChanged() {
	s.bus.NotifyMonitorChanged()
}

// Pending returns a wake channel indicating that pending tasks are available.
func (s *Service) Pending() <-chan struct{} {
	return s.bus.Pending()
}

// LockQueue acquires the queue gate.
func (s *Service) LockQueue(ctx context.Context) error {
	return s.queue.Lock(ctx)
}

// UnlockQueue releases the queue gate.
func (s *Service) UnlockQueue() {
	s.queue.Unlock()
}
