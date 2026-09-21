package service

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/ent"
)

// filterFixture mounts a source and indexes three movies: a fully catalogued
// 2026 one, a bare 2025 one, and one that belongs to another account so the
// filter can be shown to respect the mounted source.
type filterFixture struct {
	library *LibraryService
	first   *ent.Movie
	second  *ent.Movie
	tag     *ent.Tag
	group   FavoriteGroupItem
}

func newFilterFixture(t *testing.T) filterFixture {
	t.Helper()
	library, _, payload := libraryFixture(t)
	ctx := t.Context()
	database := library.database
	tag := database.Tag.Create().SetJavdbID("tag-1").SetName("Tag").SetCategoryID("category").SaveX(ctx)
	actor := database.Actor.Create().SetJavdbID("actor-1").SetName("Actor").SaveX(ctx)
	other := database.Actor.Create().SetJavdbID("actor-2").SetName("Other actor").SaveX(ctx)
	group, err := library.CreateFavoriteGroup(ctx, "喜欢")
	if err != nil {
		t.Fatal(err)
	}
	attach := func(film *ent.Movie, account string) {
		database.File.Create().SetFileID(film.Code).SetName(film.Code + ".mp4").SetSize(1 << 30).
			SetAccountID(account).SetRootID(payload.Source.Directory.ID).SetMovie(film).ExecX(ctx)
	}
	release := time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC)
	first := database.Movie.Create().SetCode("ABP-001").SetTitle("First").SetReleaseDate(release).
		SetSeriesID("series-1").SetSeriesName("Series").SetMakerID("maker-1").SetMakerName("Maker").
		SetDirectorID("director-1").SetDirectorName("Director").SetWatched(true).
		AddTags(tag).AddActors(actor).SaveX(ctx)
	second := database.Movie.Create().SetCode("ABP-002").SetTitle("Second").
		SetReleaseDate(release.AddDate(-1, 0, 0)).AddActors(other).SaveX(ctx)
	hidden := database.Movie.Create().SetCode("ABP-003").SetTitle("Hidden").SetReleaseDate(release).
		SetSeriesID("series-hidden").SetSeriesName("Hidden series").
		SetMakerID("maker-hidden").SetMakerName("Hidden maker").
		SetDirectorID("director-hidden").SetDirectorName("Hidden director").
		AddTags(tag).AddActors(actor).SaveX(ctx)
	attach(first, payload.Source.AccountID)
	attach(second, payload.Source.AccountID)
	attach(hidden, "other")
	if err := library.SetFavorite(ctx, first.ID, []int{group.ID}); err != nil {
		t.Fatal(err)
	}
	return filterFixture{library: library, first: first, second: second, tag: tag, group: group}
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
		{name: "series", filter: LibraryFilter{SeriesIDs: []string{"series-1"}}, want: []int{fixture.first.ID}},
		{name: "maker", filter: LibraryFilter{MakerIDs: []string{"maker-1"}}, want: []int{fixture.first.ID}},
		{name: "director", filter: LibraryFilter{DirectorIDs: []string{"director-1"}}, want: []int{fixture.first.ID}},
		{name: "group", filter: LibraryFilter{GroupIDs: []int{fixture.group.ID}}, want: []int{fixture.first.ID}},
		{name: "watched", filter: LibraryFilter{Watched: "yes"}, want: []int{fixture.first.ID}},
		{name: "unwatched", filter: LibraryFilter{Watched: "no"}, want: []int{fixture.second.ID}},
		{name: "any watched", filter: LibraryFilter{Watched: "any"}, want: []int{fixture.first.ID, fixture.second.ID}},
		{name: "unknown tag", filter: LibraryFilter{TagIDs: []int{9999}}, want: []int{}},
		{name: "unknown group", filter: LibraryFilter{GroupIDs: []int{9999}}, want: []int{}},
		// The third movie carries these values, but it sits in another account.
		{name: "value outside the mounted source", filter: LibraryFilter{SeriesIDs: []string{"series-hidden"}}, want: []int{}},
		{name: "value outside the mounted source by actor", filter: LibraryFilter{ActorIDs: []string{"actor-1"}, Years: []int{2025}}, want: []int{}},
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

func TestLibraryFilterRejectsAnUnknownWatchedStateAndOutOfRangeYears(t *testing.T) {
	library, _, _ := libraryFixture(t)
	for _, filter := range []LibraryFilter{
		{Watched: "sometimes"},
		{Years: []int{1899}},
		{Years: []int{3000}},
	} {
		if _, err := library.Movies(t.Context(), 1, 20, filter); err == nil {
			t.Fatalf("filter %+v was accepted", filter)
		}
	}
}

func TestLibraryFilterOptionsCountOnlyTheMountedLibrary(t *testing.T) {
	fixture := newFilterFixture(t)
	options, err := fixture.library.FilterOptions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Tags) != 1 || options.Tags[0].ID != fmt.Sprint(fixture.tag.ID) ||
		options.Tags[0].Name != "Tag" || options.Tags[0].Count != 1 {
		t.Fatalf("tag options = %+v", options.Tags)
	}
	if len(options.Actors) != 2 || options.Actors[0].ID != "actor-1" || options.Actors[0].Count != 1 ||
		options.Actors[1].ID != "actor-2" || options.Actors[1].Count != 1 {
		t.Fatalf("actor options = %+v", options.Actors)
	}
	if len(options.Series) != 1 || options.Series[0].ID != "series-1" || options.Series[0].Count != 1 {
		t.Fatalf("series options = %+v", options.Series)
	}
	if len(options.Makers) != 1 || options.Makers[0].ID != "maker-1" || options.Makers[0].Name != "Maker" ||
		options.Makers[0].Count != 1 {
		t.Fatalf("maker options = %+v", options.Makers)
	}
	if len(options.Directors) != 1 || options.Directors[0].ID != "director-1" || options.Directors[0].Count != 1 {
		t.Fatalf("director options = %+v", options.Directors)
	}
	if len(options.Years) != 2 || options.Years[0].ID != "2026" || options.Years[0].Count != 1 ||
		options.Years[1].ID != "2025" || options.Years[1].Count != 1 {
		t.Fatalf("year options = %+v", options.Years)
	}
}

func TestLibraryFilterOptionsSkipValuesWithoutACatalogueID(t *testing.T) {
	library, _, payload := libraryFixture(t)
	ctx := t.Context()
	legacy := library.database.Movie.Create().SetCode("ABP-001").SetSeriesName("Legacy series").
		SetMakerName("Legacy maker").SetDirectorName("Legacy director").SaveX(ctx)
	library.database.File.Create().SetFileID("legacy").SetName("legacy.mp4").SetSize(1 << 30).
		SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).SetMovie(legacy).ExecX(ctx)

	options, err := library.FilterOptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Series) != 0 || len(options.Makers) != 0 || len(options.Directors) != 0 || len(options.Years) != 0 {
		t.Fatalf("a name without its id became selectable: %+v", options)
	}
}

func TestLibraryFilterOptionsStayEmptyArraysWithoutAMountedLibrary(t *testing.T) {
	library, _, _ := libraryFixture(t)
	library.database.Setting.Delete().ExecX(t.Context())
	options, err := library.FilterOptions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, dimension := range [][]LibraryFilterOption{
		options.Tags, options.Actors, options.Series, options.Makers, options.Directors, options.Years,
	} {
		if dimension == nil || len(dimension) != 0 {
			t.Fatalf("unmounted library returned %+v", options)
		}
	}
}
