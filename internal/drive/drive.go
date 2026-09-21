package drive

import (
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/setting"
	"github.com/ppxb/miyabi/internal/pan"
	"golang.org/x/sync/singleflight"
)

const (
	credentialsSetting = "pan.credentials"
	directorySetting   = "pan.library_directory"
)

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

// Client abstracts the upstream 115 API calls.
type Client interface {
	Close()
	Account(context.Context, string) (pan.Account, error)
	BeginLogin(context.Context) (*pan.Login, error)
	LoginStatus(context.Context, *pan.Login) (pan.LoginState, error)
	ExchangeToken(context.Context, *pan.Login) (pan.Tokens, error)
	RefreshToken(context.Context, string) (pan.Tokens, error)
	List(context.Context, string, string, int, int) (pan.FilePage, error)
	Info(context.Context, string, string) (pan.FileInfo, error)
	ReadMetadata(context.Context, string, string, int64) ([]byte, error)
	UploadMetadata(context.Context, string, string, string, []byte) error
	AddOffline(context.Context, string, string, string) (string, error)
	RemoveOffline(context.Context, string, string) error
	OfflineTasks(context.Context, string, int) (pan.OfflinePage, error)
	PlayURL(context.Context, string, string) ([]pan.PlaySource, error)
	OpenMedia(context.Context, string, string, http.Header) (*http.Response, error)
}

type mountRecord struct {
	AccountID string `json:"account_id"`
	domain.LibraryDirectory
}

func (r mountRecord) source() domain.LibrarySource {
	return domain.LibrarySource{AccountID: r.AccountID, Directory: r.LibraryDirectory}
}

type snapshot struct {
	tokens               pan.Tokens
	directory            mountRecord
	authorizationVersion uint64
	credentialVersion    uint64
	tokenVersion         uint64
	closed               bool
}

func (s snapshot) source() domain.LibrarySource {
	return s.directory.source()
}

func (s snapshot) matchesSource(source domain.LibrarySource, version uint64) bool {
	return s.authorizationVersion == version && s.directory.AccountID == source.AccountID && s.directory.ID == source.Directory.ID
}

// Drive coordinates 115 credentials, mount point, and authorization lifecycle.
type Drive struct {
	database *ent.Client
	client   Client

	mu                   sync.Mutex
	commit               contextLock
	tokens               pan.Tokens
	session              *loginSession
	directory            mountRecord
	authorizationVersion uint64
	credentialVersion    uint64
	tokenVersion         uint64
	refresh              singleflight.Group
	work                 sync.WaitGroup
	closed               bool

	accountCacheMu    sync.Mutex
	cachedAccount     pan.Account
	cachedAccountTime time.Time

	events *eventBus
}

func New(ctx context.Context, database *ent.Client) (*Drive, error) {
	return NewWithClient(ctx, database, pan.New())
}

func NewWithClient(ctx context.Context, database *ent.Client, client Client) (*Drive, error) {
	tokens, _, err := loadSetting[pan.Tokens](ctx, database, credentialsSetting)
	if err != nil {
		return nil, err
	}
	directory, _, err := loadSetting[mountRecord](ctx, database, directorySetting)
	if err != nil {
		return nil, err
	}
	return &Drive{
		database: database,
		client:   client,
		tokens:   tokens,
		directory: directory,
		events:   newEventBus(),
	}, nil
}

func (d *Drive) Close() {
	d.mu.Lock()
	d.closed = true
	d.mu.Unlock()
	d.work.Wait()
	d.client.Close()
}

// SetClient replaces the upstream client (primarily used for test doubles).
func (d *Drive) SetClient(client Client) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.client = client
}

func (d *Drive) startWork() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return false
	}
	d.work.Add(1)
	return true
}

func (d *Drive) snapshot() snapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	return snapshot{
		tokens:               d.tokens,
		directory:            d.directory,
		authorizationVersion: d.authorizationVersion,
		credentialVersion:    d.credentialVersion,
		tokenVersion:         d.tokenVersion,
		closed:               d.closed,
	}
}

func (d *Drive) credentials(expected snapshot) (snapshot, error) {
	current := d.snapshot()
	if current.credentialVersion != expected.credentialVersion || current.tokens.AccessToken == "" {
		return snapshot{}, pan.ErrUnauthorized
	}
	return current, nil
}

func (d *Drive) sourceState(source domain.LibrarySource, version uint64) (snapshot, error) {
	state := d.snapshot()
	if !state.matchesSource(source, version) {
		return snapshot{}, ErrSourceChanged
	}
	return state, nil
}

func (d *Drive) verifiedSource(ctx context.Context) (snapshot, error) {
	state := d.snapshot()
	account, err := d.verifyAccount(ctx, state)
	if err != nil {
		return snapshot{}, err
	}
	if _, err := d.sourceState(state.source(), state.authorizationVersion); err != nil {
		return snapshot{}, err
	}
	if state.directory.ID == "" || state.directory.AccountID != account.ID {
		return snapshot{}, ErrMediaDirectoryRequired
	}
	return state, nil
}

// StartWork increments the in-flight work counter if the drive is not closed.
// It returns a done callback and true on success, or nil and false if closed.
func (d *Drive) StartWork() (func(), bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, false
	}
	d.work.Add(1)
	return d.work.Done, true
}

// ValidateSource checks whether the drive has not been closed, is still authenticated,
// and matches the given source and authorization version.
func (d *Drive) ValidateSource(source domain.LibrarySource, version uint64) bool {
	s := d.snapshot()
	return !s.closed && s.matchesSource(source, version) && s.tokens.AccessToken != ""
}

// BumpAuthorization increments the authorization version, invalidating existing sessions.
func (d *Drive) BumpAuthorization() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.authorizationVersion++
}

// AuthorizationVersion returns the current authorization version.
func (d *Drive) AuthorizationVersion() uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.authorizationVersion
}

// Commit executes a database transaction under the drive commit lock.
func (d *Drive) Commit(ctx context.Context, fn func(tx *ent.Tx) error) error {
	if err := d.commit.Lock(ctx); err != nil {
		return err
	}
	defer d.commit.Unlock()
	return ent.WithTx(ctx, d.database, fn)
}

// OpenMedia streams media from 115 CDN directly.
func (d *Drive) OpenMedia(ctx context.Context, method, address string, headers http.Header) (*http.Response, error) {
	return d.client.OpenMedia(ctx, method, address, headers)
}

// MountSource directly configures tokens and the mounted directory, persisting both to settings.
func (d *Drive) MountSource(ctx context.Context, source domain.LibrarySource, tokens pan.Tokens) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if tokens.ExpiresAt.IsZero() {
		tokens.ExpiresAt = time.Now().Add(24 * time.Hour)
	}
	d.tokens = tokens
	d.directory = mountRecord{AccountID: source.AccountID, LibraryDirectory: source.Directory}
	d.authorizationVersion++
	if err := saveSetting(ctx, d.database, credentialsSetting, tokens); err != nil {
		return err
	}
	if err := saveSetting(ctx, d.database, directorySetting, d.directory); err != nil {
		return err
	}
	return nil
}

// Source returns the current mounted library source from the in-memory snapshot, or nil if unmounted.
func (d *Drive) Source() *domain.LibrarySource {
	s := d.snapshot()
	if s.directory.ID == "" || s.directory.AccountID == "" {
		return nil
	}
	src := s.source()
	return &src
}

// SubscribeMount registers a listener for directory mount and unmount events.
func (d *Drive) SubscribeMount(listener MountListener) func() {
	return d.events.subscribe(listener)
}

func loadSetting[T any](ctx context.Context, database *ent.Client, key string) (T, bool, error) {
	var value T
	record, err := database.Setting.Query().Where(setting.Key(key)).Only(ctx)
	if ent.IsNotFound(err) {
		return value, false, nil
	}
	if err != nil {
		return value, false, fmt.Errorf("load setting %s: %w", key, err)
	}
	if err := json.Unmarshal(record.Value, &value); err != nil {
		return value, false, fmt.Errorf("decode setting %s: %w", key, err)
	}
	return value, true, nil
}

func saveSetting(ctx context.Context, database *ent.Client, key string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode setting %s: %w", key, err)
	}
	if err := database.Setting.Create().SetKey(key).SetValue(jsontext.Value(encoded)).
		OnConflictColumns(setting.FieldKey).UpdateNewValues().Exec(ctx); err != nil {
		return fmt.Errorf("save setting %s: %w", key, err)
	}
	return nil
}
