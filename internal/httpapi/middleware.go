package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sparklyi/tunnelbox/internal/auth"
)

const requestIDKey = "request_id"

var requestSequence atomic.Uint64

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if !validRequestID(id) {
			id = newRequestID()
		}
		c.Set(requestIDKey, id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func recoveryMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("http handler panic", "request_id", requestID(c), "panic", fmt.Sprint(recovered))
				writeError(c, http.StatusInternalServerError, "internal_error", "internal server error")
			}
		}()
		c.Next()
	}
}

func loggingMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		logger.Info("http request", "request_id", requestID(c), "method", c.Request.Method,
			"path", c.FullPath(), "status", c.Writer.Status(), "duration_ms", time.Since(started).Milliseconds())
	}
}

func errorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) > 0 && !c.Writer.Written() {
			writeDomainError(c, c.Errors.Last().Err)
		}
	}
}

func authMiddleware(manager *auth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isPublicPath(c.Request.URL.Path) || strings.HasPrefix(c.Request.URL.Path, "/api/v1/auth/") {
			c.Next()
			return
		}
		if manager == nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "authentication is not configured")
			return
		}
		token, err := c.Cookie(auth.SessionCookie)
		if err != nil {
			writeError(c, http.StatusUnauthorized, "unauthorized", "login is required")
			return
		}
		valid, err := manager.Authenticate(c.Request.Context(), token)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "authentication failed")
			return
		}
		if !valid {
			writeError(c, http.StatusUnauthorized, "unauthorized", "login is required")
			return
		}
		c.Next()
	}
}

func isPublicPath(path string) bool {
	return path == "/healthz" || path == "/readyz" || path == "/" || path == "/assets" || strings.HasPrefix(path, "/assets/")
}

func requestID(c *gin.Context) string {
	value, exists := c.Get(requestIDKey)
	if !exists {
		return ""
	}
	result, _ := value.(string)
	return result
}

func validRequestID(value string) bool {
	if value == "" || len(value) > 96 {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func newRequestID() string {
	var bytes [10]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return "req_" + hex.EncodeToString(bytes[:])
	}
	return fmt.Sprintf("req_%d", requestSequence.Add(1))
}
