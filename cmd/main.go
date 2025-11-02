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

	// Routes
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "healthy"})
	})

	// API Key middleware - only apply if BASEX_API_KEY is set
	apiKey := os.Getenv("BASEX_API_KEY")
	if apiKey != "" {
		// Production mode: require API key
		apiKeyMiddleware := middleware.KeyAuth(func(key string, c echo.Context) (bool, error) {
			return key == apiKey, nil
		})
		e.POST("/v1/api/semantic/action", handleSemanticAction, apiKeyMiddleware)
		log.Printf("API Key authentication enabled")
	} else {
		// Development mode: no authentication
		e.POST("/v1/api/semantic/action", handleSemanticAction)
		log.Printf("Running in development mode (no API key required)")
	}

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
