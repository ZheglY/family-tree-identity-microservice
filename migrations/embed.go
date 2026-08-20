package migrations

import "embed"

// Files contains every versioned SQL migration compiled into the migration
// command, so deployment does not depend on the current working directory.
//
//go:embed *.sql
var Files embed.FS
