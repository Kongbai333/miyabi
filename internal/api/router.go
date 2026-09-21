package api

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	sloggin "github.com/samber/slog-gin"
)

type HealthChecker interface {
	Ping(context.Context) error
}

type Dependencies struct {
	Logger   *slog.Logger
	Health   HealthChecker
	Access   AccessGate
	Discover Discoverer
	Pan      PanManager
	Offline  OfflineManager
	Monitor  MonitorManager
	Library  LibraryManager
	Play     PlayManager
	Tasks    TaskManager
	Artwork  ArtworkReader
	Data     DataManager
	Network  NetworkManager
	Magnet   MagnetManager
	Frontend fs.FS
}

func NewRouter(deps Dependencies) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(
		sloggin.NewWithFilters(deps.Logger, sloggin.IgnoreStatus(statusClientClosedRequest)),
		recoveryMiddleware(deps.Logger),
		errorMiddleware(deps.Logger),
	)

	// A nil gate disables access control; production always wires one in.
	api := router.Group("/api", requireAccess(deps.Access))
	api.GET("/health", healthHandler(deps.Health))
	authAPI := api.Group("/auth", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	authAPI.GET("/config", accessConfigHandler(deps.Access))
	authAPI.GET("/session", accessSessionHandler(deps.Access))
	authAPI.POST("/login", accessLoginHandler(deps.Access))
	authAPI.POST("/logout", accessLogoutHandler(deps.Access))
	settingsAPI := api.Group("/settings", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	settingsAPI.GET("/system", dataInfoHandler(deps.Data))
	settingsAPI.DELETE("/cache", dataClearCacheHandler(deps.Data))
	settingsAPI.GET("/network", networkHandler(deps.Network))
	settingsAPI.PUT("/network", networkUpdateHandler(deps.Network))
	settingsAPI.POST("/network/test", networkTestHandler(deps.Network))
	api.GET("/library/movies", libraryMoviesHandler(deps.Library))
	api.GET("/library/filter-options", libraryFilterOptionsHandler(deps.Library))
	api.PUT("/library/movies/:id/watched", libraryWatchedHandler(deps.Library))
	api.GET("/library/favorite-groups", libraryFavoriteGroupsHandler(deps.Library))
	api.POST("/library/favorite-groups", libraryFavoriteGroupCreateHandler(deps.Library))
	api.PUT("/library/favorite-groups/:id", libraryFavoriteGroupUpdateHandler(deps.Library))
	api.DELETE("/library/favorite-groups/:id", libraryFavoriteGroupDeleteHandler(deps.Library))
	api.PUT("/library/movies/:id/favorite", libraryFavoriteHandler(deps.Library))
	api.DELETE("/library/history", libraryHistoryClearHandler(deps.Library))
	api.PUT("/library/history/:id/progress", libraryHistoryProgressHandler(deps.Library))
	api.POST("/library/scan", libraryScanHandler(deps.Library))
	api.POST("/library/covers/backfill", libraryCoverBackfillHandler(deps.Library))
	api.GET("/library/artwork/:key", libraryArtworkHandler(deps.Artwork))
	playAPI := api.Group("/play", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	playAPI.GET("/files", playFilesHandler(deps.Play))
	playAPI.GET("/:id", playStartHandler(deps.Play))
	playAPI.DELETE("/:id", playReleaseHandler(deps.Play))
	playAPI.GET("/:id/stream/:resource", playStreamHandler(deps.Play))
	playAPI.HEAD("/:id/stream/:resource", playStreamHandler(deps.Play))
	api.GET("/tasks", tasksHandler(deps.Tasks))
	api.GET("/tasks/queues", taskQueuesHandler(deps.Tasks))
	api.GET("/tasks/events", taskEventsHandler(deps.Tasks))
	api.POST("/tasks/pause", taskPauseHandler(deps.Tasks))
	api.POST("/tasks/resume", taskResumeHandler(deps.Tasks))
	api.DELETE("/tasks/queued", taskCancelQueuedHandler(deps.Tasks))
	api.GET("/offline/tasks", offlineActivityHandler(deps.Offline))
	api.POST("/offline/magnets", offlineAddMagnetHandler(deps.Offline))
	monitorAPI := api.Group("/monitors", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	monitorAPI.GET("", monitorListHandler(deps.Monitor))
	monitorAPI.POST("", monitorAddHandler(deps.Monitor))
	monitorAPI.DELETE("/:id", monitorRemoveHandler(deps.Monitor))
	monitorAPI.POST("/:id/retry", monitorRetryHandler(deps.Monitor))
	api.GET("/discover/movies", discoverBrowseHandler(deps.Discover))
	api.PUT("/discover/movies/:id/viewed", discoverMarkViewedHandler(deps.Discover))
	api.POST("/discover/movie-states", discoverMovieStatesHandler(deps.Discover))
	api.GET("/discover/search", discoverSearchHandler(deps.Discover))
	api.GET("/discover/tags", discoverTagsHandler(deps.Discover))
	api.GET("/discover/movies/:id", discoverMovieHandler(deps.Discover))
	api.GET("/discover/movies/:id/magnets", discoverMagnetsHandler(deps.Discover))
	api.POST("/discover/movies/:id/offline", offlineAddHandler(deps.Offline))
	api.GET("/discover/movies/:id/offline", offlineTasksHandler(deps.Offline))
	api.GET("/image", imageHandler(deps.Discover))
	api.GET("/javdb/route", javdbRouteHandler(deps.Discover))
	api.PUT("/javdb/route", javdbSelectRouteHandler(deps.Discover))
	api.POST("/javdb/reselect", javdbReselectHandler(deps.Discover))
	panAPI := api.Group("/pan", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	panAPI.GET("/account", panAccountHandler(deps.Pan))
	panAPI.DELETE("/account", panDisconnectHandler(deps.Pan))
	panAPI.POST("/login", panBeginLoginHandler(deps.Pan))
	panAPI.GET("/login/:id", panLoginStatusHandler(deps.Pan))
	panAPI.GET("/files", panFilesHandler(deps.Pan))
	panAPI.PUT("/directory", panSelectDirectoryHandler(deps.Pan))
	panAPI.DELETE("/directory", panClearDirectoryHandler(deps.Pan))
	// The settings carry the magnet server token, so nothing here is cacheable.
	magnetAPI := api.Group("/magnet", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	magnetAPI.GET("/settings", magnetSettingsHandler(deps.Magnet))
	magnetAPI.PUT("/settings", magnetSettingsUpdateHandler(deps.Magnet))
	magnetAPI.POST("/test", magnetTestHandler(deps.Magnet))
	magnetAPI.POST("/search", magnetSearchHandler(deps.Magnet))
	magnetAPI.POST("/preview", magnetPreviewHandler(deps.Magnet))
	magnetAPI.POST("/files", magnetFilesHandler(deps.Magnet))
	magnetAPI.GET("/collections", magnetCollectionsHandler(deps.Magnet))
	magnetAPI.POST("/collections", magnetCollectionCreateHandler(deps.Magnet))
	magnetAPI.GET("/collections/:key", magnetCollectionHandler(deps.Magnet))
	magnetAPI.PUT("/collections/:key", magnetCollectionUpdateHandler(deps.Magnet))
	magnetAPI.DELETE("/collections/:key", magnetCollectionDeleteHandler(deps.Magnet))
	magnetAPI.POST("/collections/:key/share", magnetShareEnableHandler(deps.Magnet))
	magnetAPI.DELETE("/collections/:key/share", magnetShareDisableHandler(deps.Magnet))
	magnetAPI.GET("/shares/:code", magnetShareDetailHandler(deps.Magnet))
	magnetAPI.POST("/shares/import", magnetShareImportHandler(deps.Magnet))

	if deps.Frontend != nil {
		installFrontend(router, deps.Frontend)
	}
	return router
}

func installFrontend(router *gin.Engine, frontend fs.FS) {
	fileServer := http.FileServer(http.FS(frontend))
	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		requestedPath := strings.TrimPrefix(c.Request.URL.Path, "/")
		if requestedPath != "" {
			if _, err := fs.Stat(frontend, requestedPath); err == nil {
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
		}

		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}
