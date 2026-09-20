package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/netx"
	"github.com/ppxb/miyabi/internal/service"
)

type NetworkManager interface {
	Network(context.Context) (netx.ProxyConfig, error)
	UpdateNetwork(context.Context, netx.ProxyConfig) error
	TestNetwork(context.Context, netx.ProxyConfig) (service.NetworkTestResponse, error)
}

func networkHandler(network NetworkManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		config, err := network.Network(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, publicNetworkConfig(config))
	}
}

func networkUpdateHandler(network NetworkManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var config netx.ProxyConfig
		if err := c.ShouldBindJSON(&config); err != nil {
			c.Error(BadRequest(err))
			return
		}
		if err := netx.Validate(config); err != nil {
			c.Error(BadRequest(err))
			return
		}
		if err := network.UpdateNetwork(c.Request.Context(), config); err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, publicNetworkConfig(config))
	}
}

func networkTestHandler(network NetworkManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var config netx.ProxyConfig
		hasCandidate := false
		if c.Request.Body != nil && c.Request.ContentLength > 0 {
			var candidate netx.ProxyConfig
			if err := c.ShouldBindJSON(&candidate); err == nil && (candidate.Enabled || candidate.URL != "") {
				if err := netx.Validate(candidate); err != nil {
					c.Error(BadRequest(err))
					return
				}
				config = candidate
				hasCandidate = true
			}
		}
		if !hasCandidate {
			current, err := network.Network(c.Request.Context())
			if err != nil {
				c.Error(err)
				return
			}
			config = current
		}
		result, err := network.TestNetwork(c.Request.Context(), config)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func publicNetworkConfig(config netx.ProxyConfig) netx.ProxyConfig {
	config.URL = redactProxyPassword(config.URL)
	return config
}

func redactProxyPassword(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User == nil {
		return raw
	}
	username := parsed.User.Username()
	if _, hasPassword := parsed.User.Password(); hasPassword {
		parsed.User = nil
		prefix := parsed.Scheme + "://"
		return prefix + username + ":" + netx.MaskedPassword + "@" + strings.TrimPrefix(parsed.String(), prefix)
	}
	return parsed.String()
}
