package service

import (
	"context"
	"encoding/json"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
)

type TaskService = tasks.Service
type TaskInfo = tasks.TaskInfo
type TaskRevisions = tasks.TaskRevisions
type TaskJob = tasks.Job

func encodeTaskPayload(value any) (json.RawMessage, error) {
	return tasks.EncodePayload(value)
}

func decodeTaskPayload[T any](payload json.RawMessage) (T, error) {
	return tasks.DecodePayload[T](payload)
}

func setTaskPayloadField(payload json.RawMessage, key string, value any) (json.RawMessage, error) {
	return tasks.SetPayloadField(payload, key, value)
}

func ensureScanTask(ctx context.Context, client *ent.TaskClient, source domain.LibrarySource, reusable ...task.Status) (*ent.Task, error) {
	return tasks.EnsureScanTask(ctx, client, source, reusable...)
}

func scanTaskInfo(record *ent.Task) (tasks.TaskInfo, error) {
	return tasks.ScanTaskInfo(record)
}

func NewTaskService(database *ent.Client, registry ...*tasks.Registry) *tasks.Service {
	var r *tasks.Registry
	if len(registry) > 0 {
		r = registry[0]
	} else {
		r = tasks.NewRegistry()
	}
	return tasks.NewService(database, r)
}
