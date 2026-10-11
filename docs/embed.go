// Package docs embeds the OpenAPI specification and the Swagger UI page so the
// running API can serve its own contract without any build-time tooling.
//
// The spec is the source of truth for the HTTP API: internal/httpapi tests fail
// if a registered route is missing from it (or vice versa), and validate real
// handler responses against its schemas.
package docs

import "embed"

// FS holds the embedded API documentation:
//
//	openapi.yaml — the OpenAPI 3.0 document, served at /openapi.yaml
//	docs.html    — a Swagger UI page, served at /docs
//
//go:embed openapi.yaml docs.html
var FS embed.FS
