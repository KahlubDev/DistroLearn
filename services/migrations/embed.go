// Package migrations embeds the SQL migration files so they travel inside the service
// binaries, per ADR 0005. One artifact per service, and no migration step in the deploy
// pipeline that can drift from the code.
package migrations

import "embed"

// FS holds the numbered .sql files. goose reads them through this rather than from disk,
// so a container needs no migrations directory at runtime.
//
//go:embed *.sql
var FS embed.FS
