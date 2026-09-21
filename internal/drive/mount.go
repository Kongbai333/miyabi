package drive

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/setting"
	"github.com/ppxb/miyabi/internal/pan"
)

// Files retrieves a single page of directory items from 115 for folder navigation.
func (d *Drive) Files(ctx context.Context, directoryID string, page int) (pan.FilePage, error) {
	state := d.snapshot()
	files, err := withPanToken(ctx, d, state, func(token string) (pan.FilePage, error) {
		return d.client.List(ctx, token, directoryID, (page-1)*100, 100)
	})
	if err != nil {
		return pan.FilePage{}, fmt.Errorf("list 115 directory: %w", err)
	}
	if _, err := d.credentials(state); err != nil {
		return pan.FilePage{}, err
	}
	return files, nil
}

// SelectDirectory mounts the given 115 directory as the media root, saves it,
// updates the in-memory state, and emits a MountChanged event.
func (d *Drive) SelectDirectory(ctx context.Context, directoryID string) (domain.LibraryDirectory, error) {
	state := d.snapshot()
	account, err := d.verifyAccount(ctx, state)
	if err != nil {
		return domain.LibraryDirectory{}, fmt.Errorf("get 115 account for directory: %w", err)
	}
	files, err := withPanToken(ctx, d, state, func(token string) (pan.FilePage, error) {
		return d.client.List(ctx, token, directoryID, 0, 1)
	})
	if err != nil {
		return domain.LibraryDirectory{}, fmt.Errorf("get 115 media directory: %w", err)
	}
	names := make([]string, 0, len(files.Path))
	for _, directory := range files.Path {
		if directory.ID != "0" {
			names = append(names, directory.Name)
		}
	}
	directory := domain.LibraryDirectory{
		ID: directoryID, Name: files.Path[len(files.Path)-1].Name,
		Path: "/" + strings.Join(names, "/"),
	}
	record := mountRecord{AccountID: account.ID, LibraryDirectory: directory}

	if err := d.commit.Lock(ctx); err != nil {
		return domain.LibraryDirectory{}, err
	}
	defer d.commit.Unlock()

	current, err := d.credentials(state)
	if err != nil {
		return domain.LibraryDirectory{}, err
	}
	if current.directory.AccountID == record.AccountID && current.directory.ID == record.ID {
		return current.directory.LibraryDirectory, nil
	}
	if _, err := d.sourceState(state.source(), state.authorizationVersion); err != nil {
		return domain.LibraryDirectory{}, err
	}

	if err := saveSetting(ctx, d.database, directorySetting, record); err != nil {
		return domain.LibraryDirectory{}, fmt.Errorf("save media directory setting: %w", err)
	}

	d.mu.Lock()
	d.directory = record
	d.authorizationVersion++
	d.mu.Unlock()

	source := domain.LibrarySource{AccountID: account.ID, Directory: directory}
	if err := d.events.publishMount(ctx, source); err != nil {
		return domain.LibraryDirectory{}, fmt.Errorf("publish mount event: %w", err)
	}
	return directory, nil
}

// ClearDirectory unmounts the active media directory and emits a MountChanged event.
func (d *Drive) ClearDirectory(ctx context.Context) error {
	if err := d.commit.Lock(ctx); err != nil {
		return err
	}
	defer d.commit.Unlock()
	return d.clearDirectory(ctx)
}

func (d *Drive) discardOtherAccountDirectory(ctx context.Context, accountID string) error {
	directory := d.snapshot().directory
	if directory.ID == "" || directory.AccountID == accountID {
		return nil
	}
	return d.clearDirectory(ctx)
}

func (d *Drive) clearDirectory(ctx context.Context) error {
	if _, err := d.database.Setting.Delete().Where(setting.Key(directorySetting)).Exec(ctx); err != nil {
		return fmt.Errorf("remove 115 media directory setting: %w", err)
	}
	d.mu.Lock()
	d.directory = mountRecord{}
	d.authorizationVersion++
	d.mu.Unlock()

	_ = d.events.publishMount(ctx, domain.LibrarySource{})
	return nil
}

// WithinSource checks whether a file info record belongs inside the mounted library source.
func WithinSource(info pan.FileInfo, source domain.LibrarySource) bool {
	return info.ID == source.Directory.ID || source.Directory.ID == "0" ||
		slices.ContainsFunc(info.Path, func(dir pan.Directory) bool { return dir.ID == source.Directory.ID })
}
