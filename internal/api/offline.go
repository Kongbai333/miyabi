package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/service"
)

type OfflineManager interface {
	Add(context.Context, string, string) (service.OfflineSubmission, error)
	Tasks(context.Context, string, string) ([]service.OfflineSubmission, error)
	Activity(context.Context) (service.OfflineActivity, error)
	AddMagnet(context.Context, string) (service.OfflineSubmission, error)
}

func offlineActivityHandler(offline OfflineManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		activity, err := offline.Activity(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, activity)
	}
}

type offlineInput struct {
	Hash string `json:"hash" binding:"required,len=40,hexadecimal"`
}

// A magnet the library does not know accepts either form, so this takes the
// link the magnet page copied rather than a bare hash.
type offlineMagnetInput struct {
	Magnet string `json:"magnet" binding:"required"`
}

type offlineTasksQuery struct {
	AccountID string `form:"account_id" binding:"required,number"`
}

func offlineTasksHandler(offline OfflineManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri movieURI
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		var query offlineTasksQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			c.Error(BadRequest(err))
			return
		}
		tasks, err := offline.Tasks(c.Request.Context(), uri.ID, query.AccountID)
		if err != nil {
			c.Error(err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, tasks)
	}
}

func offlineAddHandler(offline OfflineManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri movieURI
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		var input offlineInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Error(BadRequest(err))
			return
		}
		submission, err := offline.Add(c.Request.Context(), uri.ID, input.Hash)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusAccepted, submission)
	}
}

func offlineAddMagnetHandler(offline OfflineManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input offlineMagnetInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Error(BadRequest(err))
			return
		}
		submission, err := offline.AddMagnet(c.Request.Context(), input.Magnet)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusAccepted, submission)
	}
}
