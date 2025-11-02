package main

import (
	"fmt"
	"log"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// API Key middleware
	apiKeyMiddleware := middleware.KeyAuth(func(key string, c echo.Context) (bool, error) {
		apiKey := os.Getenv("BASEX_API_KEY")
		if apiKey == "" {
			// If no API key is set, allow all requests (development mode)
			return true, nil
		}
		return key == apiKey, nil
	})

	// Routes
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "healthy"})
	})

	// Semantic API endpoints
	e.POST("/v1/api/semantic/action", handleSemanticAction, apiKeyMiddleware)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	log.Printf("Starting BaseX Semantic Service on port %s", port)
	if err := e.Start(":" + port); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
