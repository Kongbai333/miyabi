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

// Handler handles jobs for a specific Kind.
type Handler interface {
	Kind() Kind
	Handle(ctx context.Context, job Job) error
}

// FinishedHook is an optional interface a Handler can implement to execute inside
// the completion transaction when a job finishes.
type FinishedHook interface {
	Finished(ctx context.Context, tx *ent.Tx, job Job, result error) error
}

type HandleFunc func(ctx context.Context, job Job) error
type FinishedFunc func(ctx context.Context, tx *ent.Tx, job Job, result error) error

type functionalHandler struct {
	kind     Kind
	handle   HandleFunc
	finished FinishedFunc
}

func (h functionalHandler) Kind() Kind {
	return h.kind
}

func (h functionalHandler) Handle(ctx context.Context, job Job) error {
	return h.handle(ctx, job)
}

func (h functionalHandler) Finished(ctx context.Context, tx *ent.Tx, job Job, result error) error {
	if h.finished != nil {
		return h.finished(ctx, tx, job, result)
	}
	return nil
}

// NewHandler creates a Handler with the given Kind, handle function, and optional finished hook.
func NewHandler(kind Kind, handle HandleFunc, finished ...FinishedFunc) Handler {
	var f FinishedFunc
	if len(finished) > 0 {
		f = finished[0]
	}
	return functionalHandler{kind: kind, handle: handle, finished: f}
}

// Registry manages task handlers by Kind.
type Registry struct {
	mu       sync.RWMutex
	handlers map[Kind]Handler
}

// NewRegistry initializes an empty Registry.
func NewRegistry() *Registry {
	return &Registry{handlers: make(map[Kind]Handler)}
}

// Register registers a handler. If a handler with the same Kind exists, it is overwritten.
func (r *Registry) Register(h Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[h.Kind()] = h
}

// Get retrieves a handler for the specified Kind.
func (r *Registry) Get(k Kind) (Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[k]
	return h, ok
}

// Kinds returns a sorted list of registered task kinds.
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
