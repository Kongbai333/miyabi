package service

import (
	"context"
	"fmt"
	"strings"

	"entgo.io/ent/dialect/sql"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/favorite"
	"github.com/ppxb/miyabi/internal/ent/favoritegroup"
	"github.com/ppxb/miyabi/internal/ent/movie"
)

var (
	ErrFavoriteGroupName = domain.E(domain.KindInvalid, "分组名称不能为空", nil)
	ErrFavoriteGroupUsed = domain.E(domain.KindConflict, "分组名已存在", nil)
	ErrFavoriteGroupGone = domain.E(domain.KindNotFound, "分组不存在", nil)
	ErrFavoriteMovieGone = domain.E(domain.KindNotFound, "影片不存在", nil)
)

// favoriteNameLimit keeps a group label readable as a tab.
const favoriteNameLimit = 24

type FavoriteGroupItem struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// SetFavorite replaces a movie's groups with the supplied set. An empty set
// clears the star, so the caller does not need a separate remove call.
func (service *LibraryService) SetFavorite(ctx context.Context, movieID int, groupIDs []int) error {
	if _, err := service.database.Movie.Query().Where(movie.ID(movieID)).OnlyID(ctx); err != nil {
		if ent.IsNotFound(err) {
			return ErrFavoriteMovieGone
		}
		return fmt.Errorf("set favorite: %w", err)
	}
	// De-duplicate so a doubled id cannot violate the pair index.
	unique := make([]int, 0, len(groupIDs))
	seen := make(map[int]bool, len(groupIDs))
	for _, id := range groupIDs {
		if id <= 0 {
			return fmt.Errorf("set favorite: invalid group id %d", id)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	return ent.WithTx(ctx, service.database, func(tx *ent.Tx) error {
		if len(unique) > 0 {
			found, err := tx.FavoriteGroup.Query().Where(favoritegroup.IDIn(unique...)).Count(ctx)
			if err != nil {
				return fmt.Errorf("check favorite groups: %w", err)
			}
			if found != len(unique) {
				return ErrFavoriteGroupGone
			}
		}
		if _, err := tx.Favorite.Delete().Where(favorite.MovieIDEQ(movieID)).Exec(ctx); err != nil {
			return fmt.Errorf("clear favorite: %w", err)
		}
		if len(unique) == 0 {
			return nil
		}
		builders := make([]*ent.FavoriteCreate, len(unique))
		for index, groupID := range unique {
			builders[index] = tx.Favorite.Create().SetMovieID(movieID).SetGroupID(groupID)
		}
		if err := tx.Favorite.CreateBulk(builders...).Exec(ctx); err != nil {
			return fmt.Errorf("set favorite: %w", err)
		}
		return nil
	})
}

// FavoriteGroups lists groups with their movie counts for the tab row.
func (service *LibraryService) FavoriteGroups(ctx context.Context) ([]FavoriteGroupItem, error) {
	records, err := service.database.FavoriteGroup.Query().
		Order(ent.Asc(favoritegroup.FieldID)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list favorite groups: %w", err)
	}
	result := make([]FavoriteGroupItem, len(records))
	if len(records) == 0 {
		return result, nil
	}
	ids := make([]int, len(records))
	for index, record := range records {
		result[index] = FavoriteGroupItem{ID: record.ID, Name: record.Name}
		ids[index] = record.ID
	}
	counts := make([]struct {
		GroupID int `json:"group_id"`
		Total   int `json:"total"`
	}, 0, len(records))
	if err := service.database.Favorite.Query().
		Where(favorite.GroupIDIn(ids...)).
		GroupBy(favorite.FieldGroupID).
		Aggregate(func(selector *sql.Selector) string {
			return sql.As("COUNT("+selector.C(favorite.FieldID)+")", "total")
		}).
		Scan(ctx, &counts); err != nil {
		return nil, fmt.Errorf("count favorite groups: %w", err)
	}
	byGroup := make(map[int]int, len(counts))
	for _, row := range counts {
		byGroup[row.GroupID] = row.Total
	}
	for index := range result {
		result[index].Count = byGroup[result[index].ID]
	}
	return result, nil
}

func (service *LibraryService) CreateFavoriteGroup(ctx context.Context, name string) (FavoriteGroupItem, error) {
	name, err := normalizeFavoriteName(name)
	if err != nil {
		return FavoriteGroupItem{}, err
	}
	record, err := service.database.FavoriteGroup.Create().SetName(name).Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return FavoriteGroupItem{}, ErrFavoriteGroupUsed
		}
		return FavoriteGroupItem{}, fmt.Errorf("create favorite group: %w", err)
	}
	return FavoriteGroupItem{ID: record.ID, Name: record.Name}, nil
}

func (service *LibraryService) RenameFavoriteGroup(ctx context.Context, id int, name string) (FavoriteGroupItem, error) {
	name, err := normalizeFavoriteName(name)
	if err != nil {
		return FavoriteGroupItem{}, err
	}
	record, err := service.database.FavoriteGroup.UpdateOneID(id).SetName(name).Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return FavoriteGroupItem{}, ErrFavoriteGroupGone
		}
		if ent.IsConstraintError(err) {
			return FavoriteGroupItem{}, ErrFavoriteGroupUsed
		}
		return FavoriteGroupItem{}, fmt.Errorf("rename favorite group: %w", err)
	}
	return FavoriteGroupItem{ID: record.ID, Name: record.Name}, nil
}

// DeleteFavoriteGroup removes the group and, by the cascade edge, its stars.
// The movies themselves are untouched.
func (service *LibraryService) DeleteFavoriteGroup(ctx context.Context, id int) error {
	if err := service.database.FavoriteGroup.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return ErrFavoriteGroupGone
		}
		return fmt.Errorf("delete favorite group: %w", err)
	}
	return nil
}

// favoriteRunes counts user-perceived characters so a Chinese label is not
// limited to eight characters the way len() would.
func normalizeFavoriteName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrFavoriteGroupName
	}
	if len([]rune(name)) > favoriteNameLimit {
		return "", domain.E(domain.KindInvalid, fmt.Sprintf("分组名称不能超过 %d 个字", favoriteNameLimit), nil)
	}
	return name, nil
}

// favoriteMovieIDs returns the group ids each movie belongs to, keyed by
// movie id. One query per page keeps the list endpoint independent of N+1.
func (service *LibraryService) favoriteMovieIDs(ctx context.Context, movieIDs []int) (map[int][]int, error) {
	result := make(map[int][]int, len(movieIDs))
	if len(movieIDs) == 0 {
		return result, nil
	}
	rows, err := service.database.Favorite.Query().
		Where(favorite.MovieIDIn(movieIDs...)).
		Order(ent.Asc(favorite.FieldGroupID)).
		Select(favorite.FieldMovieID, favorite.FieldGroupID).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query favorites: %w", err)
	}
	for _, row := range rows {
		result[row.MovieID] = append(result[row.MovieID], row.GroupID)
	}
	return result, nil
}

// valueOrEmpty keeps the JSON shape stable: cards always receive an array.
func valueOrEmpty(ids []int) []int {
	if ids == nil {
		return []int{}
	}
	return ids
}
