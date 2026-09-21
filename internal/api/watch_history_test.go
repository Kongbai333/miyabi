package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/service"
)

type watchHistoryStub struct {
	LibraryManager
	called   string
	ids      []int
	scope    service.WatchHistoryScope
	progress service.WatchProgress
	err      error
}

func (stub *watchHistoryStub) SaveWatchProgress(_ context.Context, id int, progress service.WatchProgress) error {
	stub.called, stub.ids, stub.progress = "progress", []int{id}, progress
	return stub.err
}

func (stub *watchHistoryStub) ClearWatchHistory(_ context.Context, scope service.WatchHistoryScope) (int, error) {
	stub.called, stub.scope = "clear", scope
	return 2, stub.err
}

func TestHistoryEndpointsValidateScopeProgress(t *testing.T) {
	progress := `{"session_id":"bf4f3ab4-0535-4aee-a38c-ebdf9462c6a8","file_id":"video","position":120.5,"duration":600,"version":2}`
	for _, scenario := range []struct {
		name, method, path, body, called string
		status                           int
		err                              error
	}{
		{name: "clear", method: "DELETE", path: "/api/library/history?account_id=100&directory_id=10", called: "clear", status: 200},
		{name: "clear without source", method: "DELETE", path: "/api/library/history", status: 400},
		{name: "clear with a partial source", method: "DELETE", path: "/api/library/history?account_id=100", status: 400},
		{name: "source changed", method: "DELETE", path: "/api/library/history?account_id=100&directory_id=10", called: "clear", err: service.ErrWatchHistorySourceChanged, status: 409},
		{name: "progress", method: "PUT", path: "/api/library/history/7/progress", body: progress, called: "progress", status: 200},
		{name: "invalid history id", method: "PUT", path: "/api/library/history/0/progress", body: progress, status: 400},
		{name: "missing session", method: "PUT", path: "/api/library/history/7/progress", body: `{"file_id":"video","duration":600,"version":1}`, status: 400},
		{name: "negative progress", method: "PUT", path: "/api/library/history/7/progress", body: strings.Replace(progress, "120.5", "-1", 1), status: 400},
		{name: "zero duration", method: "PUT", path: "/api/library/history/7/progress", body: strings.Replace(progress, "600", "0", 1), status: 400},
		{name: "write failed", method: "PUT", path: "/api/library/history/7/progress", body: progress, called: "progress", err: errors.New("write failed"), status: 500},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			stub := &watchHistoryStub{err: scenario.err}
			router := NewRouter(Dependencies{Library: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			request := httptest.NewRequest(scenario.method, scenario.path, strings.NewReader(scenario.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != scenario.status || stub.called != scenario.called {
				t.Fatalf("status=%d call=%s body=%s", response.Code, stub.called, response.Body)
			}
			if scenario.name == "progress" && (stub.ids[0] != 7 || stub.progress.Position != 120.5 || stub.progress.Version != 2) {
				t.Fatal("progress payload was changed")
			}
			if stub.called == "clear" && (stub.scope.AccountID != "100" || stub.scope.DirectoryID != "10") {
				t.Fatal("clear operation lost its expected source")
			}
		})
	}
}
