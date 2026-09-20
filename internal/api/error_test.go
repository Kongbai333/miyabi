package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/netx"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/service"
)

type publicError struct{ message string }

func (err *publicError) Error() string         { return "internal detail: " + err.message }
func (err *publicError) PublicMessage() string { return err.message }

func errorRouter(handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(errorMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil))))
	router.GET("/probe", handler)
	return router
}

func TestErrorMiddlewareMapsDomainErrorsToStatusAndMessage(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{name: "bad request wrapper", err: BadRequest(errors.New("page must be positive")), status: http.StatusBadRequest, message: "page must be positive"},
		{name: "media directory required", err: service.ErrMediaDirectoryRequired, status: http.StatusBadRequest, message: service.ErrMediaDirectoryRequired.Error()},
		{name: "magnet not found", err: fmt.Errorf("add: %w", service.ErrMagnetNotFound), status: http.StatusBadRequest, message: "add: " + service.ErrMagnetNotFound.Error()},
		{name: "invalid progress", err: service.ErrInvalidWatchProgress, status: http.StatusBadRequest, message: service.ErrInvalidWatchProgress.Error()},
		{name: "invalid proxy", err: fmt.Errorf("%w: 代理地址格式错误", netx.ErrInvalidProxy), status: http.StatusBadRequest, message: "代理配置无效: 代理地址格式错误"},
		{name: "history source changed", err: service.ErrWatchHistorySourceChanged, status: http.StatusConflict, message: service.ErrWatchHistorySourceChanged.Error()},
		{name: "cache busy", err: service.ErrCacheBusy, status: http.StatusConflict, message: service.ErrCacheBusy.Error()},
		{name: "file missing", err: fmt.Errorf("影片文件不存在，请重新扫描: %w", fs.ErrNotExist), status: http.StatusNotFound, message: "影片文件不存在，请重新扫描: file does not exist"},
		{name: "pan unauthorized", err: fmt.Errorf("list: %w", pan.ErrUnauthorized), status: http.StatusUnauthorized, message: "list: " + pan.ErrUnauthorized.Error()},
		{name: "access password", err: service.ErrAccessPassword, status: http.StatusUnauthorized, message: service.ErrAccessPassword.Error()},
		{name: "public message wins", err: fmt.Errorf("wrapped: %w", &publicError{message: "115 说明文案"}), status: http.StatusInternalServerError, message: "115 说明文案"},
		{name: "unknown error leaks text", err: errors.New("UNIQUE constraint failed: movies.code"), status: http.StatusInternalServerError, message: "UNIQUE constraint failed: movies.code"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			router := errorRouter(func(c *gin.Context) { c.Error(scenario.err) })
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/probe", nil))
			if response.Code != scenario.status {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, scenario.status, response.Body)
			}
			var body struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("body is not JSON: %v\n%s", err, response.Body)
			}
			if body.Error != scenario.message {
				t.Fatalf("message = %q, want %q", body.Error, scenario.message)
			}
		})
	}
}

func TestErrorMiddlewareSkipsWrittenResponsesAndCanceledRequests(t *testing.T) {
	t.Run("written response is kept", func(t *testing.T) {
		router := errorRouter(func(c *gin.Context) {
			c.JSON(http.StatusAccepted, gin.H{"ok": true})
			c.Error(errors.New("late failure"))
		})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/probe", nil))
		if response.Code != http.StatusAccepted || response.Body.String() != `{"ok":true}` {
			t.Fatalf("response = %d %s", response.Code, response.Body)
		}
	})
	t.Run("client cancellation becomes 499", func(t *testing.T) {
		router := errorRouter(func(c *gin.Context) { c.Error(fmt.Errorf("stream: %w", context.Canceled)) })
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/probe", nil).WithContext(ctx))
		if response.Code != statusClientClosedRequest || response.Body.Len() != 0 {
			t.Fatalf("response = %d %s", response.Code, response.Body)
		}
	})
	t.Run("canceled error on a live request is a server failure", func(t *testing.T) {
		router := errorRouter(func(c *gin.Context) { c.Error(context.Canceled) })
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/probe", nil))
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d", response.Code)
		}
	})
}
