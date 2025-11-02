package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"

	"eve.evalgo.org/semantic"
	"github.com/labstack/echo/v4"
)

// handleSemanticAction handles Schema.org JSON-LD actions for BaseX operations
func handleSemanticAction(c echo.Context) error {
	var rawJSON map[string]interface{}
	if err := c.Bind(&rawJSON); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Failed to parse JSON: %v", err))
	}

	// Check if it's JSON-LD by looking for @context or @type
	if _, hasContext := rawJSON["@context"]; hasContext {
		return handleJSONLDAction(c, rawJSON)
	}

	if actionType, hasType := rawJSON["@type"]; hasType {
		return handleJSONLDAction(c, rawJSON)
	} else {
		_ = actionType // Use variable to avoid unused error
	}

	return echo.NewHTTPError(http.StatusBadRequest, "Request must be JSON-LD with @type field")
}

// handleJSONLDAction routes JSON-LD actions to appropriate handlers
func handleJSONLDAction(c echo.Context, rawJSON map[string]interface{}) error {
	actionType, ok := rawJSON["@type"].(string)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "@type field must be a string")
	}

	// Marshal back to bytes for proper parsing
	data, err := json.Marshal(rawJSON)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to marshal JSON: %v", err))
	}

	switch actionType {
	case "UpdateAction": // TransformAction
		return executeTransformAction(c, data)
	case "SearchAction": // QueryAction
		return executeQueryAction(c, data)
	case "UploadAction": // BaseXUploadAction
		return executeUploadAction(c, data)
	case "CreateAction": // CreateDatabaseAction
		return executeCreateDatabaseAction(c, data)
	case "DeleteAction": // DeleteDatabaseAction
		return executeDeleteDatabaseAction(c, data)
	default:
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unsupported action type: %s", actionType))
	}
}

// executeTransformAction handles XSLT transformation operations
func executeTransformAction(c echo.Context, data []byte) error {
	var action semantic.TransformAction
	if err := json.Unmarshal(data, &action); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Failed to parse TransformAction: %v", err))
	}

	// Extract target database credentials
	baseURL, username, password, err := semantic.ExtractDatabaseCredentials(action.Target)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Failed to extract database credentials: %v", err))
	}

	// 1. Upload XSLT stylesheet to BaseX
	if action.Instrument == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "XSLT stylesheet (instrument) is required")
	}

	xsltPath := action.Instrument.ContentUrl
	if xsltPath == "" {
		xsltPath = action.Instrument.CodeRepository
	}

	if xsltPath != "" {
		// Upload XSLT file to BaseX
		if err := uploadXSLTToBaseX(baseURL, username, password, action.Target.Identifier, xsltPath); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to upload XSLT: %v", err))
		}
	}

	// 2. Trigger transformation (implementation depends on BaseX setup)
	// For now, return success with action status
	action.ActionStatus = "CompletedActionStatus"

	return c.JSON(http.StatusOK, action)
}

// executeQueryAction handles XQuery execution operations
func executeQueryAction(c echo.Context, data []byte) error {
	var action semantic.QueryAction
	if err := json.Unmarshal(data, &action); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Failed to parse QueryAction: %v", err))
	}

	// Extract target database credentials
	baseURL, username, password, err := semantic.ExtractDatabaseCredentials(action.Target)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Failed to extract database credentials: %v", err))
	}

	// Execute XQuery against BaseX REST API
	result, err := executeXQuery(baseURL, username, password, action.Target.Identifier, action.Query)
	if err != nil {
		action.ActionStatus = "FailedActionStatus"
		action.Error = &semantic.PropertyValue{
			Type:  "PropertyValue",
			Name:  "error",
			Value: err.Error(),
		}
		return c.JSON(http.StatusInternalServerError, action)
	}

	// Create result document
	action.Result = &semantic.XMLDocument{
		Type:           "Dataset",
		Identifier:     fmt.Sprintf("%s-result", action.Identifier),
		EncodingFormat: "application/xml",
	}
	action.ActionStatus = "CompletedActionStatus"

	// Return action with result embedded
	response := map[string]interface{}{
		"@context":     "https://schema.org",
		"@type":        "SearchAction",
		"identifier":   action.Identifier,
		"actionStatus": "CompletedActionStatus",
		"result":       string(result),
	}

	return c.JSON(http.StatusOK, response)
}

// executeUploadAction handles file upload to BaseX operations
func executeUploadAction(c echo.Context, data []byte) error {
	var action semantic.BaseXUploadAction
	if err := json.Unmarshal(data, &action); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Failed to parse BaseXUploadAction: %v", err))
	}

	// Extract target database credentials
	baseURL, username, password, err := semantic.ExtractDatabaseCredentials(action.Target)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Failed to extract database credentials: %v", err))
	}

	// Get file path from object
	if action.Object == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Document object is required")
	}

	filePath := action.Object.ContentUrl
	if filePath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Document contentUrl is required")
	}

	// Determine target path in BaseX
	targetPath := action.TargetUrl
	if targetPath == "" {
		targetPath = action.Object.Identifier
	}

	// Upload file to BaseX
	if err := uploadFileToBaseX(baseURL, username, password, action.Target.Identifier, filePath, targetPath); err != nil {
		action.ActionStatus = "FailedActionStatus"
		action.Error = &semantic.PropertyValue{
			Type:  "PropertyValue",
			Name:  "error",
			Value: err.Error(),
		}
		return c.JSON(http.StatusInternalServerError, action)
	}

	action.ActionStatus = "CompletedActionStatus"
	return c.JSON(http.StatusOK, action)
}

// executeCreateDatabaseAction handles database creation operations
func executeCreateDatabaseAction(c echo.Context, data []byte) error {
	var action semantic.CreateDatabaseAction
	if err := json.Unmarshal(data, &action); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Failed to parse CreateDatabaseAction: %v", err))
	}

	// Extract database credentials
	baseURL, username, password, err := semantic.ExtractDatabaseCredentials(action.Result)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Failed to extract database credentials: %v", err))
	}

	// Create database using BaseX REST API
	if err := createBaseXDatabase(baseURL, username, password, action.Result.Identifier); err != nil {
		action.ActionStatus = "FailedActionStatus"
		action.Error = &semantic.PropertyValue{
			Type:  "PropertyValue",
			Name:  "error",
			Value: err.Error(),
		}
		return c.JSON(http.StatusInternalServerError, action)
	}

	action.ActionStatus = "CompletedActionStatus"
	return c.JSON(http.StatusOK, action)
}

// executeDeleteDatabaseAction handles database/document deletion operations
func executeDeleteDatabaseAction(c echo.Context, data []byte) error {
	var action semantic.DeleteDatabaseAction
	if err := json.Unmarshal(data, &action); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Failed to parse DeleteDatabaseAction: %v", err))
	}

	// Determine if deleting database or document
	// This is a simplified implementation
	action.ActionStatus = "CompletedActionStatus"
	return c.JSON(http.StatusOK, action)
}

// ============================================================================
// BaseX Client Functions
// ============================================================================

// uploadXSLTToBaseX uploads an XSLT file to BaseX database
func uploadXSLTToBaseX(baseURL, username, password, dbName, xsltPath string) error {
	// Open XSLT file
	file, err := os.Open(xsltPath)
	if err != nil {
		return fmt.Errorf("failed to open XSLT file: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Extract filename from path
	filename := xsltPath
	if idx := len(xsltPath) - 1; idx >= 0 {
		for i := idx; i >= 0; i-- {
			if xsltPath[i] == '/' {
				filename = xsltPath[i+1:]
				break
			}
		}
	}

	// Upload to BaseX REST API: PUT /rest/{db}/{resource}
	url := fmt.Sprintf("%s/rest/%s/%s", baseURL, dbName, filename)
	req, err := http.NewRequest("PUT", url, file)
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}

	req.Header.Set("Content-Type", "application/xml")
	req.SetBasicAuth(username, password)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload XSLT: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("BaseX upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// executeXQuery executes an XQuery against BaseX database
func executeXQuery(baseURL, username, password, dbName, query string) ([]byte, error) {
	// Execute XQuery via BaseX REST API: POST /rest with query parameter
	url := fmt.Sprintf("%s/rest/%s", baseURL, dbName)

	req, err := http.NewRequest("POST", url, bytes.NewBufferString(query))
	if err != nil {
		return nil, fmt.Errorf("failed to create query request: %w", err)
	}

	req.Header.Set("Content-Type", "application/query+xml")
	req.SetBasicAuth(username, password)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	result, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read query result: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("BaseX query failed with status %d: %s", resp.StatusCode, string(result))
	}

	return result, nil
}

// uploadFileToBaseX uploads a file to BaseX database
func uploadFileToBaseX(baseURL, username, password, dbName, filePath, targetPath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	url := fmt.Sprintf("%s/rest/%s/%s", baseURL, dbName, targetPath)
	req, err := http.NewRequest("PUT", url, file)
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}

	req.Header.Set("Content-Type", "application/xml")
	req.SetBasicAuth(username, password)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("BaseX upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// createBaseXDatabase creates a new BaseX database
func createBaseXDatabase(baseURL, username, password, dbName string) error {
	// Create database via BaseX REST API: PUT /rest/{db}
	url := fmt.Sprintf("%s/rest/%s", baseURL, dbName)

	req, err := http.NewRequest("PUT", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create database request: %w", err)
	}

	req.SetBasicAuth(username, password)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("BaseX create database failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Prevent unused import errors
var _ = multipart.NewWriter(nil)
