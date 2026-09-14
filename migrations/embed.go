package migrations

import "embed"

// FS contains versioned SQL migrations embedded into the application binary.
//
//go:embed versions/*.sql
var FS embed.FS
