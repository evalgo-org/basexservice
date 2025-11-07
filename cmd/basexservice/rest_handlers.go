package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
)

// REST endpoint request types

type QueryRequest struct {
	Query    string `json:"query"`
	Database string `json:"database,omitempty"`
	BaseURL  string `json:"baseUrl,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type TransformRequest struct {
	XSLT     string `json:"xslt"`
	XSLTPath string `json:"xsltPath,omitempty"`
	Database string `json:"database,omitempty"`
	BaseURL  string `json:"baseUrl,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type DatabaseRequest struct {
	Name     string `json:"name"`
	BaseURL  string `json:"baseUrl,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// registerRESTEndpoints adds REST endpoints that convert to semantic actions
func registerRESTEndpoints(apiGroup *echo.Group, apiKeyMiddleware echo.MiddlewareFunc) {
	// POST /v1/api/queries - Execute XQuery
	apiGroup.POST("/queries", executeQueryREST, apiKeyMiddleware)

	// POST /v1/api/transforms - Execute XSLT transformation
	apiGroup.POST("/transforms", executeTransformREST, apiKeyMiddleware)

	// POST /v1/api/databases - Create database
	apiGroup.POST("/databases", createDatabaseREST, apiKeyMiddleware)

	// DELETE /v1/api/databases/:name - Delete database
	apiGroup.DELETE("/databases/:name", deleteDatabaseREST, apiKeyMiddleware)
}

// executeQueryREST handles REST POST /v1/api/queries
// Converts to SearchAction and delegates to semantic handler
func executeQueryREST(c echo.Context) error {
	var req QueryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Invalid request: %v", err)})
	}

	if req.Query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "query is required"})
	}

	// Build database object
	database := map[string]interface{}{
		"@type": "Database",
	}
	if req.Database != "" {
		database["identifier"] = req.Database
	}
	if req.BaseURL != "" {
		database["url"] = req.BaseURL
	}
	if req.Username != "" {
		database["username"] = req.Username
	}
	if req.Password != "" {
		database["password"] = req.Password
	}

	// Convert to JSON-LD SearchAction
	action := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "SearchAction",
		"query":    req.Query,
		"object":   database,
	}

	return callSemanticHandler(c, action)
}

// executeTransformREST handles REST POST /v1/api/transforms
// Converts to TransformAction and delegates to semantic handler
func executeTransformREST(c echo.Context) error {
	var req TransformRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Invalid request: %v", err)})
	}

	if req.XSLT == "" && req.XSLTPath == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "xslt or xsltPath is required"})
	}

	// Build XSLT stylesheet object
	stylesheet := map[string]interface{}{
		"@type": "XSLTStylesheet",
	}
	if req.XSLT != "" {
		stylesheet["text"] = req.XSLT
	}
	if req.XSLTPath != "" {
		stylesheet["contentUrl"] = req.XSLTPath
	}

	// Build database object
	database := map[string]interface{}{
		"@type": "Database",
	}
	if req.Database != "" {
		database["identifier"] = req.Database
	}
	if req.BaseURL != "" {
		database["url"] = req.BaseURL
	}
	if req.Username != "" {
		database["username"] = req.Username
	}
	if req.Password != "" {
		database["password"] = req.Password
	}

	// Convert to JSON-LD TransformAction
	action := map[string]interface{}{
		"@context":   "https://schema.org",
		"@type":      "TransformAction",
		"instrument": stylesheet,
		"object":     database,
	}

	return callSemanticHandler(c, action)
}

// createDatabaseREST handles REST POST /v1/api/databases
// Converts to CreateAction and delegates to semantic handler
func createDatabaseREST(c echo.Context) error {
	var req DatabaseRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Invalid request: %v", err)})
	}

	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name is required"})
	}

	// Build database object
	database := map[string]interface{}{
		"@type":      "Database",
		"identifier": req.Name,
	}
	if req.BaseURL != "" {
		database["url"] = req.BaseURL
	}
	if req.Username != "" {
		database["username"] = req.Username
	}
	if req.Password != "" {
		database["password"] = req.Password
	}

	// Convert to JSON-LD CreateAction
	action := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "CreateAction",
		"object":   database,
	}

	return callSemanticHandler(c, action)
}

// deleteDatabaseREST handles REST DELETE /v1/api/databases/:name
// Converts to DeleteAction and delegates to semantic handler
func deleteDatabaseREST(c echo.Context) error {
	name := c.Param("name")
	if name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "database name is required"})
	}

	// Get optional parameters from query string
	baseURL := c.QueryParam("baseUrl")
	username := c.QueryParam("username")
	password := c.QueryParam("password")

	// Build database object
	database := map[string]interface{}{
		"@type":      "Database",
		"identifier": name,
	}
	if baseURL != "" {
		database["url"] = baseURL
	}
	if username != "" {
		database["username"] = username
	}
	if password != "" {
		database["password"] = password
	}

	// Convert to JSON-LD DeleteAction
	action := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "DeleteAction",
		"object":   database,
	}

	return callSemanticHandler(c, action)
}

// callSemanticHandler converts action to JSON and calls the semantic action handler
func callSemanticHandler(c echo.Context, action map[string]interface{}) error {
	// Marshal action to JSON
	actionJSON, err := json.Marshal(action)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to marshal action: %v", err)})
	}

	// Create new request with JSON-LD body
	newReq := c.Request().Clone(c.Request().Context())
	newReq.Body = io.NopCloser(bytes.NewReader(actionJSON))
	newReq.Header.Set("Content-Type", "application/json")

	// Create new context with modified request
	newCtx := c.Echo().NewContext(newReq, c.Response())
	newCtx.SetPath(c.Path())
	newCtx.SetParamNames(c.ParamNames()...)
	newCtx.SetParamValues(c.ParamValues()...)

	// Call the existing semantic action handler
	return handleSemanticAction(newCtx)
}
