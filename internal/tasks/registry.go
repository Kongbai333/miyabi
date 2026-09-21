package tasks

import (
	"context"
	"encoding/json"
	"slices"
	"sync"

	"github.com/ppxb/miyabi/internal/ent"
)

// Job is internal execution input. API responses never expose raw payloads.
type Job struct {
	ID      int             `json:"id"`
	Type    Kind            `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Handler executes jobs of one Kind.
type Handler interface {
	Kind() Kind
	Handle(ctx context.Context, job Job) error
}

// FinishedHook runs inside the completion transaction after the task row is
// updated. It returns which revisions the completed job changed; the queue
// publishes them once the transaction commits.
type FinishedHook interface {
	Finished(ctx context.Context, tx *ent.Tx, job Job, result error) (Change, error)
}

type HandleFunc func(ctx context.Context, job Job) error
type FinishedFunc func(ctx context.Context, tx *ent.Tx, job Job, result error) (Change, error)

type funcHandler struct {
	kind   Kind
	handle HandleFunc
}

func (h funcHandler) Kind() Kind                                { return h.kind }
func (h funcHandler) Handle(ctx context.Context, job Job) error { return h.handle(ctx, job) }

type funcHookHandler struct {
	funcHandler
	finished FinishedFunc
}

func (h funcHookHandler) Finished(ctx context.Context, tx *ent.Tx, job Job, result error) (Change, error) {
	return h.finished(ctx, tx, job, result)
}

// NewHandler adapts plain functions to Handler. A nil finished hook means the
// handler has nothing to do at completion and bumps no revision.
func NewHandler(kind Kind, handle HandleFunc, finished FinishedFunc) Handler {
	base := funcHandler{kind: kind, handle: handle}
	if finished == nil {
		return base
	}
	return funcHookHandler{funcHandler: base, finished: finished}
}

// Registry maps task kinds to their handlers.
type Registry struct {
	mu       sync.RWMutex
	handlers map[Kind]Handler
}

func NewRegistry() *Registry {
	return &Registry{handlers: make(map[Kind]Handler)}
}

// Register installs a handler, replacing any previous one for the same Kind.
func (r *Registry) Register(h Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[h.Kind()] = h
}

func (r *Registry) Get(k Kind) (Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[k]
	return h, ok
}

// Kinds returns registered kinds in sorted order.
func (r *Registry) Kinds() []Kind {
	r.mu.RLock()
	defer r.mu.RUnlock()
	kinds := make([]Kind, 0, len(r.handlers))
	for k := range r.handlers {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	return kinds
}
