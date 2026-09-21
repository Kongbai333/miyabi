package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/service"
)

type libraryWatchStub struct {
	LibraryManager
	id    int
	err   error
	scope service.WatchHistoryScope
}

type libraryPageStub struct {
	LibraryManager
	page, limit int
	filter      service.LibraryFilter
}

func (stub *libraryPageStub) Movies(_ context.Context, page, limit int, filter service.LibraryFilter) (service.LibraryPage, error) {
	stub.page, stub.limit, stub.filter = page, limit, filter
	return service.LibraryPage{Page: page, Movies: []service.LibraryMovie{}}, nil
}

func TestLibraryMoviesDefaultToTwentyPerPage(t *testing.T) {
	for _, scenario := range []struct {
		query string
		page  int
	}{
		{query: "", page: 1},
		{query: "?page=2", page: 2},
	} {
		t.Run(scenario.query, func(t *testing.T) {
			stub := &libraryPageStub{}
			router := NewRouter(Dependencies{Library: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/library/movies"+scenario.query, nil))
			if response.Code != http.StatusOK || stub.page != scenario.page || stub.limit != 20 {
				t.Fatalf("library pagination: status=%d page=%d limit=%d body=%s", response.Code, stub.page, stub.limit, response.Body)
			}
		})
	}
}

func (stub *libraryWatchStub) MarkWatched(_ context.Context, id int, scope service.WatchHistoryScope) (service.WatchSession, error) {
	stub.id = id
	stub.scope = scope
	return service.WatchSession{ID: 7, SessionID: "session", FileID: "video", Position: 60, Duration: 600}, stub.err
}

func TestLibraryWatchedEndpointValidatesIDsAndReturnsSavedState(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		id     string
		err    error
		status int
		called bool
	}{
		{name: "saved", id: "42", status: http.StatusOK, called: true},
		{name: "zero", id: "0", status: http.StatusBadRequest},
		{name: "negative", id: "-1", status: http.StatusBadRequest},
		{name: "not numeric", id: "movie", status: http.StatusBadRequest},
		{name: "overflow", id: "99999999999999999999", status: http.StatusBadRequest},
		{name: "missing movie", id: "42", err: fs.ErrNotExist, status: http.StatusNotFound, called: true},
		{name: "unmounted", id: "42", err: service.ErrMediaDirectoryRequired, status: http.StatusBadRequest, called: true},
		{name: "write failed", id: "42", err: errors.New("write failed"), status: http.StatusInternalServerError, called: true},
		{name: "source changed", id: "42", err: service.ErrWatchHistorySourceChanged, status: http.StatusConflict, called: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			stub := &libraryWatchStub{err: scenario.err}
			router := NewRouter(Dependencies{Library: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/api/library/movies/"+scenario.id+"/watched",
				strings.NewReader(`{"account_id":"account","directory_id":"directory"}`)))
			if response.Code != scenario.status || (stub.id != 0) != scenario.called {
				t.Fatalf("status=%d called=%d body=%s", response.Code, stub.id, response.Body)
			}
			if scenario.called && (stub.scope.AccountID != "account" || stub.scope.DirectoryID != "directory") {
				t.Fatalf("watch source was not forwarded: %+v", stub.scope)
			}
			if scenario.status == http.StatusOK {
				var state struct {
					ID      int                  `json:"id"`
					Watched bool                 `json:"watched"`
					History service.WatchSession `json:"history"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil || state.ID != 42 || !state.Watched || state.History.Position != 60 {
					t.Fatalf("incorrect watch response: %s, %v", response.Body, err)
				}
			}
		})
	}
}

func TestLibraryWatchedEndpointRequiresTheOpeningSource(t *testing.T) {
	for _, body := range []string{"", `{}`, `{"account_id":"account"}`, `{"account_id":"account","directory_id":7}`} {
		t.Run(body, func(t *testing.T) {
			stub := &libraryWatchStub{}
			router := NewRouter(Dependencies{Library: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/api/library/movies/42/watched", strings.NewReader(body)))
			if response.Code != http.StatusBadRequest || stub.id != 0 {
				t.Fatalf("invalid opening source reached watch recording: status=%d id=%d body=%s", response.Code, stub.id, response.Body)
			}
		})
	}
}

type libraryOptionsStub struct {
	LibraryManager
	options service.LibraryFilterOptions
	called  bool
}

func (stub *libraryOptionsStub) FilterOptions(context.Context) (service.LibraryFilterOptions, error) {
	stub.called = true
	return stub.options, nil
}

func TestLibraryMoviesForwardEveryFilterDimension(t *testing.T) {
	stub := &libraryPageStub{}
	router := NewRouter(Dependencies{Library: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/library/movies?page=3&limit=5"+
		"&tag_id=1&tag_id=2&actor_id=actor-1&year=2025&year=2026&group_id=4&watched=no&sort=watched", nil))
	want := service.LibraryFilter{TagIDs: []int{1, 2}, ActorIDs: []string{"actor-1"}, Years: []int{2025, 2026},
		GroupIDs: []int{4}, Watched: "no", Sort: service.LibrarySortWatched}
	if response.Code != http.StatusOK || stub.page != 3 || stub.limit != 5 || !reflect.DeepEqual(stub.filter, want) {
		t.Fatalf("library filter: status=%d page=%d limit=%d filter=%+v body=%s",
			response.Code, stub.page, stub.limit, stub.filter, response.Body)
	}
}

func TestLibraryMoviesRejectUnusableFilterValues(t *testing.T) {
	for _, query := range []string{
		"?watched=maybe", "?tag_id=0", "?tag_id=abc", "?year=1899", "?year=3000",
		"?group_id=0", "?actor_id=", "?limit=101", "?sort=random",
	} {
		t.Run(query, func(t *testing.T) {
			stub := &libraryPageStub{}
			router := NewRouter(Dependencies{Library: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/library/movies"+query, nil))
			if response.Code != http.StatusBadRequest || stub.limit != 0 {
				t.Fatalf("invalid filter reached the service: status=%d limit=%d body=%s", response.Code, stub.limit, response.Body)
			}
		})
	}
}

func TestLibraryFilterOptionsEndpointReturnsTheMountedLibraryValues(t *testing.T) {
	stub := &libraryOptionsStub{options: service.LibraryFilterOptions{
		Tags:   []service.LibraryFilterOption{{ID: "1", Name: "Tag", Count: 2}},
		Actors: []service.LibraryFilterOption{},
		Years:  []service.LibraryFilterOption{{ID: "2026", Name: "2026", Count: 1}},
	}}
	router := NewRouter(Dependencies{Library: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/library/filter-options", nil))
	if response.Code != http.StatusOK || !stub.called {
		t.Fatalf("filter options: status=%d called=%t body=%s", response.Code, stub.called, response.Body)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("filter options are not an object: %s, %v", response.Body, err)
	}
	for _, dimension := range []string{"tags", "actors", "years"} {
		if _, exists := fields[dimension]; !exists {
			t.Errorf("filter options omit %s", dimension)
		}
	}
	var options service.LibraryFilterOptions
	if err := json.Unmarshal(response.Body.Bytes(), &options); err != nil {
		t.Fatal(err)
	}
	if len(options.Tags) != 1 || options.Tags[0].Name != "Tag" || options.Tags[0].Count != 2 || len(options.Years) != 1 {
		t.Fatalf("filter options = %+v", options)
	}
}
