// Package migrations embeds the SQL schema so a single static binary can
// migrate its own database (no migration files needed in the Railway image).
package migrations

import "embed"

//go:embed *.up.sql
var FS embed.FS
