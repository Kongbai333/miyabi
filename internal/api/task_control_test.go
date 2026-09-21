package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/service"
)

type taskControlStub struct {
	paused    []string
	resumed   []string
	cancelled []string
	removed   int
	failure   error
}

func (stub *taskControlStub) Revisions() service.TaskRevisions { return service.TaskRevisions{} }

func (stub *taskControlStub) List(context.Context) ([]service.TaskInfo, error) { return nil, nil }

func (stub *taskControlStub) Queues(context.Context) ([]service.TaskQueue, error) {
	if stub.failure != nil {
		return nil, stub.failure
	}
	return []service.TaskQueue{
		{Type: "scan", Running: 1},
		{Type: "frame", Queued: 4, Paused: len(stub.paused) > 0},
	}, nil
}

func (stub *taskControlStub) Subscribe() (<-chan struct{}, func()) {
	return make(chan struct{}), func() {}
}

func (stub *taskControlStub) Pause(_ context.Context, types []string) error {
	if stub.failure != nil {
		return stub.failure
	}
	stub.paused = append(stub.paused, types...)
	return nil
}

func (stub *taskControlStub) Resume(_ context.Context, types []string) error {
	if stub.failure != nil {
		return stub.failure
	}
	stub.resumed = append(stub.resumed, types...)
	return nil
}

func (stub *taskControlStub) CancelQueued(_ context.Context, types []string) (int, error) {
	if stub.failure != nil {
		return 0, stub.failure
	}
	stub.cancelled = append(stub.cancelled, types...)
	return stub.removed, nil
}

func taskControlRequest(t *testing.T, stub *taskControlStub, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := NewRouter(Dependencies{Tasks: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

// The page redraws from the response, so a toggle has to answer with the queues
// it produced rather than an empty acknowledgement.
func TestTaskPauseAndResumeAnswerWithTheQueues(t *testing.T) {
	stub := &taskControlStub{}
	response := taskControlRequest(t, stub, http.MethodPost, "/api/tasks/pause", `{"types":["frame","cover"]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("pause = %d %s", response.Code, response.Body)
	}
	var queues []service.TaskQueue
	if err := json.Unmarshal(response.Body.Bytes(), &queues); err != nil || len(queues) != 2 || !queues[1].Paused {
		t.Fatalf("pause payload = %s (%v)", response.Body, err)
	}
	if strings.Join(stub.paused, ",") != "frame,cover" || len(stub.resumed) != 0 {
		t.Fatalf("pause passed %q", stub.paused)
	}

	stub.paused = nil
	response = taskControlRequest(t, stub, http.MethodPost, "/api/tasks/resume", `{"types":["frame"]}`)
	if response.Code != http.StatusOK || strings.Join(stub.resumed, ",") != "frame" {
		t.Fatalf("resume = %d %s resumed=%q", response.Code, response.Body, stub.resumed)
	}
}

func TestTaskControlRejectsARequestWithoutTypes(t *testing.T) {
	stub := &taskControlStub{}
	for _, body := range []string{`{}`, `{"types":[]}`, `not json`} {
		response := taskControlRequest(t, stub, http.MethodPost, "/api/tasks/pause", body)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s", body, response.Code, response.Body)
		}
	}
	if len(stub.paused) != 0 {
		t.Fatalf("an empty request reached the service: %q", stub.paused)
	}
}

// The kinds themselves are the service's to validate, so the route only has to
// pass through every one it was given.
func TestTaskCancelQueuedPassesEveryTypeThrough(t *testing.T) {
	stub := &taskControlStub{removed: 5}
	response := taskControlRequest(t, stub, http.MethodDelete, "/api/tasks/queued?type=frame&type=cover", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"cancelled":5`) {
		t.Fatalf("cancel = %d %s", response.Code, response.Body)
	}
	if strings.Join(stub.cancelled, ",") != "frame,cover" {
		t.Fatalf("cancel passed %q", stub.cancelled)
	}
}

func TestTaskControlReportsAServiceFailure(t *testing.T) {
	stub := &taskControlStub{failure: errors.New("database locked")}
	response := taskControlRequest(t, stub, http.MethodPost, "/api/tasks/pause", `{"types":["frame"]}`)
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "database locked") {
		t.Fatalf("pause = %d %s", response.Code, response.Body)
	}
}
