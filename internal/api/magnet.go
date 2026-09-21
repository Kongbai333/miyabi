package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/service"
)

type MagnetManager interface {
	Settings(context.Context) (service.MCPSettings, error)
	UpdateSettings(context.Context, service.MCPSettings) (service.MCPSettings, error)
	Test(context.Context) (service.MCPTestResult, error)
	MagnetSearch(context.Context, string, int) (service.MagnetSearchResult, error)
	MagnetPreview(context.Context, string) (service.MagnetPreview, error)
	MagnetFiles(context.Context, string) (service.MagnetFiles, error)
	Collections(context.Context) ([]service.MagnetCollection, error)
	Collection(context.Context, string) (service.MagnetCollectionDetail, error)
	CreateCollection(context.Context, string) error
	RenameCollection(context.Context, string, string) error
	DeleteCollection(context.Context, string) error
	EnableShare(context.Context, string) (service.MagnetShare, error)
	DisableShare(context.Context, string) error
	ShareDetail(context.Context, string) (service.MagnetShareDetail, error)
	ImportShare(context.Context, string, string) error
}

// magnetSettingsInput keeps the URL optional so a blank field keeps the
// default server instead of clearing it.
type magnetSettingsInput struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

type magnetSearchInput struct {
	Query string `json:"query" binding:"required"`
	Limit int    `json:"limit" binding:"omitempty,min=1,max=20"`
}

type magnetLinkInput struct {
	MagnetLink string `json:"magnet_link" binding:"required"`
}

// The collection key travels in the path, so a body only ever carries a label.
type magnetLabelInput struct {
	Label string `json:"label" binding:"required"`
}

type magnetShareImportInput struct {
	Code  string `json:"code" binding:"required"`
	Label string `json:"label"`
}

type magnetKeyURI struct {
	Key string `uri:"key" binding:"required"`
}

type magnetCodeURI struct {
	Code string `uri:"code" binding:"required"`
}

func magnetSettingsHandler(magnet MagnetManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings, err := magnet.Settings(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, settings)
	}
}

func magnetSettingsUpdateHandler(magnet MagnetManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input magnetSettingsInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Error(BadRequest(err))
			return
		}
		settings, err := magnet.UpdateSettings(c.Request.Context(), service.MCPSettings{URL: input.URL, Token: input.Token})
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, settings)
	}
}

func magnetTestHandler(magnet MagnetManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := magnet.Test(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func magnetSearchHandler(magnet MagnetManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input magnetSearchInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Error(BadRequest(err))
			return
		}
		result, err := magnet.MagnetSearch(c.Request.Context(), input.Query, input.Limit)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func magnetPreviewHandler(magnet MagnetManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input magnetLinkInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Error(BadRequest(err))
			return
		}
		result, err := magnet.MagnetPreview(c.Request.Context(), input.MagnetLink)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func magnetFilesHandler(magnet MagnetManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input magnetLinkInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Error(BadRequest(err))
			return
		}
		result, err := magnet.MagnetFiles(c.Request.Context(), input.MagnetLink)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func magnetCollectionsHandler(magnet MagnetManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		collections, err := magnet.Collections(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, collections)
	}
}

func magnetCollectionHandler(magnet MagnetManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri magnetKeyURI
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		detail, err := magnet.Collection(c.Request.Context(), uri.Key)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, detail)
	}
}

func magnetCollectionCreateHandler(magnet MagnetManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input magnetLabelInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Error(BadRequest(err))
			return
		}
		if err := magnet.CreateCollection(c.Request.Context(), input.Label); err != nil {
			c.Error(err)
			return
		}
		collections, err := magnet.Collections(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusCreated, collections)
	}
}

func magnetCollectionUpdateHandler(magnet MagnetManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri magnetKeyURI
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		var input magnetLabelInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Error(BadRequest(err))
			return
		}
		if err := magnet.RenameCollection(c.Request.Context(), uri.Key, input.Label); err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"renamed": true})
	}
}

func magnetCollectionDeleteHandler(magnet MagnetManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri magnetKeyURI
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		if err := magnet.DeleteCollection(c.Request.Context(), uri.Key); err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"removed": true})
	}
}

func magnetShareEnableHandler(magnet MagnetManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri magnetKeyURI
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		share, err := magnet.EnableShare(c.Request.Context(), uri.Key)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, share)
	}
}

func magnetShareDisableHandler(magnet MagnetManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri magnetKeyURI
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		if err := magnet.DisableShare(c.Request.Context(), uri.Key); err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"shared": false})
	}
}

func magnetShareDetailHandler(magnet MagnetManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri magnetCodeURI
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		detail, err := magnet.ShareDetail(c.Request.Context(), uri.Code)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, detail)
	}
}

func magnetShareImportHandler(magnet MagnetManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input magnetShareImportInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Error(BadRequest(err))
			return
		}
		if err := magnet.ImportShare(c.Request.Context(), input.Code, input.Label); err != nil {
			c.Error(err)
			return
		}
		collections, err := magnet.Collections(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusCreated, collections)
	}
}
