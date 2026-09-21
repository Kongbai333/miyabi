package service

import (
	"context"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent/task"
)

func taskControlFixture(t testing.TB) (*TaskService, context.Context) {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return NewTaskService(store.Client), t.Context()
}

func queueTask(t testing.TB, service *TaskService, kind string, status task.Status) {
	t.Helper()
	if err := service.database.Task.Create().SetType(kind).SetStatus(status).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func taskQueue(t *testing.T, queues []TaskQueue, kind string) TaskQueue {
	t.Helper()
	for _, queue := range queues {
		if queue.Type == kind {
			return queue
		}
	}
	t.Fatalf("%s is missing from %+v", kind, queues)
	return TaskQueue{}
}

// Only waiting and running work is a queue. A finished task belongs to the
// history a workflow row reports, not to what the pools are holding.
func TestQueuesReportWhatEachPoolIsHolding(t *testing.T) {
	service, ctx := taskControlFixture(t)
	queueTask(t, service, "scan", task.StatusQueued)
	queueTask(t, service, "scrape", task.StatusQueued)
	queueTask(t, service, "scrape", task.StatusQueued)
	queueTask(t, service, "cover", task.StatusRunning)
	queueTask(t, service, "frame", task.StatusQueued)
	queueTask(t, service, "frame", task.StatusQueued)
	queueTask(t, service, "frame", task.StatusRunning)
	queueTask(t, service, "scan", task.StatusDone)
	queueTask(t, service, "cover", task.StatusFailed)

	queues, err := service.Queues(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(queues) != len(TaskTypes) {
		t.Fatalf("queues = %+v", queues)
	}
	for index, kind := range TaskTypes {
		if queues[index].Type != kind {
			t.Fatalf("queues are not in a fixed order: %+v", queues)
		}
	}
	if got := taskQueue(t, queues, "scan"); got.Queued != 1 || got.Running != 0 || got.Paused {
		t.Fatalf("scan queue = %+v", got)
	}
	if got := taskQueue(t, queues, "scrape"); got.Queued != 2 {
		t.Fatalf("scrape queue = %+v", got)
	}
	if got := taskQueue(t, queues, "cover"); got.Queued != 0 || got.Running != 1 {
		t.Fatalf("cover queue = %+v", got)
	}
	if got := taskQueue(t, queues, "frame"); got.Queued != 2 || got.Running != 1 {
		t.Fatalf("frame queue = %+v", got)
	}

	// A kind nobody queued still has a lane, so the page does not have to guess.
	if _, err := service.database.Task.Delete().Exec(ctx); err != nil {
		t.Fatal(err)
	}
	queues, err = service.Queues(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(queues) != len(TaskTypes) || taskQueue(t, queues, "frame").Queued != 0 {
		t.Fatalf("empty queues = %+v", queues)
	}
}

func TestPauseKeepsTheQueuedWorkOfThatKindWaiting(t *testing.T) {
	service, ctx := taskControlFixture(t)
	queueTask(t, service, "scan", task.StatusQueued)
	queueTask(t, service, "frame", task.StatusQueued)
	if err := service.Pause(ctx, []string{"frame"}); err != nil {
		t.Fatal(err)
	}

	queues, err := service.Queues(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !taskQueue(t, queues, "frame").Paused || taskQueue(t, queues, "scan").Paused {
		t.Fatalf("paused = %+v", queues)
	}

	// The paused kind is skipped while every other pool keeps working.
	job, err := service.Claim(ctx, TaskTypes)
	if err != nil || job == nil || job.Type != "scan" {
		t.Fatalf("claim while paused = %+v err=%v", job, err)
	}
	if job, err = service.Claim(ctx, TaskTypes); err != nil || job != nil {
		t.Fatalf("paused work was claimed: %+v err=%v", job, err)
	}
	if waiting, err := service.database.Task.Query().
		Where(task.TypeEQ("frame"), task.StatusEQ(task.StatusQueued)).Count(ctx); err != nil || waiting != 1 {
		t.Fatalf("queued stills = %d err=%v", waiting, err)
	}

	if err := service.Resume(ctx, []string{"frame"}); err != nil {
		t.Fatal(err)
	}
	if job, err = service.Claim(ctx, TaskTypes); err != nil || job == nil || job.Type != "frame" {
		t.Fatalf("claim after resume = %+v err=%v", job, err)
	}
}

func TestCancelQueuedDropsOnlyWhatHasNotStarted(t *testing.T) {
	service, ctx := taskControlFixture(t)
	queueTask(t, service, "frame", task.StatusQueued)
	queueTask(t, service, "frame", task.StatusQueued)
	queueTask(t, service, "frame", task.StatusRunning)
	queueTask(t, service, "scan", task.StatusQueued)

	removed, err := service.CancelQueued(ctx, []string{"frame"})
	if err != nil || removed != 2 {
		t.Fatalf("removed = %d err=%v", removed, err)
	}
	remaining, err := service.database.Task.Query().All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 2 {
		t.Fatalf("remaining tasks = %+v", remaining)
	}
	for _, record := range remaining {
		if record.Type == "frame" && record.Status != task.StatusRunning {
			t.Fatalf("cancelled tasks are still queued: %+v", record)
		}
	}
	if removed, err = service.CancelQueued(ctx, []string{"cover"}); err != nil || removed != 0 {
		t.Fatalf("removed = %d err=%v", removed, err)
	}
}

// A typo must not look like a pause that worked.
func TestTaskControlRefusesUnknownKinds(t *testing.T) {
	service, ctx := taskControlFixture(t)
	for _, types := range [][]string{nil, {}, {"frame", "nope"}, {"SCAN"}} {
		if err := service.Pause(ctx, types); err == nil {
			t.Fatalf("pause accepted %q", types)
		}
		if err := service.Resume(ctx, types); err == nil {
			t.Fatalf("resume accepted %q", types)
		}
		if _, err := service.CancelQueued(ctx, types); err == nil {
			t.Fatalf("cancel accepted %q", types)
		}
	}
	if queues, err := service.Queues(ctx); err != nil || len(queues) != len(TaskTypes) {
		t.Fatalf("queues = %+v err=%v", queues, err)
	}
}

// Every pool waits on the notification channel it captured, so one notification
// has to release all of them. A signal that reaches a single waiter leaves the
// others asleep until some unrelated task happens to notify again.
func TestOneNotificationReleasesEveryWaitingPool(t *testing.T) {
	service, _ := taskControlFixture(t)
	pools := []<-chan struct{}{service.Pending(), service.Pending(), service.Pending()}
	service.Notify()
	for index, pending := range pools {
		select {
		case <-pending:
		case <-time.After(time.Second):
			t.Fatalf("pool %d was left waiting", index)
		}
	}
	// The replacement channel is fresh, so the next wait is a real wait.
	select {
	case <-service.Pending():
		t.Fatal("the replacement channel was already closed")
	default:
	}
}

// A restart must not quietly resume a queue the user stopped: a cover backfill
// reaches the whole library again, and the pools would start on it immediately.
func TestRestoreBringsBackThePausedKinds(t *testing.T) {
	service, ctx := taskControlFixture(t)
	if err := service.Pause(ctx, []string{"frame", "cover"}); err != nil {
		t.Fatal(err)
	}

	restarted := NewTaskService(service.database)
	if queues, err := restarted.Queues(ctx); err != nil || taskQueue(t, queues, "frame").Paused {
		t.Fatalf("queues before restore = %+v err=%v", queues, err)
	}
	if err := restarted.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	queues, err := restarted.Queues(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !taskQueue(t, queues, "frame").Paused || !taskQueue(t, queues, "cover").Paused {
		t.Fatalf("restored queues = %+v", queues)
	}
	if taskQueue(t, queues, "scrape").Paused {
		t.Fatalf("restore paused a kind nobody stopped: %+v", queues)
	}

	// Resuming is just as durable as pausing.
	if err := restarted.Resume(ctx, []string{"frame"}); err != nil {
		t.Fatal(err)
	}
	again := NewTaskService(service.database)
	if err := again.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	if queues, err := again.Queues(ctx); err != nil || taskQueue(t, queues, "frame").Paused {
		t.Fatalf("queues after resuming = %+v err=%v", queues, err)
	}
}

// A value written by another build may name work this one does not run, which
// must not stop the rest of it from coming back.
func TestRestoreIgnoresKindsThisBuildDoesNotRun(t *testing.T) {
	service, ctx := taskControlFixture(t)
	if err := saveSetting(ctx, service.database, taskPausedSetting, []string{"frame", "reindex"}); err != nil {
		t.Fatal(err)
	}
	if err := service.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	queues, err := service.Queues(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !taskQueue(t, queues, "frame").Paused {
		t.Fatalf("restored queues = %+v", queues)
	}
}
