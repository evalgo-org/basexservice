package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"eve.evalgo.org/common"
	evehttp "eve.evalgo.org/http"
	"eve.evalgo.org/pkg/statemanager"
	"eve.evalgo.org/registry"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	// Initialize logger
	logger := common.ServiceLogger("basexservice", "1.0.0")

	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// EVE health check
	e.GET("/health", evehttp.HealthCheckHandler("basexservice", "1.0.0"))

	// Documentation endpoint
	e.GET("/v1/api/docs", evehttp.DocumentationHandler(evehttp.ServiceDocConfig{
		ServiceID:    "basexservice",
		ServiceName:  "BaseX XML Database Service",
		Description:  "XML database with XQuery and XSLT transformation support",
		Version:      "v1",
		Port:         8090,
		Capabilities: []string{"xml-database", "xquery", "xslt", "xml-storage", "state-tracking"},
		Endpoints: []evehttp.EndpointDoc{
			{
				Method:      "POST",
				Path:        "/v1/api/semantic/action",
				Description: "Execute semantic actions (XQuery, XSLT, CreateAction, etc.)",
			},
			{
				Method:      "GET",
				Path:        "/health",
				Description: "Health check endpoint",
			},
		},
	}))

	// Initialize state manager
	sm := statemanager.New(statemanager.Config{
		ServiceName:   "basexservice",
		MaxOperations: 100,
	})

	// Register state endpoints
	apiGroup := e.Group("/v1/api")
	sm.RegisterRoutes(apiGroup)

	// EVE API Key middleware
	apiKey := os.Getenv("BASEX_API_KEY")
	apiKeyMiddleware := evehttp.APIKeyMiddleware(apiKey)
	e.POST("/v1/api/semantic/action", handleSemanticAction, apiKeyMiddleware)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	// Auto-register with registry service if REGISTRYSERVICE_API_URL is set
	portInt, _ := strconv.Atoi(port)
	if _, err := registry.AutoRegister(registry.AutoRegisterConfig{
		ServiceID:    "basexservice",
		ServiceName:  "BaseX XML Database Service",
		Description:  "XML database with XQuery and XSLT transformation support",
		Port:         portInt,
		Directory:    "/home/opunix/basexservice",
		Binary:       "basexservice",
		Version:      "v1",
		Capabilities: []string{"xml-database", "xquery", "xslt", "xml-storage", "state-tracking"},
		APIVersions: []registry.APIVersion{
			{
				Version:       "v1",
				URL:           fmt.Sprintf("http://localhost:%d/v1", portInt),
				Documentation: fmt.Sprintf("http://localhost:%d/v1/api/docs", portInt),
				IsDefault:     true,
				Status:        "stable",
				ReleaseDate:   "2024-01-01",
				Capabilities:  []string{"xml-database", "xquery", "xslt", "xml-storage"},
			},
		},
	}); err != nil {
		logger.WithError(err).Error("Failed to register with registry")
	}

	// Start server in goroutine
	go func() {
		logger.Infof("Starting BaseX Semantic Service on port %s", port)
		if err := e.Start(":" + port); err != nil {
			logger.WithError(err).Error("Server error")
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Unregister from registry
	if err := registry.AutoUnregister("basexservice"); err != nil {
		logger.WithError(err).Error("Failed to unregister from registry")
	}

	// Shutdown server
	if err := e.Close(); err != nil {
		logger.WithError(err).Error("Error during shutdown")
	}

	logger.Info("Server stopped")
}
