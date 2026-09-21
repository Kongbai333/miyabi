package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/service"
)

type TaskManager interface {
	Revisions() service.TaskRevisions
	List(context.Context) ([]service.TaskInfo, error)
	Queues(context.Context) ([]service.TaskQueue, error)
	Pause(context.Context, []string) error
	Resume(context.Context, []string) error
	CancelQueued(context.Context, []string) (int, error)
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

type taskTypesInput struct {
	Types []string `json:"types" binding:"required,min=1"`
}

// A queue is what the pools are holding right now, so the page can draw the
// lanes before the stream delivers its first event.
func taskQueuesHandler(tasks TaskManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		queues, err := tasks.Queues(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, queues)
	}
}

// The direction is wrapped rather than passed as a method value, so registering
// the route never touches a dependency that a partial router leaves unset.
func taskPauseHandler(tasks TaskManager) gin.HandlerFunc {
	return taskToggleHandler(tasks, func(ctx context.Context, types []string) error {
		return tasks.Pause(ctx, types)
	})
}

func taskResumeHandler(tasks TaskManager) gin.HandlerFunc {
	return taskToggleHandler(tasks, func(ctx context.Context, types []string) error {
		return tasks.Resume(ctx, types)
	})
}

// Pausing and resuming answer with the resulting queues, so the page that asked
// redraws from the response instead of waiting for the next event.
func taskToggleHandler(tasks TaskManager, toggle func(context.Context, []string) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input taskTypesInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Error(BadRequest(err))
			return
		}
		if err := toggle(c.Request.Context(), input.Types); err != nil {
			c.Error(err)
			return
		}
		queues, err := tasks.Queues(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, queues)
	}
}

func taskCancelQueuedHandler(tasks TaskManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		removed, err := tasks.CancelQueued(c.Request.Context(), c.QueryArray("type"))
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"cancelled": removed})
	}
}
