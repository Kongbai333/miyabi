package service

import (
	"context"
	"fmt"
	"io/fs"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/predicate"
	"github.com/ppxb/miyabi/internal/ent/watchhistory"
)

var ErrWatchHistorySourceChanged = domain.E(domain.KindConflict, "媒体目录已切换，请刷新观看历史后重试", nil)
var ErrInvalidWatchProgress = domain.E(domain.KindInvalid, "观看进度无效", nil)

type WatchHistoryScope struct {
	AccountID   string `json:"account_id" form:"account_id" binding:"required,max=128"`
	DirectoryID string `json:"directory_id" form:"directory_id" binding:"required,max=128"`
}

type WatchSession struct {
	ID        int     `json:"id"`
	SessionID string  `json:"session_id"`
	FileID    string  `json:"file_id"`
	Position  float64 `json:"position"`
	Duration  float64 `json:"duration"`
}

// WatchResume is read without creating a history entry or rotating its session.
type WatchResume struct {
	ID       int     `json:"id"`
	FileID   string  `json:"file_id"`
	Position float64 `json:"position"`
	Duration float64 `json:"duration"`
}

// watchProgress returns the saved playback position of each movie in the
// mounted source, keyed by movie id. Rows that were never played carry no
// progress, so the card can leave the bar off entirely.
func (service *LibraryService) watchProgress(ctx context.Context, source LibrarySource, movieIDs []int) (map[int]*LibraryProgress, error) {
	result := make(map[int]*LibraryProgress, len(movieIDs))
	if len(movieIDs) == 0 {
		return result, nil
	}
	rows, err := service.database.WatchHistory.Query().
		Where(watchhistory.AccountIDEQ(source.AccountID), watchhistory.RootIDEQ(source.Directory.ID),
			watchhistory.MovieIDIn(movieIDs...), watchhistory.DurationGT(0)).
		Select(watchhistory.FieldMovieID, watchhistory.FieldPosition, watchhistory.FieldDuration).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query watch progress: %w", err)
	}
	for _, row := range rows {
		result[row.MovieID] = &LibraryProgress{Position: row.Position, Duration: row.Duration}
	}
	return result, nil
}

type WatchProgress struct {
	SessionID string  `json:"session_id" binding:"required,uuid"`
	FileID    string  `json:"file_id" binding:"required,max=128"`
	Position  float64 `json:"position" binding:"gte=0"`
	Duration  float64 `json:"duration" binding:"gt=0"`
	Version   int     `json:"version" binding:"min=1"`
}

func historyScope(source LibrarySource) predicate.WatchHistory {
	return watchhistory.And(watchhistory.AccountIDEQ(source.AccountID), watchhistory.RootIDEQ(source.Directory.ID))
}

// Opening a movie creates one source-scoped history entry and starts a fresh
// progress session. Reopening preserves its saved position and watched badge.
func (service *LibraryService) MarkWatched(ctx context.Context, movieID int, scope WatchHistoryScope) (WatchSession, error) {
	var result WatchSession
	libraryChanged := false
	err := ent.WithTx(ctx, service.database, func(tx *ent.Tx) error {
		source, err := loadLibrarySource(ctx, tx.Client())
		if err != nil {
			return err
		}
		if source == nil {
			return ErrMediaDirectoryRequired
		}
		if source.AccountID != scope.AccountID || source.Directory.ID != scope.DirectoryID {
			return ErrWatchHistorySourceChanged
		}
		record, err := tx.Movie.Query().Where(movie.IDEQ(movieID), movie.HasFilesWith(libraryFiles(*source))).
			Select(movie.FieldID, movie.FieldWatched).Only(ctx)
		if err != nil {
			return err
		}
		if !record.Watched {
			if err := tx.Movie.UpdateOneID(movieID).SetWatched(true).Exec(ctx); err != nil {
				return err
			}
			libraryChanged = true
		}
		if err := tx.WatchHistory.Create().SetAccountID(source.AccountID).SetRootID(source.Directory.ID).
			SetMovieID(movieID).SetWatchedAt(time.Now().UTC()).SetSessionID(uuid.NewString()).
			OnConflictColumns(watchhistory.FieldAccountID, watchhistory.FieldRootID, watchhistory.FieldMovieID).
			Update(func(update *ent.WatchHistoryUpsert) {
				update.UpdateWatchedAt().UpdateSessionID().SetProgressVersion(0)
			}).Exec(ctx); err != nil {
			return err
		}
		history, err := tx.WatchHistory.Query().Where(historyScope(*source), watchhistory.MovieIDEQ(movieID)).Only(ctx)
		if err != nil {
			return err
		}
		result = WatchSession{ID: history.ID, SessionID: history.SessionID, FileID: history.FileID,
			Position: history.Position, Duration: history.Duration}
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("record library watch: %w", err)
	}
	if libraryChanged {
		service.tasks.NotifyLibraryChanged()
	} else {
		service.tasks.NotifyWatchHistoryChanged()
	}
	return result, nil
}

func (service *LibraryService) SaveWatchProgress(ctx context.Context, id int, progress WatchProgress) error {
	if progress.SessionID == "" || progress.FileID == "" || progress.Version < 1 || progress.Position < 0 ||
		progress.Duration <= 0 || math.IsNaN(progress.Position) || math.IsInf(progress.Position, 0) ||
		math.IsNaN(progress.Duration) || math.IsInf(progress.Duration, 0) {
		return ErrInvalidWatchProgress
	}
	changed := false
	err := ent.WithTx(ctx, service.database, func(tx *ent.Tx) error {
		source, err := loadLibrarySource(ctx, tx.Client())
		if err != nil {
			return err
		}
		if source == nil {
			return ErrMediaDirectoryRequired
		}
		record, err := tx.WatchHistory.Query().Where(historyScope(*source), watchhistory.IDEQ(id)).Only(ctx)
		if err != nil {
			return err
		}
		// A late request from another session or an older seek must not restore
		// stale progress. Updating existing rows also cannot recreate cleared history.
		if record.SessionID != progress.SessionID || record.ProgressVersion >= progress.Version {
			return nil
		}
		found, err := tx.File.Query().Where(libraryFiles(*source), file.MovieIDEQ(record.MovieID), file.FileIDEQ(progress.FileID)).Exist(ctx)
		if err != nil {
			return err
		}
		if !found {
			return fs.ErrNotExist
		}
		position := math.Min(progress.Position, progress.Duration)
		update := tx.WatchHistory.UpdateOneID(id).SetFileID(progress.FileID).SetPosition(position).
			SetDuration(progress.Duration).SetProgressVersion(progress.Version)
		changed = record.FileID != progress.FileID || record.Position != position || record.Duration != progress.Duration
		if changed {
			update.SetWatchedAt(time.Now().UTC())
		}
		return update.Exec(ctx)
	})
	if err != nil {
		return fmt.Errorf("save watch progress: %w", err)
	}
	if changed {
		service.tasks.NotifyWatchHistoryChanged()
	}
	return nil
}

// ClearWatchHistory drops every progress entry of the mounted source. Movies
// and their watched badges are kept, so the library grid only loses its bars.
func (service *LibraryService) ClearWatchHistory(ctx context.Context, scope WatchHistoryScope) (int, error) {
	count := 0
	err := ent.WithTx(ctx, service.database, func(tx *ent.Tx) error {
		source, err := loadLibrarySource(ctx, tx.Client())
		if err != nil {
			return err
		}
		if source == nil || source.AccountID != scope.AccountID || source.Directory.ID != scope.DirectoryID {
			return ErrWatchHistorySourceChanged
		}
		count, err = tx.WatchHistory.Delete().Where(historyScope(*source)).Exec(ctx)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("clear watch history: %w", err)
	}
	if count > 0 {
		service.tasks.NotifyWatchHistoryChanged()
	}
	return count, nil
}
