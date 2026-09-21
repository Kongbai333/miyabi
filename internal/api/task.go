package api

import (
	"context"
	"github.com/ppxb/miyabi/internal/tasks"
	"net/http"

	"github.com/gin-gonic/gin"
)

type TaskManager interface {
	Revisions() tasks.TaskRevisions
	List(context.Context) ([]tasks.TaskInfo, error)
	Subscribe() (<-chan struct{}, func())
}

func tasksHandler(tasks TaskManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := tasks.List(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, result)
	}
}
