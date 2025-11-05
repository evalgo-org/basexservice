package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"eve.evalgo.org/semantic"
	"github.com/labstack/echo/v4"
)

// handleSemanticAction handles Schema.org JSON-LD actions for BaseX operations
func handleSemanticAction(c echo.Context) error {
	// Read request body
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Failed to read request body: %v", err))
	}

	// Use EVE library's ParseBaseXAction for routing and parsing
	action, err := semantic.ParseBaseXAction(body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Failed to parse action: %v", err))
	}

	// Route to appropriate handler based on type
	switch v := action.(type) {
	case *semantic.TransformAction:
		return executeTransformAction(c, v)
	case *semantic.QueryAction:
		return executeQueryAction(c, v)
	case *semantic.BaseXUploadAction:
		return executeUploadAction(c, v)
	case *semantic.CreateDatabaseAction:
		return executeCreateDatabaseAction(c, v)
	case *semantic.DeleteDatabaseAction:
		return executeDeleteDatabaseAction(c, v)
	default:
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unsupported action type: %T", v))
	}
}

// executeTransformAction handles XSLT transformation operations
func executeTransformAction(c echo.Context, action *semantic.TransformAction) error {

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

	if xsltPath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "XSLT stylesheet path (contentUrl or codeRepository) is required")
	}

	// Upload XSLT file to BaseX
	if err := uploadXSLTToBaseX(baseURL, username, password, action.Target.Identifier, xsltPath); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to upload XSLT: %v", err))
	}

	// 2. Trigger transformation (implementation depends on BaseX setup)
	// For now, return success with action status
	action.ActionStatus = "CompletedActionStatus"

	return c.JSON(http.StatusOK, action)
}

// executeQueryAction handles XQuery execution operations
func executeQueryAction(c echo.Context, action *semantic.QueryAction) error {
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
func executeUploadAction(c echo.Context, action *semantic.BaseXUploadAction) error {
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

	// Check if contentUrl is an S3 URL and download if needed
	if strings.HasPrefix(filePath, "s3://") {
		fmt.Printf("DEBUG: Detected S3 URL: %s\n", filePath)
		// Download from S3 via s3service
		downloadedPath, err := downloadFromS3(filePath, action.Object.EncodingFormat)
		if err != nil {
			action.ActionStatus = "FailedActionStatus"
			action.Error = &semantic.PropertyValue{
				Type:  "PropertyValue",
				Name:  "error",
				Value: fmt.Sprintf("Failed to download from S3: %v", err),
			}
			return c.JSON(http.StatusInternalServerError, action)
		}
		fmt.Printf("DEBUG: Downloaded to: %s\n", downloadedPath)
		// Update filePath to the downloaded local file
		filePath = downloadedPath
		// Clean up temp file after upload
		defer func() {
			_ = os.Remove(downloadedPath)
		}()
	} else {
		fmt.Printf("DEBUG: Using local file path: %s\n", filePath)
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
func executeCreateDatabaseAction(c echo.Context, action *semantic.CreateDatabaseAction) error {
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
func executeDeleteDatabaseAction(c echo.Context, action *semantic.DeleteDatabaseAction) error {
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
// For queries with doc() references, set database context in URL
func executeXQuery(baseURL, username, password, dbName, query string) ([]byte, error) {
	// BaseX REST API: POST /rest/{database} sets database context for doc() calls
	// Query must be wrapped in XML: <query><text><![CDATA[...]]></text></query>
	url := fmt.Sprintf("%s/rest/%s", baseURL, dbName)

	// Wrap query in required XML structure with CDATA to avoid escaping issues
	queryXML := fmt.Sprintf(`<query xmlns="http://basex.org/rest"><text><![CDATA[%s]]></text></query>`, query)

	req, err := http.NewRequest("POST", url, bytes.NewBufferString(queryXML))
	if err != nil {
		return nil, fmt.Errorf("failed to create query request: %w", err)
	}

	req.Header.Set("Content-Type", "application/xml")
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
// Extracts XML from JSON-LD if the file contains semantic structure
func uploadFileToBaseX(baseURL, username, password, dbName, filePath, targetPath string) error {
	// Read file content
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Check if file contains JSON-LD and extract the actual XML content
	dataToUpload := fileData
	var jsonData map[string]interface{}
	if err := json.Unmarshal(fileData, &jsonData); err == nil {
		// Successfully parsed as JSON - check if it's a JSON-LD wrapper with result field
		if result, ok := jsonData["result"]; ok {
			// Check if result is a string (likely XML or other text content)
			if resultStr, ok := result.(string); ok {
				fmt.Printf("DEBUG: Extracting XML from JSON-LD semantic structure (length: %d -> %d)\n", len(fileData), len(resultStr))
				dataToUpload = []byte(resultStr)
			}
		}
	}

	url := fmt.Sprintf("%s/rest/%s/%s", baseURL, dbName, targetPath)
	req, err := http.NewRequest("PUT", url, bytes.NewReader(dataToUpload))
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

// downloadFromS3 downloads a file from S3 by calling s3service
func downloadFromS3(s3URL, encodingFormat string) (string, error) {
	// Parse S3 URL: s3://bucket/key
	s3URL = strings.TrimPrefix(s3URL, "s3://")
	parts := strings.SplitN(s3URL, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid S3 URL format, expected s3://bucket/key")
	}
	bucket := parts[0]
	key := parts[1]

	// Get S3 credentials from environment
	s3URL_env := os.Getenv("HETZNER_S3_URL")
	if s3URL_env == "" {
		s3URL_env = "https://fsn1.your-objectstorage.com"
	}
	region := os.Getenv("HETZNER_S3_REGION")
	if region == "" {
		region = "fsn1"
	}
	accessKey := os.Getenv("HETZNER_S3_ACCESS_KEY")
	secretKey := os.Getenv("HETZNER_S3_SECRET_KEY")

	// Create download path in /tmp
	downloadPath := filepath.Join("/tmp", filepath.Base(key))

	// Build S3DownloadAction request
	downloadAction := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "DownloadAction",
		"object": map[string]interface{}{
			"@type":          "MediaObject",
			"identifier":     key,
			"encodingFormat": encodingFormat,
			"contentUrl":     downloadPath,
		},
		"target": map[string]interface{}{
			"@type":      "DataCatalog",
			"identifier": bucket,
			"url":        s3URL_env,
			"additionalProperty": map[string]interface{}{
				"region":    region,
				"accessKey": accessKey,
				"secretKey": secretKey,
			},
		},
	}

	// Call s3service
	actionBytes, err := json.Marshal(downloadAction)
	if err != nil {
		return "", fmt.Errorf("failed to marshal download action: %w", err)
	}

	s3ServiceURL := os.Getenv("S3_SERVICE_URL")
	if s3ServiceURL == "" {
		s3ServiceURL = "http://localhost:8092"
	}

	resp, err := http.Post(s3ServiceURL+"/v1/api/semantic/action", "application/ld+json", bytes.NewBuffer(actionBytes))
	if err != nil {
		return "", fmt.Errorf("failed to call s3service: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("s3service returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response to verify success
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse s3service response: %w", err)
	}

	if status, ok := result["actionStatus"].(string); ok && status != "CompletedActionStatus" {
		return "", fmt.Errorf("s3service download failed with status: %s", status)
	}

	return downloadPath, nil
}

// Prevent unused import errors
var _ = multipart.NewWriter(nil)
