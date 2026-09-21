package service

import (
	"errors"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
)

func TestMarkViewedIsIdempotentAndResolvesPerBatch(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	// No JavDB client or 115 account: viewed state must stay entirely local.
	discover := &DiscoverService{database: store.Client}
	ctx := t.Context()

	viewed := func(ids ...string) map[string]bool {
		t.Helper()
		result, err := discover.viewedMovies(ctx, ids)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	if got := viewed("movie-a", "movie-b"); len(got) != 0 {
		t.Fatalf("unvisited movies reported as viewed: %v", got)
	}
	for range 3 {
		if err := discover.MarkViewed(ctx, "movie-a"); err != nil {
			t.Fatal(err)
		}
	}
	if got := viewed("movie-a", "movie-b"); len(got) != 1 || !got["movie-a"] || got["movie-b"] {
		t.Fatalf("viewed flags = %v", got)
	}
	// Revisiting must not create a second row or shift the first-viewed time.
	first, err := store.Client.ViewedMovie.Query().Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := discover.MarkViewed(ctx, "movie-a"); err != nil {
		t.Fatal(err)
	}
	again, err := store.Client.ViewedMovie.Query().Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID || !again.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("revisit rewrote the record: %+v then %+v", first, again)
	}

	if err := discover.MarkViewed(ctx, "  "); !errors.Is(err, ErrInvalidViewedMovie) {
		t.Fatalf("blank movie id error = %v", err)
	}
}

func TestMovieStatesReportViewedWithoutALibrarySource(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	discover := &DiscoverService{database: store.Client}
	ctx := t.Context()

	identities := []MovieIdentity{{ID: "movie-a", Code: "ABP-001"}, {ID: "movie-b", Code: "ABP-002"}}
	if err := discover.MarkViewed(ctx, "movie-a"); err != nil {
		t.Fatal(err)
	}
	states, err := discover.MovieStates(ctx, identities)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 || !states[0].Viewed || states[1].Viewed {
		t.Fatalf("viewed flags = %+v", states)
	}
	if states[0].State != MovieNotInLibrary || states[1].State != MovieNotInLibrary {
		t.Fatalf("unmounted library changed the admission state: %+v", states)
	}
}
