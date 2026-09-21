package service

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/pan"
)

type panStub struct {
	drive.Client
	account        func(context.Context, string) (pan.Account, error)
	beginLogin     func(context.Context) (*pan.Login, error)
	loginStatus    func(context.Context, *pan.Login) (pan.LoginState, error)
	exchangeToken  func(context.Context, *pan.Login) (pan.Tokens, error)
	refreshToken   func(context.Context, string) (pan.Tokens, error)
	list           func(context.Context, string, string, int, int) (pan.FilePage, error)
	info           func(context.Context, string, string) (pan.FileInfo, error)
	readMetadata   func(context.Context, string, string, int64) ([]byte, error)
	uploadMetadata func(context.Context, string, string, string, []byte) error
	addOffline     func(context.Context, string, string, string) (string, error)
	removeOffline  func(context.Context, string, string) error
	offlineTasks   func(context.Context, string, int) (pan.OfflinePage, error)
	playURL        func(context.Context, string, string) ([]pan.PlaySource, error)
	openMedia      func(context.Context, string, string, http.Header) (*http.Response, error)
}

func (client *panStub) Close() {
	if client.Client != nil {
		client.Client.Close()
	}
}

func (client *panStub) Account(ctx context.Context, token string) (pan.Account, error) {
	if client.account != nil {
		return client.account(ctx, token)
	}
	if client.Client != nil {
		return client.Client.Account(ctx, token)
	}
	return pan.Account{}, nil
}

func (client *panStub) BeginLogin(ctx context.Context) (*pan.Login, error) {
	if client.beginLogin != nil {
		return client.beginLogin(ctx)
	}
	if client.Client != nil {
		return client.Client.BeginLogin(ctx)
	}
	return &pan.Login{}, nil
}

func (client *panStub) LoginStatus(ctx context.Context, login *pan.Login) (pan.LoginState, error) {
	if client.loginStatus != nil {
		return client.loginStatus(ctx, login)
	}
	if client.Client != nil {
		return client.Client.LoginStatus(ctx, login)
	}
	return pan.LoginAuthorized, nil
}

func (client *panStub) ExchangeToken(ctx context.Context, login *pan.Login) (pan.Tokens, error) {
	if client.exchangeToken != nil {
		return client.exchangeToken(ctx, login)
	}
	if client.Client != nil {
		return client.Client.ExchangeToken(ctx, login)
	}
	return pan.Tokens{}, nil
}

func (client *panStub) RefreshToken(ctx context.Context, token string) (pan.Tokens, error) {
	if client.refreshToken != nil {
		return client.refreshToken(ctx, token)
	}
	if client.Client != nil {
		return client.Client.RefreshToken(ctx, token)
	}
	return pan.Tokens{}, nil
}

func (client *panStub) List(ctx context.Context, token, directory string, offset, limit int) (pan.FilePage, error) {
	if client.list != nil {
		return client.list(ctx, token, directory, offset, limit)
	}
	if client.Client != nil {
		return client.Client.List(ctx, token, directory, offset, limit)
	}
	return pan.FilePage{}, nil
}

func (client *panStub) Info(ctx context.Context, token, id string) (pan.FileInfo, error) {
	if client.info != nil {
		return client.info(ctx, token, id)
	}
	if client.Client != nil {
		return client.Client.Info(ctx, token, id)
	}
	return pan.FileInfo{}, nil
}

func (client *panStub) ReadMetadata(ctx context.Context, token, pickCode string, limit int64) ([]byte, error) {
	if client.readMetadata != nil {
		return client.readMetadata(ctx, token, pickCode, limit)
	}
	if client.Client != nil {
		return client.Client.ReadMetadata(ctx, token, pickCode, limit)
	}
	return nil, nil
}

func (client *panStub) UploadMetadata(ctx context.Context, token, directory, name string, body []byte) error {
	if client.uploadMetadata != nil {
		return client.uploadMetadata(ctx, token, directory, name, body)
	}
	if client.Client != nil {
		return client.Client.UploadMetadata(ctx, token, directory, name, body)
	}
	return nil
}

func (client *panStub) AddOffline(ctx context.Context, token, uri, directory string) (string, error) {
	if client.addOffline != nil {
		return client.addOffline(ctx, token, uri, directory)
	}
	if client.Client != nil {
		return client.Client.AddOffline(ctx, token, uri, directory)
	}
	return "", nil
}

func (client *panStub) RemoveOffline(ctx context.Context, token, hash string) error {
	if client.removeOffline != nil {
		return client.removeOffline(ctx, token, hash)
	}
	if client.Client != nil {
		return client.Client.RemoveOffline(ctx, token, hash)
	}
	return nil
}

func (client *panStub) OfflineTasks(ctx context.Context, token string, page int) (pan.OfflinePage, error) {
	if client.offlineTasks != nil {
		return client.offlineTasks(ctx, token, page)
	}
	if client.Client != nil {
		return client.Client.OfflineTasks(ctx, token, page)
	}
	return pan.OfflinePage{}, nil
}

func (client *panStub) PlayURL(ctx context.Context, token, pickCode string) ([]pan.PlaySource, error) {
	if client.playURL != nil {
		return client.playURL(ctx, token, pickCode)
	}
	if client.Client != nil {
		return client.Client.PlayURL(ctx, token, pickCode)
	}
	return nil, nil
}

func (client *panStub) OpenMedia(ctx context.Context, method, address string, headers http.Header) (*http.Response, error) {
	if client.openMedia != nil {
		return client.openMedia(ctx, method, address, headers)
	}
	if client.Client != nil {
		return client.Client.OpenMedia(ctx, method, address, headers)
	}
	return pan.New().OpenMedia(ctx, method, address, headers)
}

func panTestTokens(prefix string) pan.Tokens {
	return pan.Tokens{AccessToken: prefix + "-access", RefreshToken: prefix + "-refresh", ExpiresAt: time.Now().Add(time.Hour)}
}

func panConcurrencyFixture(t *testing.T) (*LibraryService, *panStub) {
	t.Helper()
	library, _, payload := libraryFixture(t)
	client := &panStub{
		account: func(context.Context, string) (pan.Account, error) {
			return pan.Account{ID: payload.Source.AccountID}, nil
		},
		beginLogin:  func(context.Context) (*pan.Login, error) { return &pan.Login{QRCode: []byte("fixture")}, nil },
		loginStatus: func(context.Context, *pan.Login) (pan.LoginState, error) { return pan.LoginAuthorized, nil },
	}
	d, err := drive.NewWithClient(t.Context(), library.database, client)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.MountSource(t.Context(), payload.Source, panTestTokens("original")); err != nil {
		t.Fatal(err)
	}
	library.drive = d
	t.Cleanup(library.drive.Close)
	return library, client
}

func panTestGate(t *testing.T) (<-chan struct{}, func()) {
	t.Helper()
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	t.Cleanup(release)
	return gate, release
}

func awaitPan[T any](t *testing.T, ready <-chan T) T {
	t.Helper()
	select {
	case value := <-ready:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for concurrent 115 operation")
		var zero T
		return zero
	}
}

func awaitPanCondition(t *testing.T, ready func() bool) {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for !ready() {
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("concurrent 115 operation did not reach the expected state")
		}
	}
}
