// Package frontend embeds the built task-queue dashboard so the API binary can
// serve it without any Node toolchain at runtime.
//
// The contents of dist/ are committed (see "Building the dashboard" in the
// README) so that `go build ./...` and CI work on a machine with only Go.
// Rebuild them after changing anything under frontend/src with `make frontend-build`.
package frontend

import "embed"

// Dist holds the built single-page app, rooted so that "dist" is the top-level
// entry; serve it with fs.Sub(Dist, "dist").
//
//go:embed all:dist
var Dist embed.FS
