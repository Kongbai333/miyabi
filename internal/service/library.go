package service

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/predicate"
	"github.com/ppxb/miyabi/internal/ent/tag"
	mediaimage "github.com/ppxb/miyabi/internal/image"
)

type LibrarySource struct {
	AccountID string              `json:"account_id"`
	Directory PanLibraryDirectory `json:"directory"`
}

type LibraryMovie struct {
	ID               int                `json:"id"`
	Code             string             `json:"code"`
	Title            string             `json:"title"`
	JavDBID          *string            `json:"javdb_id,omitempty"`
	Cover            *string            `json:"cover,omitempty"`
	Poster           *string            `json:"poster,omitempty"`
	Fanart           string             `json:"fanart,omitempty"`
	ReleaseDate      string             `json:"release_date,omitempty"`
	Duration         int                `json:"duration"`
	Rating           float64            `json:"rating"`
	Size             int64              `json:"size"`
	Director         *LibraryEntity     `json:"director,omitempty"`
	Maker            *LibraryEntity     `json:"maker,omitempty"`
	Series           *LibraryEntity     `json:"series,omitempty"`
	Actors           []LibraryEntity    `json:"actors"`
	Tags             []LibraryTag       `json:"tags"`
	ScrapeStatus     movie.ScrapeStatus `json:"scrape_status"`
	Watched          bool               `json:"watched"`
	FavoriteGroupIDs []int              `json:"favorite_group_ids"`
	// Progress is absent until the movie has actually been played, so the card
	// can tell "opened once" apart from "watched half of it".
	Progress *LibraryProgress `json:"progress,omitempty"`
}

// LibraryProgress is the saved playback position within the mounted source.
type LibraryProgress struct {
	Position float64 `json:"position"`
	Duration float64 `json:"duration"`
}

type LibraryTag struct {
	ID      int    `json:"id"`
	JavDBID string `json:"javdb_id"`
	Name    string `json:"name"`
}

type LibraryEntity struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}

type LibraryPage struct {
	Source  *LibrarySource `json:"source,omitempty"`
	Movies  []LibraryMovie `json:"movies"`
	Total   int            `json:"total"`
	Page    int            `json:"page"`
	HasMore bool           `json:"has_more"`
}

type LibraryFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type LibraryService struct {
	images   *mediaimage.Cache
	database *ent.Client
	drive    *PanService
	tasks    *TaskService
}

func NewLibraryService(database *ent.Client, drive *PanService, tasks *TaskService, images *mediaimage.Cache) *LibraryService {
	return &LibraryService{database: database, drive: drive, tasks: tasks, images: images}
}

// Browsing an existing index only reads SQLite. 115 is contacted when scanning,
// not on each visit to the library or while paging through indexed movies.
func loadLibrarySource(ctx context.Context, database *ent.Client) (*LibrarySource, error) {
	directory, found, err := loadSetting[panLibraryDirectory](ctx, database, panDirectorySetting)
	if err != nil || !found {
		return nil, err
	}
	return &LibrarySource{AccountID: directory.AccountID, Directory: directory.PanLibraryDirectory}, nil
}

func libraryFiles(source LibrarySource) predicate.File {
	return file.And(file.AccountIDEQ(source.AccountID), file.RootIDEQ(source.Directory.ID))
}

func (service *LibraryService) Movies(ctx context.Context, page, limit int, filter LibraryFilter) (LibraryPage, error) {
	result := LibraryPage{Movies: []LibraryMovie{}, Page: page}
	source, err := loadLibrarySource(ctx, service.database)
	if err != nil {
		return result, err
	}
	if source == nil {
		return result, nil
	}
	result.Source = source
	scope := libraryFiles(*source)
	// Filters narrow the same scope as the count, so paging stays consistent.
	movieScope, err := filter.predicate(movie.HasFilesWith(scope))
	if err != nil {
		return result, err
	}
	result.Total, err = service.database.Movie.Query().Where(movieScope).Count(ctx)
	if err != nil {
		return result, fmt.Errorf("count library index: %w", err)
	}
	if result.Total == 0 {
		return result, nil
	}
	query := filter.order(*source, service.database.Movie.Query().Where(movieScope))
	records, err := query.
		Select(movie.FieldID, movie.FieldCode, movie.FieldTitle, movie.FieldJavdbID, movie.FieldCover, movie.FieldPoster,
			movie.FieldFanarts, movie.FieldReleaseDate, movie.FieldDuration, movie.FieldRating,
			movie.FieldDirectorID, movie.FieldDirectorName, movie.FieldMakerID, movie.FieldMakerName,
			movie.FieldSeriesID, movie.FieldSeriesName,
			movie.FieldScrapeStatus, movie.FieldWatched).
		Offset((page - 1) * limit).Limit(limit).
		WithActors(func(query *ent.ActorQuery) {
			query.Select(actor.FieldID, actor.FieldJavdbID, actor.FieldName).Order(ent.Asc(actor.FieldName), ent.Asc(actor.FieldID))
		}).
		WithTags(func(query *ent.TagQuery) {
			query.Select(tag.FieldID, tag.FieldJavdbID, tag.FieldName).Order(ent.Asc(tag.FieldName), ent.Asc(tag.FieldID))
		}).All(ctx)
	if err != nil {
		return result, fmt.Errorf("list library movies: %w", err)
	}
	// One lookup for the whole page; the card only needs the group ids it is in.
	movieIDs := make([]int, len(records))
	for index, record := range records {
		movieIDs[index] = record.ID
	}
	favorites, err := service.favoriteMovieIDs(ctx, movieIDs)
	if err != nil {
		return result, err
	}
	progress, err := service.watchProgress(ctx, *source, movieIDs)
	if err != nil {
		return result, err
	}
	sizes, err := service.movieFileSizes(ctx, *source, movieIDs)
	if err != nil {
		return result, err
	}

	for _, record := range records {
		item := LibraryMovie{
			ID: record.ID, Code: record.Code, Title: record.Title,
			JavDBID: record.JavdbID, Cover: record.Cover, Poster: record.Poster,
			Duration: valueOrZero(record.Duration), Rating: valueOrZero(record.Rating), Size: sizes[record.ID],
			Director: libraryEntity(record.DirectorID, record.DirectorName),
			Maker:    libraryEntity(record.MakerID, record.MakerName), Series: libraryEntity(record.SeriesID, record.SeriesName),
			Actors: make([]LibraryEntity, 0, len(record.Edges.Actors)),
			Tags:   make([]LibraryTag, 0, len(record.Edges.Tags)), ScrapeStatus: record.ScrapeStatus, Watched: record.Watched,
			FavoriteGroupIDs: valueOrEmpty(favorites[record.ID]), Progress: progress[record.ID],
		}
		if record.ReleaseDate != nil {
			item.ReleaseDate = record.ReleaseDate.Format(time.DateOnly)
		}
		if len(record.Fanarts) > 0 {
			item.Fanart = record.Fanarts[0]
		}
		for _, person := range record.Edges.Actors {
			item.Actors = append(item.Actors, LibraryEntity{ID: person.JavdbID, Name: person.Name})
		}
		for _, label := range record.Edges.Tags {
			item.Tags = append(item.Tags, LibraryTag{ID: label.ID, JavDBID: label.JavdbID, Name: label.Name})
		}
		result.Movies = append(result.Movies, item)
	}
	result.HasMore = (page-1)*limit+len(result.Movies) < result.Total
	return result, nil
}

func libraryEntity(id, name *string) *LibraryEntity {
	if name == nil || *name == "" {
		return nil
	}
	return &LibraryEntity{ID: valueOrZero(id), Name: *name}
}

// movieFileSizes returns the total size of each movie's video files in the
// mounted source. A movie split into parts reports the sum, which is what the
// card shows. One grouped query per page keeps the list independent of N+1.
func (service *LibraryService) movieFileSizes(ctx context.Context, source LibrarySource, movieIDs []int) (map[int]int64, error) {
	result := make(map[int]int64, len(movieIDs))
	if len(movieIDs) == 0 {
		return result, nil
	}
	rows := []struct {
		MovieID int   `json:"movie_files"`
		Total   int64 `json:"total"`
	}{}
	if err := service.database.File.Query().
		Where(libraryFiles(source), file.MovieIDIn(movieIDs...)).
		GroupBy(file.FieldMovieID).
		Aggregate(func(selector *sql.Selector) string {
			return sql.As("SUM("+selector.C(file.FieldSize)+")", "total")
		}).
		Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("sum movie file sizes: %w", err)
	}
	for _, row := range rows {
		result[row.MovieID] = row.Total
	}
	return result, nil
}
