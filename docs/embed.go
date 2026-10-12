// Package docs embeds the OpenAPI specification, its JSON form, the Swagger UI
// page, and the vendored Swagger UI assets so the running API can serve its own
// contract — offline, with no build-time tooling.
//
// The spec is the source of truth for the HTTP API: backend/internal/httpapi tests fail
// if a registered route is missing from it (or vice versa), and validate real
// handler responses against its schemas. openapi.json is generated from
// openapi.yaml (`make openapi-json`) and a test fails if the two drift.
package docs

import "embed"

// FS holds the embedded API documentation:
//
//	openapi.yaml — the OpenAPI 3.0 document, served at /openapi.yaml
//	openapi.json — the same document as JSON, served at /openapi.json
//	docs.html    — a Swagger UI page, served at /docs
//	swagger/     — the vendored Swagger UI assets, served at /swagger/
//
//go:embed openapi.yaml openapi.json docs.html swagger
var FS embed.FS
