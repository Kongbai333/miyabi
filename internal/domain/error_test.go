package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestDomainError(t *testing.T) {
	t.Run("basic error formatting and unwrapping", func(t *testing.T) {
		cause := errors.New("underlying network timeout")
		err := E(KindUpstream, "上游服务无响应", cause)

		if err.Kind != KindUpstream {
			t.Errorf("Kind = %v, want %v", err.Kind, KindUpstream)
		}
		if got := err.Error(); got != "上游服务无响应: underlying network timeout" {
			t.Errorf("Error() = %q, want %q", got, "上游服务无响应: underlying network timeout")
		}
		if got := err.PublicMessage(); got != "上游服务无响应" {
			t.Errorf("PublicMessage() = %q, want %q", got, "上游服务无响应")
		}
		if !errors.Is(err, cause) {
			t.Errorf("errors.Is(err, cause) = false, want true")
		}
		if errors.Unwrap(err) != cause {
			t.Errorf("Unwrap() = %v, want %v", errors.Unwrap(err), cause)
		}
	})

	t.Run("default public messages", func(t *testing.T) {
		err := E(KindNotFound, "", nil)
		if got := err.PublicMessage(); got != "资源不存在" {
			t.Errorf("PublicMessage() = %q, want %q", got, "资源不存在")
		}
	})

	t.Run("Is and IsKind", func(t *testing.T) {
		sentinel := E(KindInvalid, "参数无效", nil)
		wrapped := fmt.Errorf("wrap: %w", sentinel)

		if !IsKind(wrapped, KindInvalid) {
			t.Errorf("IsKind(wrapped, KindInvalid) = false, want true")
		}
		if IsKind(wrapped, KindNotFound) {
			t.Errorf("IsKind(wrapped, KindNotFound) = true, want false")
		}
		if !errors.Is(wrapped, sentinel) {
			t.Errorf("errors.Is(wrapped, sentinel) = false, want true")
		}
		if KindOf(wrapped) != KindInvalid {
			t.Errorf("KindOf(wrapped) = %v, want %v", KindOf(wrapped), KindInvalid)
		}
	})

	t.Run("sentinels match by identity only", func(t *testing.T) {
		first := E(KindInvalid, "参数无效", nil)
		second := E(KindInvalid, "参数无效", nil)
		if errors.Is(first, second) {
			t.Errorf("errors.Is matched two distinct sentinels with equal fields")
		}
		if !errors.Is(fmt.Errorf("wrap: %w", first), first) {
			t.Errorf("errors.Is lost the sentinel through wrapping")
		}
	})
}
