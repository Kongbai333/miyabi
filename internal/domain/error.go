package domain

import (
	"errors"
	"fmt"
)

// Kind classifies a domain error to determine response status and handling behavior.
type Kind int

const (
	KindUnexpected   Kind = iota
	KindInvalid           // 400 Bad Request
	KindUnauthorized      // 401 Unauthorized
	KindNotFound          // 404 Not Found
	KindConflict          // 409 Conflict
	KindBusy              // 409 Conflict / 503 Busy
	KindUpstream          // 502 Bad Gateway
	KindCanceled          // 499 Client Closed Request
	KindInternal          // 500 Internal Server Error
)

func (k Kind) String() string {
	switch k {
	case KindInvalid:
		return "invalid"
	case KindUnauthorized:
		return "unauthorized"
	case KindNotFound:
		return "not_found"
	case KindConflict:
		return "conflict"
	case KindBusy:
		return "busy"
	case KindUpstream:
		return "upstream"
	case KindCanceled:
		return "canceled"
	case KindInternal:
		return "internal"
	default:
		return "unexpected"
	}
}

// Error represents a structured domain error with a user-facing public message
// and an optional underlying diagnostic cause.
type Error struct {
	Kind    Kind
	Message string
	Cause   error
}

// E creates a new domain Error.
func E(kind Kind, message string, cause error) *Error {
	return &Error{
		Kind:    kind,
		Message: message,
		Cause:   cause,
	}
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil && e.Message != "" {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return e.Kind.String()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// PublicMessage returns the user-facing message safe to be exposed via API.
func (e *Error) PublicMessage() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	switch e.Kind {
	case KindInvalid:
		return "请求参数无效"
	case KindUnauthorized:
		return "未授权访问"
	case KindNotFound:
		return "资源不存在"
	case KindConflict:
		return "资源状态冲突"
	case KindBusy:
		return "服务正忙，请稍后重试"
	case KindUpstream:
		return "上游服务异常"
	case KindCanceled:
		return "请求已取消"
	default:
		return "内部服务错误"
	}
}

// Is reports whether this error matches the target.
func (e *Error) Is(target error) bool {
	if e == nil || target == nil {
		return e == target
	}
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	if t.Kind != KindUnexpected && e.Kind != t.Kind {
		return false
	}
	if t.Message != "" && e.Message != t.Message {
		return false
	}
	return true
}

// IsKind checks if an error or any error in its unwrap chain is a domain.Error with the specified Kind.
func IsKind(err error, kind Kind) bool {
	var de *Error
	if errors.As(err, &de) {
		return de.Kind == kind
	}
	return false
}

// HasKind allows external error types to supply a domain Kind.
type HasKind interface {
	DomainKind() Kind
}

// KindOf returns the domain Kind of the error, or KindUnexpected if not recognized.
func KindOf(err error) Kind {
	if err == nil {
		return KindUnexpected
	}
	var de *Error
	if errors.As(err, &de) {
		return de.Kind
	}
	var hk HasKind
	if errors.As(err, &hk) {
		return hk.DomainKind()
	}
	return KindUnexpected
}
