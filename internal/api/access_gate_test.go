package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/service"
)

// accessRouter wires the real gate so the test exercises the middleware the
// deployment relies on, not a stub that always allows.
func accessRouter(password string) http.Handler {
	return NewRouter(Dependencies{
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Access:  service.NewAccessGateService(password),
		Library: accessLibraryStub{},
		Discover: accessDiscoverStub{},
		Health:   accessHealthStub{},
	})
}

type accessHealthStub struct{}

func (accessHealthStub) Ping(context.Context) error { return nil }

type accessDiscoverStub struct {
	Discoverer
}

func (accessDiscoverStub) Tags(context.Context, domain.Zone) ([]domain.TagCategory, error) {
	return []domain.TagCategory{}, nil
}

func (accessDiscoverStub) MovieStates(_ context.Context, movies []service.MovieIdentity) ([]service.DiscoverMovieState, error) {
	return []service.DiscoverMovieState{}, nil
}

func (accessDiscoverStub) MarkViewed(context.Context, string) error { return nil }

type accessLibraryStub struct {
	LibraryManager
}

func (accessLibraryStub) Movies(context.Context, int, int, service.LibraryFilter) (service.LibraryPage, error) {
	return service.LibraryPage{Movies: []service.LibraryMovie{}}, nil
}

func TestProtectedRoutesRequireTheSessionCookie(t *testing.T) {
	router := accessRouter("test-password")
	protected := []struct{ method, path string }{
		{http.MethodGet, "/api/library/movies"},
		{http.MethodGet, "/api/library/filter-options"},
		{http.MethodGet, "/api/tasks"},
		{http.MethodGet, "/api/settings/system"},
		{http.MethodGet, "/api/discover/tags"},
		{http.MethodPost, "/api/discover/movie-states"},
		{http.MethodPut, "/api/discover/movies/abc/viewed"},
		{http.MethodGet, "/api/image?url=https://example.com/a.jpg"},
		{http.MethodGet, "/api/play/7"},
	}

	// Without a cookie every protected route must refuse before reaching its handler.
	for _, route := range protected {
		t.Run("anonymous "+route.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(route.method, route.path, nil))
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d body = %s", response.Code, response.Body)
			}
		})
	}

	// The healthcheck and the sign-in endpoints must stay reachable.
	for _, route := range []string{"/api/health", "/api/auth/config"} {
		t.Run("open "+route, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", response.Code, response.Body)
			}
		})
	}
}

func TestLoginIssuesACookieThatUnlocksProtectedRoutes(t *testing.T) {
	router := accessRouter("test-password")

	wrong := httptest.NewRecorder()
	router.ServeHTTP(wrong, httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"password":"wrong"}`)))
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password status = %d", wrong.Code)
	}
	if len(wrong.Result().Cookies()) != 0 {
		t.Fatal("a failed login issued a cookie")
	}

	login := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"password":"test-password"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(login, request)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d body = %s", login.Code, login.Body)
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login set %d cookies", len(cookies))
	}
	if cookie := cookies[0]; !cookie.HttpOnly || cookie.Path != "/" {
		t.Fatalf("cookie is not hardened: %+v", cookie)
	}

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/library/movies"},
		{http.MethodGet, "/api/discover/tags"},
		{http.MethodPost, "/api/discover/movie-states"},
	} {
		t.Run("authenticated "+route.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(route.method, route.path, nil)
			request.AddCookie(cookies[0])
			router.ServeHTTP(response, request)
			if response.Code == http.StatusUnauthorized {
				t.Fatalf("session cookie was rejected on %s", route.path)
			}
		})
	}
}

func TestLogoutExpiresTheCookie(t *testing.T) {
	router := accessRouter("test-password")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("logout status = %d", response.Code)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge >= 0 || cookies[0].Value != "" {
		t.Fatalf("logout did not expire the cookie: %+v", cookies)
	}
}

func TestDisabledGateLeavesRoutesOpen(t *testing.T) {
	router := accessRouter("")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/library/movies", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body)
	}
}
