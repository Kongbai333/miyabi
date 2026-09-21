package service

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/ent"
)

// filterFixture mounts one source and indexes three movies: a fully catalogued
// 2026 one, a bare 2025 one, and one that belongs to another account so every
// dimension can be shown to respect the mounted source.
type filterFixture struct {
	library *LibraryService
	source  LibrarySource
	first   *ent.Movie
	second  *ent.Movie
	tag     *ent.Tag
	group   FavoriteGroupItem
}

func newFilterFixture(t *testing.T) filterFixture {
	t.Helper()
	library, _, payload := libraryFixture(t)
	ctx := t.Context()
	source := payload.Source
	database := library.database
	tag := database.Tag.Create().SetJavdbID("tag-1").SetName("Tag").SetCategoryID("category").SaveX(ctx)
	hiddenTag := database.Tag.Create().SetJavdbID("tag-hidden").SetName("Hidden tag").SetCategoryID("category").SaveX(ctx)
	actor := database.Actor.Create().SetJavdbID("actor-1").SetName("Actor").SaveX(ctx)
	other := database.Actor.Create().SetJavdbID("actor-2").SetName("Other actor").SaveX(ctx)
	hiddenActor := database.Actor.Create().SetJavdbID("actor-hidden").SetName("Hidden actor").SaveX(ctx)
	group, err := library.CreateFavoriteGroup(ctx, "喜欢")
	if err != nil {
		t.Fatal(err)
	}
	attach := func(film *ent.Movie, account string) {
		database.File.Create().SetFileID(film.Code).SetName(film.Code + ".mp4").SetSize(1 << 30).
			SetAccountID(account).SetRootID(source.Directory.ID).SetMovie(film).ExecX(ctx)
	}
	release := time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC)
	first := database.Movie.Create().SetCode("ABP-001").SetTitle("First").SetReleaseDate(release).
		SetWatched(true).AddTags(tag).AddActors(actor).SaveX(ctx)
	second := database.Movie.Create().SetCode("ABP-002").SetTitle("Second").
		SetReleaseDate(release.AddDate(-1, 0, 0)).AddActors(other).SaveX(ctx)
	hidden := database.Movie.Create().SetCode("ABP-003").SetTitle("Hidden").SetReleaseDate(release).
		AddTags(hiddenTag).AddActors(hiddenActor).SaveX(ctx)
	attach(first, source.AccountID)
	attach(second, source.AccountID)
	attach(hidden, "other")
	if err := library.SetFavorite(ctx, first.ID, []int{group.ID}); err != nil {
		t.Fatal(err)
	}
	return filterFixture{library: library, source: source, first: first, second: second, tag: tag, group: group}
}

// filterIDs returns the matching movie ids in a stable order and refuses a page
// whose total disagrees with what it returned.
func filterIDs(t *testing.T, library *LibraryService, filter LibraryFilter) []int {
	t.Helper()
	page, err := library.Movies(t.Context(), 1, 50, filter)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int, 0, len(page.Movies))
	for _, film := range page.Movies {
		ids = append(ids, film.ID)
	}
	sort.Ints(ids)
	if page.Total != len(ids) {
		t.Fatalf("total %d disagrees with %d returned movies", page.Total, len(ids))
	}
	return ids
}

func TestLibraryFilterNarrowsEveryDimension(t *testing.T) {
	fixture := newFilterFixture(t)
	for _, scenario := range []struct {
		name   string
		filter LibraryFilter
		want   []int
	}{
		{name: "no filter", filter: LibraryFilter{}, want: []int{fixture.first.ID, fixture.second.ID}},
		{name: "tag", filter: LibraryFilter{TagIDs: []int{fixture.tag.ID}}, want: []int{fixture.first.ID}},
		{name: "actor", filter: LibraryFilter{ActorIDs: []string{"actor-2"}}, want: []int{fixture.second.ID}},
		{name: "group", filter: LibraryFilter{GroupIDs: []int{fixture.group.ID}}, want: []int{fixture.first.ID}},
		{name: "watched", filter: LibraryFilter{Watched: "yes"}, want: []int{fixture.first.ID}},
		{name: "unwatched", filter: LibraryFilter{Watched: "no"}, want: []int{fixture.second.ID}},
		{name: "any watched", filter: LibraryFilter{Watched: "any"}, want: []int{fixture.first.ID, fixture.second.ID}},
		{name: "unknown tag", filter: LibraryFilter{TagIDs: []int{9999}}, want: []int{}},
		{name: "unknown group", filter: LibraryFilter{GroupIDs: []int{9999}}, want: []int{}},
		// The third movie carries these values, but it sits in another account.
		{name: "tag outside the mounted source", filter: LibraryFilter{TagIDs: []int{9998}}, want: []int{}},
		{name: "actor outside the mounted source", filter: LibraryFilter{ActorIDs: []string{"actor-hidden"}}, want: []int{}},
		{name: "value outside the mounted source", filter: LibraryFilter{ActorIDs: []string{"actor-1"}, Years: []int{2025}}, want: []int{}},
		// Values inside one dimension OR together while dimensions AND together.
		{name: "several years", filter: LibraryFilter{Years: []int{2025, 2026}}, want: []int{fixture.first.ID, fixture.second.ID}},
		{name: "tag and year", filter: LibraryFilter{TagIDs: []int{fixture.tag.ID}, Years: []int{2026}}, want: []int{fixture.first.ID}},
		{name: "year without the actor", filter: LibraryFilter{Years: []int{2026}, ActorIDs: []string{"actor-2"}}, want: []int{}},
		{name: "group and watched", filter: LibraryFilter{GroupIDs: []int{fixture.group.ID}, Watched: "no"}, want: []int{}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			got := filterIDs(t, fixture.library, scenario.filter)
			if len(got) != len(scenario.want) {
				t.Fatalf("filter %+v selected %v, want %v", scenario.filter, got, scenario.want)
			}
			for index := range got {
				if got[index] != scenario.want[index] {
					t.Fatalf("filter %+v selected %v, want %v", scenario.filter, got, scenario.want)
				}
			}
		})
	}
}

// A release date is a calendar date stored at UTC midnight, so the year window
// is half open: both dates of the year are in and its neighbours are out.
func TestLibraryFilterYearWindowFollowsTheStoredUTCDate(t *testing.T) {
	library, _, payload := libraryFixture(t)
	ctx := t.Context()
	for _, release := range []string{"2025-12-31", "2026-01-01", "2026-12-31", "2027-01-01"} {
		date, err := time.Parse(time.DateOnly, release)
		if err != nil {
			t.Fatal(err)
		}
		film := library.database.Movie.Create().SetCode(release).SetReleaseDate(date).SaveX(ctx)
		library.database.File.Create().SetFileID(release).SetName(release + ".mp4").SetSize(1 << 30).
			SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).SetMovie(film).ExecX(ctx)
	}

	page, err := library.Movies(ctx, 1, 50, LibraryFilter{Years: []int{2026}})
	if err != nil {
		t.Fatal(err)
	}
	selected := make([]string, 0, len(page.Movies))
	for _, film := range page.Movies {
		selected = append(selected, film.ReleaseDate)
	}
	sort.Strings(selected)
	if len(selected) != 2 || selected[0] != "2026-01-01" || selected[1] != "2026-12-31" {
		t.Fatalf("year 2026 selected %v", selected)
	}
}

func TestLibraryFilterRejectsValuesItCannotHonour(t *testing.T) {
	library, _, _ := libraryFixture(t)
	for _, filter := range []LibraryFilter{
		{Watched: "sometimes"},
		{Years: []int{1899}},
		{Years: []int{3000}},
		{Sort: "random"},
	} {
		if _, err := library.Movies(t.Context(), 1, 20, filter); err == nil {
			t.Fatalf("filter %+v was accepted", filter)
		}
	}
}

// codes returns the page in the order the service chose, which is what the
// sort tests assert on.
func codes(t *testing.T, library *LibraryService, filter LibraryFilter) []string {
	t.Helper()
	page, err := library.Movies(t.Context(), 1, 50, filter)
	if err != nil {
		t.Fatal(err)
	}
	result := make([]string, 0, len(page.Movies))
	for _, film := range page.Movies {
		result = append(result, film.Code)
	}
	return result
}

// The sort has to survive two traps: a movie opened from a previously mounted
// directory must not be ordered by that stale timestamp, and a movie that was
// never opened must still appear.
func TestLibrarySortWatchedOrdersByRecentPlayback(t *testing.T) {
	library, _, payload := libraryFixture(t)
	ctx := t.Context()
	source := payload.Source
	added := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	older := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)

	// Created in the opposite order of the playback order, so the two sorts
	// cannot accidentally agree.
	create := func(code string, createdAt time.Time, history func(*ent.WatchHistoryCreate)) {
		film := library.database.Movie.Create().SetCode(code).SetCreatedAt(createdAt).SaveX(ctx)
		library.database.File.Create().SetFileID(code).SetName(code + ".mp4").SetSize(1 << 30).
			SetAccountID(source.AccountID).SetRootID(source.Directory.ID).SetMovie(film).ExecX(ctx)
		if history != nil {
			history(library.database.WatchHistory.Create().SetMovie(film).SetSessionID(code))
		}
	}
	create("OLDEST-ADDED", added, func(create *ent.WatchHistoryCreate) {
		create.SetAccountID(source.AccountID).SetRootID(source.Directory.ID).SetWatchedAt(recent).ExecX(ctx)
	})
	create("MIDDLE-ADDED", added.AddDate(0, 1, 0), func(create *ent.WatchHistoryCreate) {
		create.SetAccountID(source.AccountID).SetRootID(source.Directory.ID).SetWatchedAt(older).ExecX(ctx)
	})
	create("NEWEST-ADDED", added.AddDate(0, 2, 0), nil)
	create("STALE-SOURCE", added.AddDate(0, 3, 0), func(create *ent.WatchHistoryCreate) {
		create.SetAccountID("other-account").SetRootID("other-root").SetWatchedAt(recent.AddDate(0, 1, 0)).ExecX(ctx)
	})

	if got := codes(t, library, LibraryFilter{Sort: LibrarySortAdded}); len(got) != 4 ||
		got[0] != "STALE-SOURCE" || got[1] != "NEWEST-ADDED" || got[2] != "MIDDLE-ADDED" || got[3] != "OLDEST-ADDED" {
		t.Fatalf("added order = %v", got)
	}
	got := codes(t, library, LibraryFilter{Sort: LibrarySortWatched})
	if len(got) != 4 || got[0] != "OLDEST-ADDED" || got[1] != "MIDDLE-ADDED" {
		t.Fatalf("watched order = %v", got)
	}
	tail := append([]string{}, got[2:]...)
	sort.Strings(tail)
	if tail[0] != "NEWEST-ADDED" || tail[1] != "STALE-SOURCE" {
		t.Fatalf("never-played movies are not last: %v", got)
	}
	// The count has to agree with the joined page: the left join must not
	// duplicate a movie that carries more than one history row.
	page, err := library.Movies(ctx, 1, 50, LibraryFilter{Sort: LibrarySortWatched})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 4 || len(page.Movies) != 4 {
		t.Fatalf("watched order changed the total: %+v", page)
	}
}

func TestLibraryMoviesCarrySourceScopedProgress(t *testing.T) {
	fixture := newFilterFixture(t)
	ctx := t.Context()
	source := fixture.source
	played := fixture.library.database.Movie.Create().SetCode("ABP-010").SaveX(ctx)
	fixture.library.database.File.Create().SetFileID("played").SetName("played.mp4").SetSize(1 << 30).
		SetAccountID(source.AccountID).SetRootID(source.Directory.ID).SetMovie(played).ExecX(ctx)
	fixture.library.database.WatchHistory.Create().SetAccountID(source.AccountID).SetRootID(source.Directory.ID).
		SetMovie(played).SetSessionID("played").SetPosition(61.5).SetDuration(7200).ExecX(ctx)
	// Opened but never played: a history row without a duration is not progress.
	opened := fixture.library.database.Movie.Create().SetCode("ABP-011").SetWatched(true).SaveX(ctx)
	fixture.library.database.File.Create().SetFileID("opened").SetName("opened.mp4").SetSize(1 << 30).
		SetAccountID(source.AccountID).SetRootID(source.Directory.ID).SetMovie(opened).ExecX(ctx)
	fixture.library.database.WatchHistory.Create().SetAccountID(source.AccountID).SetRootID(source.Directory.ID).
		SetMovie(opened).SetSessionID("opened").ExecX(ctx)
	// Progress recorded against a directory that is no longer mounted.
	stale := fixture.library.database.Movie.Create().SetCode("ABP-012").SaveX(ctx)
	fixture.library.database.File.Create().SetFileID("stale").SetName("stale.mp4").SetSize(1 << 30).
		SetAccountID(source.AccountID).SetRootID(source.Directory.ID).SetMovie(stale).ExecX(ctx)
	fixture.library.database.WatchHistory.Create().SetAccountID("other-account").SetRootID("other-root").
		SetMovie(stale).SetSessionID("stale").SetPosition(5).SetDuration(600).ExecX(ctx)

	page, err := fixture.library.Movies(ctx, 1, 50, LibraryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	byCode := map[string]LibraryMovie{}
	for _, film := range page.Movies {
		byCode[film.Code] = film
	}
	if progress := byCode["ABP-010"].Progress; progress == nil || progress.Position != 61.5 || progress.Duration != 7200 {
		t.Fatalf("played movie progress = %+v", progress)
	}
	if progress := byCode["ABP-011"].Progress; progress != nil {
		t.Fatalf("an unplayed movie reported progress: %+v", progress)
	}
	if progress := byCode["ABP-012"].Progress; progress != nil {
		t.Fatalf("another source's progress leaked in: %+v", progress)
	}
}

func TestLibraryFilterOptionsCountOnlyTheMountedLibrary(t *testing.T) {
	fixture := newFilterFixture(t)
	options, err := fixture.library.FilterOptions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// The third movie carries its own tag and actor, so an unfiltered list
	// proves the mounted source bounds every dimension.
	if len(options.Tags) != 1 || options.Tags[0].ID != fmt.Sprint(fixture.tag.ID) ||
		options.Tags[0].Name != "Tag" || options.Tags[0].Count != 1 {
		t.Fatalf("tag options = %+v", options.Tags)
	}
	if len(options.Actors) != 2 || options.Actors[0].ID != "actor-1" || options.Actors[0].Count != 1 ||
		options.Actors[1].ID != "actor-2" || options.Actors[1].Count != 1 {
		t.Fatalf("actor options = %+v", options.Actors)
	}
	if len(options.Years) != 2 || options.Years[0].ID != "2026" || options.Years[0].Count != 1 ||
		options.Years[1].ID != "2025" || options.Years[1].Count != 1 {
		t.Fatalf("year options = %+v", options.Years)
	}
}

// A selectable value needs both its catalogue id and its label: an option the
// request cannot address would filter the grid to nothing.
func TestCountOptionSkipsValuesWithoutAnID(t *testing.T) {
	options := map[string]*LibraryFilterOption{}
	countOption(options, "", "Named but unaddressable")
	countOption(options, "actor-1", "")
	countOption(options, "actor-1", "Actor")
	countOption(options, "actor-1", "Actor")
	if len(options) != 1 || options["actor-1"] == nil || options["actor-1"].Count != 2 {
		t.Fatalf("options = %+v", options)
	}
}

func TestLibraryFilterOptionsStayEmptyArraysWithoutAMountedLibrary(t *testing.T) {
	library, _, _ := libraryFixture(t)
	library.database.Setting.Delete().ExecX(t.Context())
	options, err := library.FilterOptions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, dimension := range [][]LibraryFilterOption{options.Tags, options.Actors, options.Years} {
		if dimension == nil || len(dimension) != 0 {
			t.Fatalf("unmounted library returned %+v", options)
		}
	}
}
