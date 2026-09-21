package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// Viewed movies are the JavDB IDs the user has opened in the detail page. They
// live in the settings table as a single bounded list for now; REFACTOR_PLAN
// B4 moves them to a dedicated table under library alongside watch history.
const (
	viewedMoviesSetting = "browse.viewed_movies"
	maxViewedMovies     = 5000
)

type viewedMoviesPayload struct {
	IDs []string `json:"ids"`
}

// ViewedMovieIDs returns viewed JavDB IDs, most recent first.
func (service *DiscoverService) ViewedMovieIDs(ctx context.Context) ([]string, error) {
	service.viewedMu.Lock()
	defer service.viewedMu.Unlock()
	payload, err := service.loadViewedMovies(ctx)
	if err != nil {
		return nil, err
	}
	if payload.IDs == nil {
		return []string{}, nil
	}
	return payload.IDs, nil
}

// AddViewedMovieIDs prepends new IDs, drops blanks and duplicates, and keeps
// the list within maxViewedMovies by evicting the oldest entries.
func (service *DiscoverService) AddViewedMovieIDs(ctx context.Context, ids []string) error {
	service.viewedMu.Lock()
	defer service.viewedMu.Unlock()

	payload, err := service.loadViewedMovies(ctx)
	if err != nil {
		return err
	}
	merged := mergeViewedMovies(ids, payload.IDs, maxViewedMovies)
	if slices.Equal(merged, payload.IDs) {
		return nil
	}
	if err := saveSetting(ctx, service.database, viewedMoviesSetting, viewedMoviesPayload{IDs: merged}); err != nil {
		return fmt.Errorf("save viewed movies: %w", err)
	}
	return nil
}

func (service *DiscoverService) loadViewedMovies(ctx context.Context) (viewedMoviesPayload, error) {
	payload, _, err := loadSetting[viewedMoviesPayload](ctx, service.database, viewedMoviesSetting)
	if err != nil {
		return viewedMoviesPayload{}, fmt.Errorf("load viewed movies: %w", err)
	}
	return payload, nil
}

// mergeViewedMovies places incoming IDs (in order, trimmed, deduplicated)
// ahead of existing ones and truncates to limit.
func mergeViewedMovies(incoming, existing []string, limit int) []string {
	merged := make([]string, 0, min(len(incoming)+len(existing), limit))
	seen := make(map[string]struct{}, len(incoming)+len(existing))
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || len(merged) >= limit {
			return
		}
		if _, dup := seen[id]; dup {
			return
		}
		seen[id] = struct{}{}
		merged = append(merged, id)
	}
	for _, id := range incoming {
		add(id)
	}
	for _, id := range existing {
		add(id)
	}
	return merged
}
