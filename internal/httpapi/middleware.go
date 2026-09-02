package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
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

func authMiddleware(expected string) gin.HandlerFunc {
	expected = strings.TrimSpace(expected)
	return func(c *gin.Context) {
		if expected == "" || isPublicPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		value := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if len(value) <= len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
			writeError(c, http.StatusUnauthorized, "unauthorized", "bearer token is required")
			return
		}
		token := strings.TrimSpace(value[len(prefix):])
		if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
			writeError(c, http.StatusUnauthorized, "unauthorized", "bearer token is invalid")
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
