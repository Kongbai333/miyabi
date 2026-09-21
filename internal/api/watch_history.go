package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/service"
)

func libraryHistoryProgressHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			ID int `uri:"id" binding:"min=1"`
		}
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		var progress service.WatchProgress
		if err := c.ShouldBindJSON(&progress); err != nil {
			c.Error(BadRequest(err))
			return
		}
		if err := library.SaveWatchProgress(c.Request.Context(), uri.ID, progress); err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, nil)
	}
}

func libraryHistoryClearHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var scope service.WatchHistoryScope
		if err := c.ShouldBindQuery(&scope); err != nil {
			c.Error(BadRequest(err))
			return
		}
		count, err := library.ClearWatchHistory(c.Request.Context(), scope)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"removed": count})
	}
}
