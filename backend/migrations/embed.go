// Package migrations embeds the SQL migration files that create and evolve the
// task-queue schema.
//
// The .sql files live next to this file so they can be compiled into the
// binaries with go:embed — the API and workers ship as single files with no
// separate migrations directory to install. Files are applied in lexical
// filename order by internal/queue.Migrate.
package migrations

import "embed"

// FS holds every .sql migration in this directory.
//
//go:embed *.sql
var FS embed.FS
