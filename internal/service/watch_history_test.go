package service

import (
	"errors"
	"io/fs"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/watchhistory"
)

func historyFilm(t *testing.T, library *LibraryService, source LibrarySource, code string) (*ent.Movie, *ent.File) {
	t.Helper()
	film := library.database.Movie.Create().SetCode(code).SetTitle("Title " + code).SetWatched(true).SaveX(t.Context())
	video := library.database.File.Create().SetFileID(code).SetName(code + ".mp4").SetSize(1 << 30).
		SetAccountID(source.AccountID).SetRootID(source.Directory.ID).SetMovie(film).SaveX(t.Context())
	return film, video
}

// libraryMovie reads one card out of the grid, which is where playback progress
// is now surfaced.
func libraryMovie(t *testing.T, library *LibraryService, movieID int) LibraryMovie {
	t.Helper()
	page, err := library.Movies(t.Context(), 1, 50, LibraryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, film := range page.Movies {
		if film.ID == movieID {
			return film
		}
	}
	t.Fatalf("movie %d is missing from the grid", movieID)
	return LibraryMovie{}
}

func TestWatchProgressPreservesResumeAndRejectsStaleSessionsAndVersions(t *testing.T) {
	library, _, payload := libraryFixture(t)
	ctx := t.Context()
	film, video := historyFilm(t, library, payload.Source, "ABP-001")
	session, err := library.MarkWatched(ctx, film.ID, testWatchScope(payload.Source))
	if err != nil {
		t.Fatal(err)
	}
	before := library.tasks.Revisions()
	progress := WatchProgress{SessionID: session.SessionID, FileID: video.FileID, Position: 120, Duration: 600, Version: 2}
	if err := library.SaveWatchProgress(ctx, session.ID, progress); err != nil {
		t.Fatal(err)
	}
	after := library.tasks.Revisions()
	if after.History != before.History+1 || after.Library != before.Library || after.Offline != before.Offline {
		t.Fatal("saving progress refreshed unrelated library or offline data")
	}
	progress.Version, progress.Position = 1, 300
	if err := library.SaveWatchProgress(ctx, session.ID, progress); err != nil {
		t.Fatal(err)
	}
	if row := library.database.WatchHistory.GetX(ctx, session.ID); row.Position != 120 || row.ProgressVersion != 2 || library.tasks.Revisions() != after {
		t.Fatalf("an older request overwrote progress: %+v", row)
	}
	// Seeking backward is valid when it belongs to a newer request.
	progress.Version, progress.Position = 3, 30
	if err := library.SaveWatchProgress(ctx, session.ID, progress); err != nil {
		t.Fatal(err)
	}
	reopened, err := library.MarkWatched(ctx, film.ID, testWatchScope(payload.Source))
	if err != nil || reopened.ID != session.ID || reopened.SessionID == session.SessionID || reopened.Position != 30 || reopened.Duration != 600 || reopened.FileID != video.FileID {
		t.Fatalf("reopening lost resume information: %+v, %v", reopened, err)
	}
	progress.Version, progress.Position = 100, 550
	if err := library.SaveWatchProgress(ctx, session.ID, progress); err != nil {
		t.Fatal(err)
	}
	if row := library.database.WatchHistory.GetX(ctx, session.ID); row.Position != 30 || row.ProgressVersion != 0 {
		t.Fatal("a previous playback session replaced the new session")
	}
	progress.SessionID, progress.Version, progress.Position = reopened.SessionID, 1, 601
	if err := library.SaveWatchProgress(ctx, session.ID, progress); err != nil {
		t.Fatal(err)
	}
	if row := library.database.WatchHistory.GetX(ctx, session.ID); row.Position != 600 {
		t.Fatal("completed playback escaped its duration")
	}
}

func TestWatchProgressValidatesFilesNumbersAndMountedSource(t *testing.T) {
	library, _, payload := libraryFixture(t)
	ctx := t.Context()
	film, video := historyFilm(t, library, payload.Source, "ABP-001")
	_, otherVideo := historyFilm(t, library, payload.Source, "ABP-002")
	session, err := library.MarkWatched(ctx, film.ID, testWatchScope(payload.Source))
	if err != nil {
		t.Fatal(err)
	}
	progress := WatchProgress{SessionID: session.SessionID, FileID: video.FileID, Position: 1, Duration: 600, Version: 1}
	for _, mutate := range []func(*WatchProgress){
		func(p *WatchProgress) { p.Position = -1 }, func(p *WatchProgress) { p.Position = math.NaN() },
		func(p *WatchProgress) { p.Duration = math.Inf(1) }, func(p *WatchProgress) { p.Duration = 0 },
		func(p *WatchProgress) { p.Version = 0 },
	} {
		invalid := progress
		mutate(&invalid)
		if err := library.SaveWatchProgress(ctx, session.ID, invalid); !errors.Is(err, ErrInvalidWatchProgress) {
			t.Fatalf("invalid progress was accepted: %+v, %v", invalid, err)
		}
	}
	progress.FileID = otherVideo.FileID
	if err := library.SaveWatchProgress(ctx, session.ID, progress); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a file from another movie was accepted: %v", err)
	}
	progress.FileID = video.FileID
	if err := saveSetting(ctx, library.database, panDirectorySetting, panLibraryDirectory{
		AccountID: "other", PanLibraryDirectory: payload.Source.Directory,
	}); err != nil {
		t.Fatal(err)
	}
	if err := library.SaveWatchProgress(ctx, session.ID, progress); !ent.IsNotFound(err) {
		t.Fatalf("an inactive source accepted progress: %v", err)
	}
	if row := library.database.WatchHistory.GetX(ctx, session.ID); row.Position != 0 || row.ProgressVersion != 0 {
		t.Fatal("rejected progress mutated the stored record")
	}
}

func TestClearingHistoryPreservesMoviesAndCannotBeUndoneByLateProgress(t *testing.T) {
	library, _, payload := libraryFixture(t)
	ctx := t.Context()
	first, video := historyFilm(t, library, payload.Source, "ABP-001")
	second, _ := historyFilm(t, library, payload.Source, "ABP-002")
	firstSession, err := library.MarkWatched(ctx, first.ID, testWatchScope(payload.Source))
	if err != nil {
		t.Fatal(err)
	}
	secondSession, err := library.MarkWatched(ctx, second.ID, testWatchScope(payload.Source))
	if err != nil {
		t.Fatal(err)
	}
	other := library.database.WatchHistory.Create().SetAccountID("other").SetRootID("10").SetMovie(first).
		SetSessionID(uuid.NewString()).SaveX(ctx)
	scope := WatchHistoryScope{AccountID: payload.Source.AccountID, DirectoryID: payload.Source.Directory.ID}
	if _, err := library.ClearWatchHistory(ctx, WatchHistoryScope{AccountID: "other", DirectoryID: "10"}); !errors.Is(err, ErrWatchHistorySourceChanged) {
		t.Fatalf("a stale clear operation was accepted: %v", err)
	}
	if count, err := library.ClearWatchHistory(ctx, scope); err != nil || count != 2 {
		t.Fatalf("clear did not remove the source's records: %d, %v", count, err)
	}
	progress := WatchProgress{SessionID: firstSession.SessionID, FileID: video.FileID, Position: 100, Duration: 600, Version: 1}
	if err := library.SaveWatchProgress(ctx, firstSession.ID, progress); !ent.IsNotFound(err) {
		t.Fatalf("late progress recreated a cleared record: %v", err)
	}
	if library.database.WatchHistory.Query().CountX(ctx) != 1 || !library.database.WatchHistory.Query().Where(watchhistory.IDEQ(other.ID)).ExistX(ctx) {
		t.Fatal("clear removed another account's history")
	}
	if !library.database.Movie.GetX(ctx, first.ID).Watched || library.database.File.Query().CountX(ctx) != 2 {
		t.Fatal("clearing history removed files or reset watched badges")
	}
	if library.database.WatchHistory.Query().Where(watchhistory.IDEQ(secondSession.ID)).ExistX(ctx) {
		t.Fatal("clear left an active history record")
	}
	// Removing an orphan movie during scanning must cascade to history.
	library.database.File.Delete().Where(file.MovieIDEQ(first.ID)).ExecX(ctx)
	library.database.Movie.DeleteOneID(first.ID).ExecX(ctx)
	if library.database.WatchHistory.Query().CountX(ctx) != 0 {
		t.Fatal("removed movie left orphan history")
	}
}

// The grid is where progress is shown now, so clearing history has to take the
// bar away while leaving the movie and its watched badge alone.
func TestClearingHistoryRemovesLibraryProgress(t *testing.T) {
	library, _, payload := libraryFixture(t)
	ctx := t.Context()
	film, video := historyFilm(t, library, payload.Source, "ABP-001")
	session, err := library.MarkWatched(ctx, film.ID, testWatchScope(payload.Source))
	if err != nil {
		t.Fatal(err)
	}
	if err := library.SaveWatchProgress(ctx, session.ID, WatchProgress{
		SessionID: session.SessionID, FileID: video.FileID, Position: 120, Duration: 600, Version: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if progress := libraryMovie(t, library, film.ID).Progress; progress == nil || progress.Position != 120 {
		t.Fatalf("progress is missing before the clear: %+v", progress)
	}

	if _, err := library.ClearWatchHistory(ctx, testWatchScope(payload.Source)); err != nil {
		t.Fatal(err)
	}
	card := libraryMovie(t, library, film.ID)
	if card.Progress != nil {
		t.Fatalf("cleared history left a progress bar: %+v", card.Progress)
	}
	if !card.Watched {
		t.Fatal("clearing history reset the watched badge")
	}
}
