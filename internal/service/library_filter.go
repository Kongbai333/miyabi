package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/favorite"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/predicate"
	"github.com/ppxb/miyabi/internal/ent/tag"
	"github.com/ppxb/miyabi/internal/ent/watchhistory"
)

// LibraryFilter narrows the library grid. Every slice is a multi-select: one
// matching value is enough, and an empty slice means "no restriction".
type LibraryFilter struct {
	TagIDs   []int
	ActorIDs []string
	Years    []int
	GroupIDs []int
	// Watched accepts "", "any", "yes" or "no".
	Watched string
	// Sort orders the surviving movies; an empty value keeps the scan order.
	Sort LibrarySort
}

// LibrarySort selects the grid order. "added" is the default: newest scan first.
type LibrarySort string

const (
	LibrarySortAdded   LibrarySort = "added"
	LibrarySortWatched LibrarySort = "watched"
)

// LibraryFilterOption is one selectable value plus how many movies carry it.
type LibraryFilterOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// LibraryFilterOptions carries the values this library actually contains, so
// the UI offers real choices instead of the whole JavDB taxonomy.
type LibraryFilterOptions struct {
	Tags   []LibraryFilterOption `json:"tags"`
	Actors []LibraryFilterOption `json:"actors"`
	Years  []LibraryFilterOption `json:"years"`
}

// libraryFilterOptionLimit keeps a huge library from returning an unusable
// dropdown.
const libraryFilterOptionLimit = 300

// predicate folds every selected dimension into one query. Dimensions are
// ANDed and values inside a dimension are ORed, which is what a multi-select
// filter bar is expected to do. It also rejects the values this service cannot
// honour, so one call validates the whole request.
func (filter LibraryFilter) predicate(scope predicate.Movie) (predicate.Movie, error) {
	switch filter.Sort {
	case "", LibrarySortAdded, LibrarySortWatched:
	default:
		return nil, fmt.Errorf("filter sort must be added or watched, got %q", filter.Sort)
	}
	if len(filter.TagIDs) > 0 {
		scope = movie.And(scope, movie.HasTagsWith(tag.IDIn(filter.TagIDs...)))
	}
	if len(filter.ActorIDs) > 0 {
		scope = movie.And(scope, movie.HasActorsWith(actor.JavdbIDIn(filter.ActorIDs...)))
	}
	if len(filter.GroupIDs) > 0 {
		scope = movie.And(scope, movie.HasFavoritesWith(favorite.GroupIDIn(filter.GroupIDs...)))
	}
	if len(filter.Years) > 0 {
		// A year is a half-open range, so several years OR together.
		years := make([]predicate.Movie, 0, len(filter.Years))
		for _, year := range filter.Years {
			if year < 1900 || year > 2999 {
				return nil, fmt.Errorf("filter year out of range: %d", year)
			}
			// Release dates are stored as UTC calendar dates, so the window must
			// be UTC too: a local-midnight window drops January 1st everywhere
			// west of Greenwich.
			start := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
			years = append(years, movie.And(
				movie.ReleaseDateNotNil(),
				movie.ReleaseDateGTE(start),
				movie.ReleaseDateLT(start.AddDate(1, 0, 0)),
			))
		}
		scope = movie.And(scope, movie.Or(years...))
	}
	switch filter.Watched {
	case "", "any":
	case "yes":
		scope = movie.And(scope, movie.Watched(true))
	case "no":
		scope = movie.And(scope, movie.Watched(false))
	default:
		return nil, fmt.Errorf("filter watched must be yes or no, got %q", filter.Watched)
	}
	return scope, nil
}

// order appends the grid order to the query. "watched" needs the history table,
// which is joined rather than denormalised so a cleared or re-mounted source
// cannot leave a stale timestamp behind.
func (filter LibraryFilter) order(source LibrarySource, query *ent.MovieQuery) *ent.MovieQuery {
	if filter.Sort == LibrarySortWatched {
		return query.Order(orderByWatchTime(source), ent.Desc(movie.FieldID))
	}
	return query.Order(ent.Desc(movie.FieldCreatedAt), ent.Desc(movie.FieldID))
}

// orderByWatchTime sorts the grid by when each movie was last opened, with
// never-watched movies last. The source is part of the join, so a movie opened
// from a previously mounted directory is ordered as if it had never been
// watched instead of leaking an old timestamp into the current library.
func orderByWatchTime(source LibrarySource) func(*sql.Selector) {
	return func(selector *sql.Selector) {
		history := sql.Table(watchhistory.Table).As("watch_order")
		selector.LeftJoin(history).OnP(sql.And(
			sql.ColumnsEQ(selector.C(movie.FieldID), history.C(watchhistory.FieldMovieID)),
			sql.EQ(history.C(watchhistory.FieldAccountID), source.AccountID),
			sql.EQ(history.C(watchhistory.FieldRootID), source.Directory.ID),
		))
		selector.OrderExpr(sql.Expr(history.C(watchhistory.FieldWatchedAt) + " DESC NULLS LAST"))
	}
}

// FilterOptions lists the distinct values the mounted library contains. The
// library is personal-scale, so one pass over its movies feeds every dimension
// instead of paying for a grouped query per column.
func (service *LibraryService) FilterOptions(ctx context.Context) (LibraryFilterOptions, error) {
	result := LibraryFilterOptions{
		Tags: []LibraryFilterOption{}, Actors: []LibraryFilterOption{}, Years: []LibraryFilterOption{},
	}
	source, err := loadLibrarySource(ctx, service.database)
	if err != nil {
		return result, err
	}
	if source == nil {
		return result, nil
	}
	records, err := service.database.Movie.Query().Where(movie.HasFilesWith(libraryFiles(*source))).
		Select(movie.FieldReleaseDate).
		WithTags(func(query *ent.TagQuery) {
			query.Select(tag.FieldID, tag.FieldName)
		}).
		WithActors(func(query *ent.ActorQuery) {
			query.Select(actor.FieldJavdbID, actor.FieldName)
		}).All(ctx)
	if err != nil {
		return result, fmt.Errorf("list filter values: %w", err)
	}

	tags, actors := map[string]*LibraryFilterOption{}, map[string]*LibraryFilterOption{}
	years := map[int]int{}
	for _, record := range records {
		for _, label := range record.Edges.Tags {
			countOption(tags, fmt.Sprint(label.ID), label.Name)
		}
		for _, person := range record.Edges.Actors {
			countOption(actors, person.JavdbID, person.Name)
		}
		if record.ReleaseDate != nil {
			years[record.ReleaseDate.Year()]++
		}
	}
	result.Tags = sortOptions(tags, libraryFilterOptionLimit)
	result.Actors = sortOptions(actors, libraryFilterOptionLimit)
	result.Years = sortYears(years, libraryFilterOptionLimit)
	return result, nil
}

// countOption accumulates how many movies carry one selectable value. A value
// the library only half knows - a name without its catalogue id, or the other
// way round - cannot be filtered by, so it is left out.
func countOption(target map[string]*LibraryFilterOption, id, name string) {
	if id == "" || name == "" {
		return
	}
	if option, found := target[id]; found {
		option.Count++
		return
	}
	target[id] = &LibraryFilterOption{ID: id, Name: name, Count: 1}
}

func sortOptions(source map[string]*LibraryFilterOption, limit int) []LibraryFilterOption {
	options := make([]LibraryFilterOption, 0, len(source))
	for _, option := range source {
		options = append(options, *option)
	}
	sort.Slice(options, func(left, right int) bool {
		if options[left].Name != options[right].Name {
			return options[left].Name < options[right].Name
		}
		return options[left].ID < options[right].ID
	})
	if len(options) > limit {
		options = options[:limit]
	}
	return options
}

func sortYears(years map[int]int, limit int) []LibraryFilterOption {
	ordered := make([]int, 0, len(years))
	for year := range years {
		ordered = append(ordered, year)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ordered)))
	if len(ordered) > limit {
		ordered = ordered[:limit]
	}
	options := make([]LibraryFilterOption, 0, len(ordered))
	for _, year := range ordered {
		options = append(options, LibraryFilterOption{ID: fmt.Sprint(year), Name: fmt.Sprint(year), Count: years[year]})
	}
	return options
}
