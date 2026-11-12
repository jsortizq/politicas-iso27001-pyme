// Package migrations embeds the versioned SQL migrations so that the
// migrate command works from any directory and inside containers.
package migrations

import "embed"

// FS holds every *.sql migration file in this directory.
//
//go:embed *.sql
var FS embed.FS
