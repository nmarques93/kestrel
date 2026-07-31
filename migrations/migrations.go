// Package migrations embeds Kestrel's goose migration files so they can be
// applied from a compiled binary without shipping the .sql files separately.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
