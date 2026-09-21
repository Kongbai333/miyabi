package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/favorite"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/predicate"
	"github.com/ppxb/miyabi/internal/ent/tag"
)

// LibraryFilter narrows the library grid. Every slice is a multi-select: one
// matching value is enough, and an empty slice means "no restriction".
type LibraryFilter struct {
	TagIDs      []int
	ActorIDs    []string
	SeriesIDs   []string
	MakerIDs    []string
	DirectorIDs []string
	Years       []int
	GroupIDs    []int
	// Watched accepts "", "any", "yes" or "no".
	Watched string
}

// LibraryFilterOption is one selectable value plus how many movies carry it.
type LibraryFilterOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// LibraryFilterOptions carries the values this library actually contains, so
// the UI offers real choices instead of the whole JavDB taxonomy.
type LibraryFilterOptions struct {
	Tags      []LibraryFilterOption `json:"tags"`
	Actors    []LibraryFilterOption `json:"actors"`
	Series    []LibraryFilterOption `json:"series"`
	Makers    []LibraryFilterOption `json:"makers"`
	Directors []LibraryFilterOption `json:"directors"`
	Years     []LibraryFilterOption `json:"years"`
}

// libraryFilterOptionLimit keeps a huge library from returning an unusable
// dropdown.
const libraryFilterOptionLimit = 300

// predicate folds every selected dimension into one query. Dimensions are
// ANDed and values inside a dimension are ORed, which is what a multi-select
// filter bar is expected to do.
func (filter LibraryFilter) predicate(scope predicate.Movie) (predicate.Movie, error) {
	if len(filter.TagIDs) > 0 {
		scope = movie.And(scope, movie.HasTagsWith(tag.IDIn(filter.TagIDs...)))
	}
	if len(filter.ActorIDs) > 0 {
		scope = movie.And(scope, movie.HasActorsWith(actor.JavdbIDIn(filter.ActorIDs...)))
	}
	if len(filter.SeriesIDs) > 0 {
		scope = movie.And(scope, movie.SeriesIDIn(filter.SeriesIDs...))
	}
	if len(filter.MakerIDs) > 0 {
		scope = movie.And(scope, movie.MakerIDIn(filter.MakerIDs...))
	}
	if len(filter.DirectorIDs) > 0 {
		scope = movie.And(scope, movie.DirectorIDIn(filter.DirectorIDs...))
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

// FilterOptions lists the distinct values the mounted library contains. The
// library is personal-scale, so one pass over its movies feeds every dimension
// instead of paying for a grouped query per column.
func (service *LibraryService) FilterOptions(ctx context.Context) (LibraryFilterOptions, error) {
	result := LibraryFilterOptions{
		Tags: []LibraryFilterOption{}, Actors: []LibraryFilterOption{}, Series: []LibraryFilterOption{},
		Makers: []LibraryFilterOption{}, Directors: []LibraryFilterOption{}, Years: []LibraryFilterOption{},
	}
	source, err := loadLibrarySource(ctx, service.database)
	if err != nil {
		return result, err
	}
	if source == nil {
		return result, nil
	}
	records, err := service.database.Movie.Query().Where(movie.HasFilesWith(libraryFiles(*source))).
		Select(movie.FieldSeriesID, movie.FieldSeriesName, movie.FieldMakerID, movie.FieldMakerName,
			movie.FieldDirectorID, movie.FieldDirectorName, movie.FieldReleaseDate).
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
	series, makers := map[string]*LibraryFilterOption{}, map[string]*LibraryFilterOption{}
	directors, years := map[string]*LibraryFilterOption{}, map[int]int{}
	for _, record := range records {
		for _, label := range record.Edges.Tags {
			countOption(tags, fmt.Sprint(label.ID), label.Name)
		}
		for _, person := range record.Edges.Actors {
			countOption(actors, person.JavdbID, person.Name)
		}
		countNamed(series, record.SeriesID, record.SeriesName)
		countNamed(makers, record.MakerID, record.MakerName)
		countNamed(directors, record.DirectorID, record.DirectorName)
		if record.ReleaseDate != nil {
			years[record.ReleaseDate.Year()]++
		}
	}
	result.Tags = sortOptions(tags, libraryFilterOptionLimit)
	result.Actors = sortOptions(actors, libraryFilterOptionLimit)
	result.Series = sortOptions(series, libraryFilterOptionLimit)
	result.Makers = sortOptions(makers, libraryFilterOptionLimit)
	result.Directors = sortOptions(directors, libraryFilterOptionLimit)
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

func countNamed(target map[string]*LibraryFilterOption, id, name *string) {
	if id == nil || name == nil {
		return
	}
	countOption(target, *id, *name)
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
