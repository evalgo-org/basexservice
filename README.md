# BaseX Semantic Service

Semantic orchestration service for BaseX XML database operations using Schema.org JSON-LD vocabulary.

## Overview

basexservice provides a semantic API for orchestrating BaseX XML database operations through Schema.org actions. It enables When (task orchestrator) to manage XSLT transformations, XQuery execution, and XML document management using pure JSON-LD workflows.

## Architecture

```
When (Orchestrator) → Schema.org JSON-LD workflows
  ↓ HTTP POST with ScheduledAction
basexservice → /v1/api/semantic/action
  ↓ TransformAction, QueryAction, UploadAction
BaseX (XML Database/XSLT Processor)
```

## Features

✅ **Schema.org Semantic API**: Full JSON-LD compliance
✅ **XSLT Transformations**: TransformAction for XML processing
✅ **XQuery Execution**: QueryAction for database queries
✅ **File Management**: UploadAction for XML/XSLT uploads
✅ **Database Operations**: Create/Delete databases
✅ **When Integration**: Schedule workflows via When
✅ **IQS Pipeline Support**: SPARQL → XSLT → Cache workflows

## Installation

### Build from Source

```bash
cd /home/opunix/basexservice
go build -o basexservice ./cmd/
```

### Run

```bash
# Set environment variables
export BASEX_API_KEY="your-api-key-here"
export PORT=8090

# Start service
./basexservice
```

## API Endpoints

### Health Check

```bash
GET /health
```

### Semantic Action Endpoint

```bash
POST /v1/api/semantic/action
Content-Type: application/json
x-api-key: your-api-key
```

## Supported Actions

### 1. TransformAction (UpdateAction)

XSLT transformation of XML documents.

**Example**:
```json
{
  "@context": "https://schema.org",
  "@type": "UpdateAction",
  "identifier": "transform-xml",
  "name": "Transform XML with XSLT",
  "object": {
    "@type": "Dataset",
    "identifier": "source-xml",
    "contentUrl": "/path/to/source.xml",
    "encodingFormat": "application/xml"
  },
  "instrument": {
    "@type": "SoftwareSourceCode",
    "identifier": "transformation-xslt",
    "contentUrl": "/path/to/transform.xsl",
    "programmingLanguage": "XSLT"
  },
  "target": {
    "@type": "DataCatalog",
    "identifier": "IQS",
    "url": "http://localhost:8080",
    "additionalProperty": {
      "username": "admin",
      "password": "password"
    }
  }
}
```

### 2. QueryAction (SearchAction)

Execute XQuery against BaseX database.

**Example**:
```json
{
  "@context": "https://schema.org",
  "@type": "SearchAction",
  "identifier": "query-database",
  "name": "Execute XQuery",
  "query": "for $x in //document return $x/title",
  "target": {
    "@type": "DataCatalog",
    "identifier": "IQS",
    "url": "http://localhost:8080",
    "additionalProperty": {
      "username": "admin",
      "password": "password"
    }
  }
}
```

### 3. BaseXUploadAction (UploadAction)

Upload XML or XSLT files to BaseX database.

**Example**:
```json
{
  "@context": "https://schema.org",
  "@type": "UploadAction",
  "identifier": "upload-xslt",
  "name": "Upload XSLT Stylesheet",
  "object": {
    "@type": "Dataset",
    "identifier": "stylesheet",
    "contentUrl": "/path/to/stylesheet.xsl",
    "encodingFormat": "text/xsl"
  },
  "target": {
    "@type": "DataCatalog",
    "identifier": "IQS",
    "url": "http://localhost:8080",
    "additionalProperty": {
      "username": "admin",
      "password": "password"
    }
  },
  "targetUrl": "stylesheets/transform.xsl"
}
```

### 4. CreateDatabaseAction (CreateAction)

Create a new BaseX database.

**Example**:
```json
{
  "@context": "https://schema.org",
  "@type": "CreateAction",
  "identifier": "create-db",
  "name": "Create New Database",
  "result": {
    "@type": "DataCatalog",
    "identifier": "NewDB",
    "url": "http://localhost:8080",
    "additionalProperty": {
      "username": "admin",
      "password": "password"
    }
  }
}
```

## When Integration

basexservice is designed to be orchestrated by When. Example workflows are in `examples/workflows/`.

### Submit Workflow to When

```bash
curl -X POST http://localhost:3000/api/workflows/create \
  -H "Content-Type: application/json" \
  -d @examples/workflows/01-concept-schemes.json
```

## IQS Pipeline Examples

### Concept Schemes Workflow

See `examples/workflows/01-concept-schemes.json`

- Queries SPARQL endpoint for concept schemes
- Transforms RDF/XML with XSLT
- Stores result in BaseX

### Empolis JSON Pipeline

See `examples/workflows/02-empolis-json.json`

Sequential pipeline:
1. Upload XSLT stylesheets to BaseX
2. Transform SPARQL results with XSLT
3. Generate Empolis JSON format

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `BASEX_API_KEY` | API key for authentication | (none, allows all) |
| `PORT` | HTTP server port | `8090` |
| `BASEX_URL` | BaseX REST API URL | (per request) |
| `BASEX_USER` | BaseX username | (per request) |
| `BASEX_PASSWORD` | BaseX password | (per request) |

## BaseX REST API Compatibility

basexservice uses the BaseX REST API:

- `PUT /rest/{db}/{resource}` - Upload file
- `POST /rest/{db}` - Execute XQuery
- `PUT /rest/{db}` - Create database
- `DELETE /rest/{db}` - Delete database

See [BaseX REST Documentation](https://docs.basex.org/wiki/REST) for details.

## Schema.org Types Used

### Databases and Documents

- **DataCatalog**: BaseX database
- **Dataset**: XML documents
- **SoftwareSourceCode**: XSLT stylesheets

### Actions

- **UpdateAction**: XSLT transformations (TransformAction)
- **SearchAction**: XQuery queries (QueryAction)
- **UploadAction**: File uploads (BaseXUploadAction)
- **CreateAction**: Database creation (CreateDatabaseAction)
- **DeleteAction**: Database/document deletion (DeleteDatabaseAction)

## Complete Semantic Stack

```
┌─────────────────────────────────────────┐
│ When (Semantic Task Orchestrator)      │
│ - Schedule workflows                    │
│ - Manage dependencies                   │
│ - Parallel/sequential execution         │
└──────────────┬──────────────────────────┘
               │
               ├──────────────────────────────┐
               ▼                              ▼
┌──────────────────────┐   ┌──────────────────────┐
│ basexservice         │   │ pxgraphservice       │
│ (BaseX/XML)          │   │ (GraphDB)            │
│                      │   │                      │
│ POST /v1/api/        │   │ POST /v1/api/        │
│      semantic/action │   │      semantic/action │
│                      │   │                      │
│ - TransformAction    │   │ - TransferAction     │
│ - QueryAction        │   │ - CreateAction       │
│ - UploadAction       │   │                      │
└──────────────┬───────┘   └──────────────┬───────┘
               ▼                          ▼
    ┌──────────────────┐      ┌──────────────────┐
    │ BaseX            │      │ GraphDB          │
    │ (XML Database)   │      │ (RDF Store)      │
    └──────────────────┘      └──────────────────┘
```

## Development

### Project Structure

```
basexservice/
├── cmd/
│   ├── main.go           # HTTP server
│   └── semantic_api.go   # Semantic action handlers
├── examples/
│   └── workflows/        # When workflow examples
├── docs/                 # Documentation
├── go.mod
└── README.md
```

### Dependencies

- `eve.evalgo.org@v0.0.16` - Semantic types
- `github.com/labstack/echo/v4` - HTTP framework

## Testing

```bash
# Test health endpoint
curl http://localhost:8090/health

# Test semantic action
curl -X POST http://localhost:8090/v1/api/semantic/action \
  -H "Content-Type: application/json" \
  -H "x-api-key: your-key" \
  -d @examples/workflows/01-concept-schemes.json
```

## Migration from IQS http-orchestrator

The basexservice replaces the IQS http-orchestrator CLI tool:

**Before** (http-orchestrator):
```bash
./http-orchestrator --output-response concept-schemes \
  --config config.yaml
```

**After** (basexservice + When):
```json
{
  "@type": "ScheduledAction",
  "additionalProperty": {
    "url": "http://localhost:8090/v1/api/semantic/action",
    "body": { "@type": "UpdateAction", ... }
  }
}
```

Benefits:
- Schedulable via When
- Semantic/machine-readable
- Dependency management
- Parallel execution
- Reusable across projects

## License

Proprietary

## See Also

- [When](https://github.com/your-org/when) - Semantic task orchestrator
- [EVE](https://github.com/evalgo-org/eve) - Semantic type library
- [pxgraphservice](../pxgraphservice) - GraphDB semantic service
- [BaseX Documentation](https://docs.basex.org/)
