package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sparklyi/tunnelbox/internal/operation"
	"github.com/sparklyi/tunnelbox/internal/service"
)

// ServiceActions is the small API contract needed by the HTTP handlers.
type ServiceActions interface {
	List(context.Context) ([]service.Service, error)
	Get(context.Context, string) (service.Service, error)
	Create(context.Context, service.CreateInput) (service.Service, error)
	Update(context.Context, string, service.UpdateInput) (service.Service, error)
	Delete(context.Context, string) error
}

// OperationReader is kept separate so read-only operation endpoints do not
// depend on the operation runner implementation.
type OperationReader interface {
	Get(context.Context, string) (operation.Operation, error)
}

// ServiceDeployer starts an asynchronous deployment for one service.
type ServiceDeployer interface {
	Deploy(context.Context, string) (operation.Operation, error)
}

type Dependencies struct {
	Services   ServiceActions
	Operations OperationReader
	Deployer   ServiceDeployer
	AdminToken string
	Logger     *slog.Logger
	Readiness  func(context.Context) error
}

func NewRouter(deps Dependencies) (*gin.Engine, error) {
	if deps.Services == nil {
		return nil, errors.New("http api requires service actions")
	}
	if deps.Operations == nil {
		return nil, errors.New("http api requires operation reader")
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}

	router := gin.New()
	router.Use(requestIDMiddleware(), recoveryMiddleware(deps.Logger), loggingMiddleware(deps.Logger), errorMiddleware())
	router.Use(authMiddleware(deps.AdminToken))

	router.GET("/healthz", healthHandler)
	router.GET("/readyz", func(c *gin.Context) {
		if deps.Readiness != nil {
			if err := deps.Readiness(c.Request.Context()); err != nil {
				writeError(c, http.StatusServiceUnavailable, "not_ready", "tunnelbox is not ready")
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := router.Group("/api/v1")
	api.GET("/services", listServicesHandler(deps.Services))
	api.POST("/services", createServiceHandler(deps.Services))
	api.GET("/services/:id", getServiceHandler(deps.Services))
	api.PATCH("/services/:id", updateServiceHandler(deps.Services))
	api.DELETE("/services/:id", deleteServiceHandler(deps.Services))
	api.POST("/services/:id/deploy", deployServiceHandler(deps.Deployer))
	api.GET("/operations/:id", getOperationHandler(deps.Operations))
	return router, nil
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func contextForOperation(c *gin.Context) context.Context {
	// A disconnected HTTP client must not cancel a deployment already accepted.
	return context.WithoutCancel(c.Request.Context())
}
