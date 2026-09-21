package service

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAccessGate(t *testing.T) {
	for _, test := range []struct {
		name     string
		password string
		input    string
		valid    bool
	}{
		{name: "disabled", valid: true},
		{name: "disabled with input", input: "anything", valid: true},
		{name: "correct", password: "test-password", input: "test-password", valid: true},
		{name: "wrong", password: "test-password", input: "wrong"},
		{name: "empty", password: "test-password"},
		{name: "prefix", password: "test-password", input: "test"},
		{name: "case sensitive", password: "test-password", input: "TEST-PASSWORD"},
		{name: "unicode", password: "测试密码", input: "测试密码", valid: true},
		{name: "preserve whitespace", password: " test-password ", input: "test-password"},
	} {
		t.Run(test.name, func(t *testing.T) {
			gate := NewAccessGateService(test.password)
			if gate.Enabled() != (test.password != "") {
				t.Fatal("unexpected gate state")
			}
			err := gate.Verify(test.input)
			if test.valid && err != nil {
				t.Fatalf("verify password: %v", err)
			}
			if !test.valid && !errors.Is(err, ErrAccessPassword) {
				t.Fatalf("expected password error, got %v", err)
			}
		})
	}
}

func TestAccessSessionAcceptsIssuedTokenAndRejectsTampering(t *testing.T) {
	gate := NewAccessGateService("test-password")
	token, expires := gate.IssueSession()
	if !expires.After(time.Now()) {
		t.Fatalf("issued session already expired: %v", expires)
	}
	if !gate.ValidSession(token) {
		t.Fatal("issued session was rejected")
	}
	if !strings.Contains(token, ".") {
		t.Fatalf("token is not signed: %q", token)
	}

	deadline, signature, _ := strings.Cut(token, ".")
	for _, scenario := range []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"unsigned expiry", deadline},
		{"missing signature", deadline + "."},
		{"forged signature", deadline + "." + strings.Repeat("0", len(signature))},
		{"truncated signature", deadline + "." + signature[:len(signature)-1]},
		{"extended expiry", "99999999999." + signature},
		{"not a number", "later." + signature},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if gate.ValidSession(scenario.token) {
				t.Fatalf("accepted %q", scenario.token)
			}
		})
	}
}

func TestAccessSessionRejectsExpiredTokensAndOtherPasswords(t *testing.T) {
	gate := NewAccessGateService("test-password")
	token, _ := gate.IssueSession()

	// The expiry is signed, so backdating it invalidates the signature too.
	_, signature, _ := strings.Cut(token, ".")
	past := time.Now().Add(-time.Hour).Unix()
	if gate.ValidSession(strconv.FormatInt(past, 10) + "." + signature) {
		t.Fatal("accepted a backdated expiry")
	}
	// Changing the deployment password must invalidate cookies issued before.
	rotated := NewAccessGateService("new-password")
	if rotated.ValidSession(token) {
		t.Fatal("a rotated password accepted an old session")
	}
	// A disabled gate never validates, so an empty password cannot be bypassed.
	if NewAccessGateService("").ValidSession(token) {
		t.Fatal("a disabled gate validated a token")
	}
}
