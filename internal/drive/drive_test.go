package drive_test

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/pan"
)

type testPanClient struct {
	accountCalls   atomic.Int32
	listCalls      atomic.Int32
	accountFunc    func(context.Context, string) (pan.Account, error)
	listFunc       func(context.Context, string, string, int, int) (pan.FilePage, error)
	infoFunc       func(context.Context, string, string) (pan.FileInfo, error)
	readFunc       func(context.Context, string, string, int64) ([]byte, error)
	uploadFunc     func(context.Context, string, string, string, []byte) error
	refreshFunc    func(context.Context, string) (pan.Tokens, error)
	beginLoginFunc func(context.Context) (*pan.Login, error)
	loginStateFunc func(context.Context, *pan.Login) (pan.LoginState, error)
	exchangeFunc   func(context.Context, *pan.Login) (pan.Tokens, error)
	addOfflineFunc func(context.Context, string, string, string) (string, error)
	delOfflineFunc func(context.Context, string, string) error
	offlineTasksFn func(context.Context, string, int) (pan.OfflinePage, error)
	playURLFunc    func(context.Context, string, string) ([]pan.PlaySource, error)
}

func (c *testPanClient) Close() {}
func (c *testPanClient) Account(ctx context.Context, token string) (pan.Account, error) {
	c.accountCalls.Add(1)
	if c.accountFunc != nil {
		return c.accountFunc(ctx, token)
	}
	return pan.Account{ID: "test-account"}, nil
}
func (c *testPanClient) BeginLogin(ctx context.Context) (*pan.Login, error) {
	if c.beginLoginFunc != nil {
		return c.beginLoginFunc(ctx)
	}
	return &pan.Login{QRCode: []byte("qr")}, nil
}
func (c *testPanClient) LoginStatus(ctx context.Context, login *pan.Login) (pan.LoginState, error) {
	if c.loginStateFunc != nil {
		return c.loginStateFunc(ctx, login)
	}
	return pan.LoginAuthorized, nil
}
func (c *testPanClient) ExchangeToken(ctx context.Context, login *pan.Login) (pan.Tokens, error) {
	if c.exchangeFunc != nil {
		return c.exchangeFunc(ctx, login)
	}
	return pan.Tokens{AccessToken: "acc", RefreshToken: "ref", ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (c *testPanClient) RefreshToken(ctx context.Context, token string) (pan.Tokens, error) {
	if c.refreshFunc != nil {
		return c.refreshFunc(ctx, token)
	}
	return pan.Tokens{AccessToken: "refreshed-acc", RefreshToken: "refreshed-ref", ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (c *testPanClient) List(ctx context.Context, token, dir string, offset, limit int) (pan.FilePage, error) {
	c.listCalls.Add(1)
	if c.listFunc != nil {
		return c.listFunc(ctx, token, dir, offset, limit)
	}
	return pan.FilePage{
		Total: 1,
		Files: []pan.File{{ID: "f1", Name: "video.mp4"}},
		Path:  []pan.Directory{{ID: "0", Name: "Root"}, {ID: dir, Name: "Media"}},
	}, nil
}
func (c *testPanClient) Info(ctx context.Context, token, id string) (pan.FileInfo, error) {
	if c.infoFunc != nil {
		return c.infoFunc(ctx, token, id)
	}
	return pan.FileInfo{ID: id, Name: "video.mp4", PickCode: "p1"}, nil
}
func (c *testPanClient) ReadMetadata(ctx context.Context, token, pickCode string, limit int64) ([]byte, error) {
	if c.readFunc != nil {
		return c.readFunc(ctx, token, pickCode, limit)
	}
	return []byte("<nfo>test</nfo>"), nil
}
func (c *testPanClient) UploadMetadata(ctx context.Context, token, dir, name string, body []byte) error {
	if c.uploadFunc != nil {
		return c.uploadFunc(ctx, token, dir, name, body)
	}
	return nil
}
func (c *testPanClient) AddOffline(ctx context.Context, token, uri, dir string) (string, error) {
	if c.addOfflineFunc != nil {
		return c.addOfflineFunc(ctx, token, uri, dir)
	}
	return "hash123", nil
}
func (c *testPanClient) RemoveOffline(ctx context.Context, token, hash string) error {
	if c.delOfflineFunc != nil {
		return c.delOfflineFunc(ctx, token, hash)
	}
	return nil
}
func (c *testPanClient) OfflineTasks(ctx context.Context, token string, page int) (pan.OfflinePage, error) {
	if c.offlineTasksFn != nil {
		return c.offlineTasksFn(ctx, token, page)
	}
	return pan.OfflinePage{PageCount: 1, Tasks: []pan.OfflineTask{{Hash: "hash123", Status: 2}}}, nil
}
func (c *testPanClient) PlayURL(ctx context.Context, token, pickCode string) ([]pan.PlaySource, error) {
	if c.playURLFunc != nil {
		return c.playURLFunc(ctx, token, pickCode)
	}
	return []pan.PlaySource{{URL: "https://cdn.example.com/video.m3u8", Height: 1080}}, nil
}
func (c *testPanClient) OpenMedia(ctx context.Context, method, address string, headers http.Header) (*http.Response, error) {
	return nil, nil
}

func setupTestDrive(t *testing.T, client *testPanClient) (*drive.Drive, *ent.Client) {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	d, err := drive.NewWithClient(t.Context(), store.Client, client)
	if err != nil {
		t.Fatalf("new drive: %v", err)
	}
	t.Cleanup(d.Close)
	return d, store.Client
}

func TestAccountCacheAndTTL(t *testing.T) {
	client := &testPanClient{}
	d, _ := setupTestDrive(t, client)
	ctx := t.Context()

	// Initial login
	session, err := d.BeginLogin(ctx)
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}
	status, err := d.LoginStatus(ctx, session.ID)
	if err != nil || status.State != pan.LoginAuthorized {
		t.Fatalf("login status: status=%+v, err=%v", status, err)
	}

	// First Account call hits client
	callsBefore := client.accountCalls.Load()
	acc1, err := d.Account(ctx)
	if err != nil || !acc1.Connected {
		t.Fatalf("account 1: %+v, err=%v", acc1, err)
	}
	if client.accountCalls.Load() <= callsBefore {
		t.Fatal("expected client Account call")
	}

	// Second Account call immediately after should use 60s TTL cache without calling client again
	callsAfter := client.accountCalls.Load()
	acc2, err := d.Account(ctx)
	if err != nil || !acc2.Connected {
		t.Fatalf("account 2: %+v, err=%v", acc2, err)
	}
	if client.accountCalls.Load() != callsAfter {
		t.Fatalf("expected cached account without calling upstream again: got %d calls, want %d", client.accountCalls.Load(), callsAfter)
	}
}

func TestMountChangedEventsAndSession(t *testing.T) {
	client := &testPanClient{}
	d, _ := setupTestDrive(t, client)
	ctx := t.Context()

	// Login
	sess, err := d.BeginLogin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.LoginStatus(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}

	// Subscribe to mount events
	var receivedEvent drive.MountEvent
	var eventCalls atomic.Int32
	unsub := d.SubscribeMount(func(ctx context.Context, event drive.MountEvent) error {
		receivedEvent = event
		eventCalls.Add(1)
		return nil
	})
	defer unsub()

	// Select directory
	mounted, err := d.SelectDirectory(ctx, "dir-100")
	if err != nil {
		t.Fatalf("select dir: %v", err)
	}
	if mounted.ID != "dir-100" {
		t.Fatalf("mounted ID: %s", mounted.ID)
	}
	if eventCalls.Load() != 1 || receivedEvent.Source.Directory.ID != "dir-100" {
		t.Fatalf("mount event not received properly: calls=%d, event=%+v", eventCalls.Load(), receivedEvent)
	}

	// Open session for mounted source
	session, err := d.Open(ctx)
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	if session.Source().Directory.ID != "dir-100" {
		t.Fatalf("session source mismatch: %+v", session.Source())
	}

	// Session operations work
	page, err := session.List(ctx, "dir-100", 0)
	if err != nil || len(page.Files) == 0 {
		t.Fatalf("session list: %+v, err=%v", page, err)
	}

	info, err := session.Info(ctx, "f1")
	if err != nil || info.ID != "f1" {
		t.Fatalf("session info: %+v, err=%v", info, err)
	}

	body, err := session.Read(ctx, "p1", 1024)
	if err != nil || len(body) == 0 {
		t.Fatalf("session read: %s, err=%v", string(body), err)
	}

	if err := session.Upload(ctx, "dir-100", "test.nfo", []byte("data")); err != nil {
		t.Fatalf("session upload: %v", err)
	}

	// Session commit works
	committed := false
	err = session.Commit(ctx, func(tx *ent.Tx) error {
		committed = true
		return nil
	})
	if err != nil || !committed {
		t.Fatalf("session commit: committed=%v, err=%v", committed, err)
	}

	// Changing directory invalidates the active session
	_, err = d.SelectDirectory(ctx, "dir-200")
	if err != nil {
		t.Fatalf("select new dir: %v", err)
	}
	if eventCalls.Load() != 2 {
		t.Fatalf("expected second mount event, got %d", eventCalls.Load())
	}

	// Old session calls must now fail with ErrSourceChanged
	if _, err := session.List(ctx, "dir-100", 0); !errors.Is(err, drive.ErrSourceChanged) {
		t.Fatalf("expected ErrSourceChanged for old session List, got: %v", err)
	}
	if _, err := session.Info(ctx, "f1"); !errors.Is(err, drive.ErrSourceChanged) {
		t.Fatalf("expected ErrSourceChanged for old session Info, got: %v", err)
	}
	if err := session.Commit(ctx, func(tx *ent.Tx) error { return nil }); !errors.Is(err, drive.ErrSourceChanged) {
		t.Fatalf("expected ErrSourceChanged for old session Commit, got: %v", err)
	}
}

func TestWalkFilePagesAndOfflinePages(t *testing.T) {
	ctx := context.Background()

	// Test WalkFilePages normal traversal
	pages := []pan.FilePage{
		{Total: 3, Files: []pan.File{{ID: "1"}, {ID: "2"}}, HasMore: true},
		{Total: 3, Files: []pan.File{{ID: "3"}}, HasMore: false},
	}
	callIdx := 0
	var collected []string
	err := drive.WalkFilePages(ctx, func(offset int) (pan.FilePage, error) {
		p := pages[callIdx]
		callIdx++
		return p, nil
	}, func(page pan.FilePage) (bool, error) {
		for _, f := range page.Files {
			collected = append(collected, f.ID)
		}
		return true, nil
	})
	if err != nil || len(collected) != 3 {
		t.Fatalf("walk file pages: len=%d, err=%v", len(collected), err)
	}

	// Test incomplete page returns ErrDirectoryIncomplete
	err = drive.WalkFilePages(ctx, func(offset int) (pan.FilePage, error) {
		return pan.FilePage{Total: 10, Files: []pan.File{{ID: "1"}}, HasMore: false}, nil
	}, func(page pan.FilePage) (bool, error) {
		return true, nil
	})
	if !errors.Is(err, drive.ErrDirectoryIncomplete) {
		t.Fatalf("expected ErrDirectoryIncomplete, got %v", err)
	}

	// Test WalkOfflinePages
	var taskHashes []string
	err = drive.WalkOfflinePages(ctx, func(page int) (pan.OfflinePage, error) {
		if page == 1 {
			return pan.OfflinePage{PageCount: 2, Tasks: []pan.OfflineTask{{Hash: "h1"}}}, nil
		}
		return pan.OfflinePage{PageCount: 2, Tasks: []pan.OfflineTask{{Hash: "h2"}}}, nil
	}, func(page pan.OfflinePage) (bool, error) {
		for _, task := range page.Tasks {
			taskHashes = append(taskHashes, task.Hash)
		}
		return true, nil
	})
	if err != nil || len(taskHashes) != 2 {
		t.Fatalf("walk offline pages: len=%d, err=%v", len(taskHashes), err)
	}
}

func TestWithinSource(t *testing.T) {
	source := domain.LibrarySource{
		AccountID: "acc1",
		Directory: domain.LibraryDirectory{ID: "100", Name: "Media"},
	}

	directMatch := pan.FileInfo{ID: "100"}
	if !drive.WithinSource(directMatch, source) {
		t.Fatal("direct match should be within source")
	}

	childMatch := pan.FileInfo{
		ID:   "200",
		Path: []pan.Directory{{ID: "0"}, {ID: "100"}, {ID: "150"}},
	}
	if !drive.WithinSource(childMatch, source) {
		t.Fatal("child item should be within source")
	}

	outsideItem := pan.FileInfo{
		ID:   "300",
		Path: []pan.Directory{{ID: "0"}, {ID: "999"}},
	}
	if drive.WithinSource(outsideItem, source) {
		t.Fatal("outside item should not be within source")
	}
}
