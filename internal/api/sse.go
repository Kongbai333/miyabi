package api

import (
	"context"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/service"
)

// A stream event carries both views of the same instant: the workflows the
// library page follows and the queues the task centre controls.
func taskEventsPayload(ctx context.Context, tasks TaskManager) ([]service.TaskInfo, []service.TaskQueue, error) {
	snapshot, err := tasks.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	queues, err := tasks.Queues(ctx)
	if err != nil {
		return nil, nil, err
	}
	return snapshot, queues, nil
}

func taskEventsHandler(tasks TaskManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Subscribe before reading to avoid losing an update between the initial
		// snapshot and stream setup. Reconnection always starts with a snapshot.
		updates, unsubscribe := tasks.Subscribe()
		defer unsubscribe()
		snapshot, queues, err := taskEventsPayload(c.Request.Context(), tasks)
		if err != nil {
			c.Error(err)
			return
		}
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache, no-transform")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		writeTaskEvents(c, snapshot, queues, tasks)
		c.Writer.Flush()
		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()
		c.Stream(func(io.Writer) bool {
			select {
			case <-c.Request.Context().Done():
				return false
			case <-heartbeat.C:
				c.SSEvent("ping", nil)
			case <-updates:
				snapshot, queues, err := taskEventsPayload(c.Request.Context(), tasks)
				if err != nil {
					c.Error(err)
					return false
				}
				writeTaskEvents(c, snapshot, queues, tasks)
			}
			return true
		})
	}
}

func writeTaskEvents(c *gin.Context, snapshot []service.TaskInfo, queues []service.TaskQueue, tasks TaskManager) {
	c.SSEvent("tasks", snapshot)
	c.SSEvent("queues", queues)
	c.SSEvent("changes", tasks.Revisions())
}
