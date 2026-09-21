package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// accessCookieName carries the signed session token. It is HttpOnly so page
// scripts cannot read it, and SameSite=Lax still allows top-level navigation
// into a bookmarked page while blocking cross-site subrequests.
const accessCookieName = "miyabi_access"

type AccessGate interface {
	Enabled() bool
	Verify(string) error
	IssueSession() (string, time.Time)
	ValidSession(string) bool
}

type accessLoginInput struct {
	Password string `json:"password" binding:"required"`
}

func accessConfigHandler(gate AccessGate) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"enabled": gate.Enabled()})
	}
}

func accessLoginHandler(gate AccessGate) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input accessLoginInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Error(BadRequest(err))
			return
		}
		if err := gate.Verify(input.Password); err != nil {
			c.Error(err)
			return
		}
		if gate.Enabled() {
			token, expires := gate.IssueSession()
			http.SetCookie(c.Writer, &http.Cookie{
				Name:     accessCookieName,
				Value:    token,
				Path:     "/",
				Expires:  expires,
				MaxAge:   int(time.Until(expires).Seconds()),
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				Secure:   c.Request.TLS != nil,
			})
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// accessLogoutHandler expires the cookie so a shared browser can hand the
// session back without waiting for the token to lapse.
func accessLogoutHandler(gate AccessGate) gin.HandlerFunc {
	return func(c *gin.Context) {
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     accessCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   c.Request.TLS != nil,
		})
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// accessSessionHandler lets the page decide between the login form and the
// app. It sits behind requireAccess, so a 200 means the cookie is valid.
func accessSessionHandler(gate AccessGate) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"authenticated": true, "enabled": gate.Enabled()})
	}
}

// accessOpenRoutes stay reachable without a session: the container
// healthcheck and the sign-in flow itself. Everything else, including
// images and streams, needs the cookie.
var accessOpenRoutes = map[string]bool{
	"/api/health":      true,
	"/api/auth/config": true,
	"/api/auth/login":  true,
	"/api/auth/logout": true,
}

func requireAccess(gate AccessGate) gin.HandlerFunc {
	return func(c *gin.Context) {
		if gate == nil || !gate.Enabled() || accessOpenRoutes[c.Request.URL.Path] {
			c.Next()
			return
		}
		token, err := c.Cookie(accessCookieName)
		if err != nil || !gate.ValidSession(token) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "访问密码已失效，请重新登录"})
			return
		}
		c.Next()
	}
}

