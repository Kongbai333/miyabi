package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/viewedmovie"
)

// ErrInvalidViewedMovie rejects a blank catalogue id before it reaches the table.
var ErrInvalidViewedMovie = domain.E(domain.KindInvalid, "影片 ID 无效", nil)

// MarkViewed records that a movie detail page was opened. Revisiting an
// already recorded movie keeps the original first-viewed timestamp.
func (service *DiscoverService) MarkViewed(ctx context.Context, movieID string) error {
	movieID = strings.TrimSpace(movieID)
	if movieID == "" {
		return ErrInvalidViewedMovie
	}
	exists, err := service.database.ViewedMovie.Query().
		Where(viewedmovie.MovieIDEQ(movieID)).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("mark viewed movie: %w", err)
	}
	if exists {
		return nil
	}
	err = service.database.ViewedMovie.Create().SetMovieID(movieID).Exec(ctx)
	// A concurrent request may have inserted the same movie between the check
	// and the insert; the unique index makes that a harmless duplicate.
	if err != nil && !ent.IsConstraintError(err) {
		return fmt.Errorf("mark viewed movie: %w", err)
	}
	return nil
}

// viewedMovies resolves the viewed flag for one page of catalogue results.
// A missing row means "not viewed"; the caller only needs the true set.
func (service *DiscoverService) viewedMovies(ctx context.Context, ids []string) (map[string]bool, error) {
	viewed := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return viewed, nil
	}
	records, err := service.database.ViewedMovie.Query().
		Where(viewedmovie.MovieIDIn(ids...)).
		Select(viewedmovie.FieldMovieID).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query viewed movies: %w", err)
	}
	for _, record := range records {
		viewed[record.MovieID] = true
	}
	return viewed, nil
}
