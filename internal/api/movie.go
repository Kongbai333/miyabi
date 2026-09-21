package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/service"
)

type LibraryManager interface {
	Movies(context.Context, int, int, service.LibraryFilter) (service.LibraryPage, error)
	FilterOptions(context.Context) (service.LibraryFilterOptions, error)
	SetFavorite(context.Context, int, []int) error
	FavoriteGroups(context.Context) ([]service.FavoriteGroupItem, error)
	CreateFavoriteGroup(context.Context, string) (service.FavoriteGroupItem, error)
	RenameFavoriteGroup(context.Context, int, string) (service.FavoriteGroupItem, error)
	DeleteFavoriteGroup(context.Context, int) error
	MarkWatched(context.Context, int, service.WatchHistoryScope) (service.WatchSession, error)
	WatchHistory(context.Context, int, int) (service.WatchHistoryPage, error)
	SaveWatchProgress(context.Context, int, service.WatchProgress) error
	RemoveWatchHistory(context.Context, service.WatchHistoryScope, []int) (int, error)
	ClearWatchHistory(context.Context, service.WatchHistoryScope) (int, error)
	StartScan(context.Context) (service.TaskInfo, error)
}

type ArtworkReader interface {
	Artwork(string) ([]byte, error)
}

func libraryArtworkHandler(artwork ArtworkReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			Key string `uri:"key" binding:"required,len=64,hexadecimal"`
		}
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		body, err := artwork.Artwork(uri.Key)
		if err != nil {
			c.Error(err)
			return
		}
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.Data(http.StatusOK, "image/jpeg", body)
	}
}

type libraryPageQuery struct {
	Page        int      `form:"page,default=1" binding:"min=1"`
	Limit       int      `form:"limit,default=20" binding:"min=1,max=100"`
	TagIDs      []int    `form:"tag_id" binding:"omitempty,dive,min=1"`
	ActorIDs    []string `form:"actor_id" binding:"omitempty,dive,min=1,max=64"`
	SeriesIDs   []string `form:"series_id" binding:"omitempty,dive,min=1,max=64"`
	MakerIDs    []string `form:"maker_id" binding:"omitempty,dive,min=1,max=64"`
	DirectorIDs []string `form:"director_id" binding:"omitempty,dive,min=1,max=64"`
	Years       []int    `form:"year" binding:"omitempty,dive,min=1900,max=2999"`
	GroupIDs    []int    `form:"group_id" binding:"omitempty,dive,min=1"`
	Watched     string   `form:"watched" binding:"omitempty,oneof=yes no"`
}

func (query libraryPageQuery) filter() service.LibraryFilter {
	return service.LibraryFilter{
		TagIDs: query.TagIDs, ActorIDs: query.ActorIDs, SeriesIDs: query.SeriesIDs,
		MakerIDs: query.MakerIDs, DirectorIDs: query.DirectorIDs, Years: query.Years,
		GroupIDs: query.GroupIDs, Watched: query.Watched,
	}
}

func libraryMoviesHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var query libraryPageQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			c.Error(BadRequest(err))
			return
		}
		movies, err := library.Movies(c.Request.Context(), query.Page, query.Limit, query.filter())
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, movies)
	}
}

func libraryFilterOptionsHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		options, err := library.FilterOptions(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, options)
	}
}

func libraryWatchedHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			ID int `uri:"id" binding:"required,min=1"`
		}
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		var scope service.WatchHistoryScope
		if err := c.ShouldBindJSON(&scope); err != nil {
			c.Error(BadRequest(err))
			return
		}
		history, err := library.MarkWatched(c.Request.Context(), uri.ID, scope)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": uri.ID, "watched": true, "history": history})
	}
}

type favoriteGroupInput struct {
	Name string `json:"name" binding:"required,max=200"`
}

type favoriteInput struct {
	GroupIDs []int `json:"group_ids" binding:"max=50,dive,min=1"`
}

func libraryFavoriteGroupsHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		groups, err := library.FavoriteGroups(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, groups)
	}
}

func libraryFavoriteGroupCreateHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input favoriteGroupInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Error(BadRequest(err))
			return
		}
		group, err := library.CreateFavoriteGroup(c.Request.Context(), input.Name)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusCreated, group)
	}
}

func libraryFavoriteGroupUpdateHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			ID int `uri:"id" binding:"required,min=1"`
		}
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		var input favoriteGroupInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Error(BadRequest(err))
			return
		}
		group, err := library.RenameFavoriteGroup(c.Request.Context(), uri.ID, input.Name)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, group)
	}
}

func libraryFavoriteGroupDeleteHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			ID int `uri:"id" binding:"required,min=1"`
		}
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		if err := library.DeleteFavoriteGroup(c.Request.Context(), uri.ID); err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, nil)
	}
}

func libraryFavoriteHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			ID int `uri:"id" binding:"required,min=1"`
		}
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		var input favoriteInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Error(BadRequest(err))
			return
		}
		if err := library.SetFavorite(c.Request.Context(), uri.ID, input.GroupIDs); err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": uri.ID, "group_ids": input.GroupIDs})
	}
}

func libraryScanHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		task, err := library.StartScan(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusAccepted, task)
	}
}
