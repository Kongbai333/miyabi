package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type markViewedStub struct {
	Discoverer
	id     string
	failed error
	calls  int
}

func (stub *markViewedStub) MarkViewed(_ context.Context, id string) error {
	stub.calls++
	stub.id = id
	return stub.failed
}

func TestMarkViewedHandlerRequiresAnIDAndDisablesHTTPCaching(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, scenario := range []struct {
		name string
		id   string
	}{
		{"valid id", "abc123"},
		{"path escaped id", "a/b"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/discover/movies/x/viewed", nil)
			c.Params = gin.Params{{Key: "id", Value: scenario.id}}
			stub := &markViewedStub{}
			discoverMarkViewedHandler(stub)(c)
			if stub.calls != 1 || stub.id != scenario.id {
				t.Fatalf("service call = %d, id = %q", stub.calls, stub.id)
			}
			if response.Code != http.StatusOK ||
				response.Header().Get("Cache-Control") != "no-store" ||
				response.Body.String() != `{"id":"`+scenario.id+`","viewed":true}` {
				t.Fatalf("response = %d %s", response.Code, response.Body)
			}
		})
	}
}

func TestMarkViewedHandlerRejectsAMissingID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/discover/movies//viewed", nil)
	c.Params = gin.Params{{Key: "id", Value: ""}}
	stub := &markViewedStub{}
	discoverMarkViewedHandler(stub)(c)
	if len(c.Errors) != 1 || stub.calls != 0 {
		t.Fatalf("blank id reached the service: errors=%v calls=%d", c.Errors, stub.calls)
	}
}
