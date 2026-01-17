// Package http provides the HTTP server implementation for the orchestrator.
package http

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/xiaot623/gogo/orchestrator/internal/service"
	"github.com/xiaot623/gogo/orchestrator/internal/transport/http/internalapi"
	"github.com/xiaot623/gogo/orchestrator/internal/transport/http/llmproxy"
	v1 "github.com/xiaot623/gogo/orchestrator/internal/transport/http/v1"
)

// NewExternalServer creates and configures the external-facing HTTP server.
// This server handles agent registration, tool invocations, and LLM proxying.
func NewExternalServer(svc *service.Service) *echo.Echo {
	e := echo.New()
	e.HideBanner = true

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Handlers
	v1Handler := v1.NewHandler(svc)
	llmHandler := llmproxy.NewHandler(svc)

	// Register Routes
	v1Handler.RegisterRoutes(e)
	llmHandler.RegisterRoutes(e)

	return e
}

// NewInternalServer creates and configures the internal-facing HTTP server.
// This server handles requests from the ingress service and other internal components.
func NewInternalServer(svc *service.Service) *echo.Echo {
	e := echo.New()
	e.HideBanner = true

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Handlers
	internalHandler := internalapi.NewHandler(svc)

	// Register Routes
	internalHandler.RegisterRoutes(e)

	return e
}
