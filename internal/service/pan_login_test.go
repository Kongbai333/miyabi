package service

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/pan"
)

// panTestPollWindow shortens a long poll so a test does not wait one out.
func panTestPollWindow(t *testing.T, window time.Duration) func() {
	t.Helper()
	previous := panLoginPollWindow
	panLoginPollWindow = window
	return func() { panLoginPollWindow = previous }
}

func TestPanLoginPollsShareExchangeAfterCallerCancellation(t *testing.T) {
	library, client := panConcurrencyFixture(t)
	drive, tokens := library.drive, panTestTokens("login")
	const waiters = 8
	started := make(chan struct{}, waiters+1)
	hold, release := panTestGate(t)
	var polls, exchanges atomic.Int32
	client.loginStatus = func(context.Context, *pan.Login) (pan.LoginState, error) {
		polls.Add(1)
		return pan.LoginAuthorized, nil
	}
	client.exchangeToken = func(ctx context.Context, _ *pan.Login) (pan.Tokens, error) {
		exchanges.Add(1)
		started <- struct{}{}
		<-hold
		return tokens, ctx.Err()
	}
	login, err := drive.BeginLogin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	departed := make(chan error, 1)
	go func() {
		_, err := drive.LoginStatus(ctx, login.ID)
		departed <- err
	}()
	awaitPan(t, started)
	finished := make(chan error, waiters)
	for range waiters {
		go func() {
			status, err := drive.LoginStatus(t.Context(), login.ID)
			if err == nil && status.State != pan.LoginAuthorized {
				err = fmt.Errorf("login state = %s", status.State)
			}
			finished <- err
		}()
	}
	cancel()
	if err := awaitPan(t, departed); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled login waiter = %v", err)
	}
	release()
	for range waiters {
		if err := awaitPan(t, finished); err != nil {
			t.Fatal(err)
		}
	}
	if polls.Load() != 1 || exchanges.Load() != 1 {
		t.Fatalf("shared login polled %d times and exchanged %d times", polls.Load(), exchanges.Load())
	}
	assertPanTokens(t, drive, tokens)
}

func TestPanLateLoginExchangeCannotReplaceCurrentSession(t *testing.T) {
	for _, action := range []string{"disconnect", "login"} {
		t.Run(action, func(t *testing.T) {
			library, client := panConcurrencyFixture(t)
			drive, newTokens := library.drive, panTestTokens("current-login")
			oldLogin := &pan.Login{QRCode: []byte("old")}
			var logins atomic.Int32
			client.beginLogin = func(context.Context) (*pan.Login, error) {
				if logins.Add(1) == 1 {
					return oldLogin, nil
				}
				return &pan.Login{QRCode: []byte("new")}, nil
			}
			started := make(chan struct{}, 1)
			hold, release := panTestGate(t)
			client.exchangeToken = func(_ context.Context, login *pan.Login) (pan.Tokens, error) {
				if login == oldLogin {
					started <- struct{}{}
					<-hold
					return panTestTokens("stale-login"), nil
				}
				return newTokens, nil
			}
			old, err := drive.BeginLogin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			finished := make(chan error, 1)
			go func() {
				status, err := drive.LoginStatus(t.Context(), old.ID)
				if err == nil && status.State != pan.LoginExpired {
					err = fmt.Errorf("old login state = %s", status.State)
				}
				finished <- err
			}()
			awaitPan(t, started)
			var want pan.Tokens
			if action == "disconnect" {
				if _, err := drive.Disconnect(t.Context()); err != nil {
					t.Fatal(err)
				}
			} else {
				want = newTokens
				current, err := drive.BeginLogin(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if status, err := drive.LoginStatus(t.Context(), current.ID); err != nil || status.State != pan.LoginAuthorized {
					t.Fatalf("current login = %+v, %v", status, err)
				}
			}
			release()
			if err := awaitPan(t, finished); err != nil {
				t.Fatal(err)
			}
			assertPanTokens(t, drive, want)
		})
	}
}

// A long poll that runs out our own window carries no news about the login, and
// the user is usually still lining the QR code up.
func TestPanLoginPollWindowElapsingIsNotAFailure(t *testing.T) {
	library, client := panConcurrencyFixture(t)
	drive := library.drive
	t.Cleanup(panTestPollWindow(t, 20*time.Millisecond))
	var polls atomic.Int32
	client.loginStatus = func(ctx context.Context, _ *pan.Login) (pan.LoginState, error) {
		if polls.Add(1) == 1 {
			<-ctx.Done()
			return "", ctx.Err()
		}
		return pan.LoginScanned, nil
	}
	login, err := drive.BeginLogin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status, err := drive.LoginStatus(t.Context(), login.ID); err != nil || status.State != pan.LoginWaiting {
		t.Fatalf("elapsed poll window = %+v, %v", status, err)
	}
	if status, err := drive.LoginStatus(t.Context(), login.ID); err != nil || status.State != pan.LoginScanned {
		t.Fatalf("poll after the window elapsed = %+v, %v", status, err)
	}
}

// The caller owns the retry budget, so a transport failure must leave the session
// usable rather than writing the failure into it.
func TestPanLoginTransportFailureKeepsTheSession(t *testing.T) {
	library, client := panConcurrencyFixture(t)
	drive := library.drive
	var polls atomic.Int32
	client.loginStatus = func(context.Context, *pan.Login) (pan.LoginState, error) {
		if polls.Add(1) == 1 {
			return "", errors.New("connection reset by peer")
		}
		return pan.LoginScanned, nil
	}
	login, err := drive.BeginLogin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := drive.LoginStatus(t.Context(), login.ID); err == nil {
		t.Fatal("transport failure was reported as a login state")
	}
	if status, err := drive.LoginStatus(t.Context(), login.ID); err != nil || status.State != pan.LoginScanned {
		t.Fatalf("poll after a transport failure = %+v, %v", status, err)
	}
}

// Scanning is progress: a poll that brings no news must not walk the dialog back
// from "confirm on your phone" to "waiting for a scan".
func TestPanLoginKeepsScannedWhenAPollBringsNoNews(t *testing.T) {
	library, client := panConcurrencyFixture(t)
	drive := library.drive
	var polls atomic.Int32
	client.loginStatus = func(context.Context, *pan.Login) (pan.LoginState, error) {
		if polls.Add(1) == 1 {
			return pan.LoginScanned, nil
		}
		return pan.LoginWaiting, nil
	}
	login, err := drive.BeginLogin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status, err := drive.LoginStatus(t.Context(), login.ID); err != nil || status.State != pan.LoginScanned {
		t.Fatalf("scan = %+v, %v", status, err)
	}
	if status, err := drive.LoginStatus(t.Context(), login.ID); err != nil || status.State != pan.LoginScanned {
		t.Fatalf("poll with no news = %+v, %v", status, err)
	}
}

// 115 does not always report a dead QR, so a login has to expire on its own.
func TestPanLoginExpiresAfterItsLifetime(t *testing.T) {
	library, client := panConcurrencyFixture(t)
	drive := library.drive
	client.loginStatus = func(context.Context, *pan.Login) (pan.LoginState, error) {
		return pan.LoginWaiting, nil
	}
	login, err := drive.BeginLogin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	drive.mu.Lock()
	drive.session.startedAt = time.Now().Add(-panLoginLifetime - time.Minute)
	drive.mu.Unlock()
	if status, err := drive.LoginStatus(t.Context(), login.ID); err != nil || status.State != pan.LoginExpired {
		t.Fatalf("stale login = %+v, %v", status, err)
	}
	if status, err := drive.LoginStatus(t.Context(), login.ID); err != nil || status.State != pan.LoginExpired {
		t.Fatalf("stale login polled again = %+v, %v", status, err)
	}
}
