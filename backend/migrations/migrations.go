// Package migrations embeds the SQL migrations, so each binary carries them.
package migrations

import "embed"

// Ratings holds the ratings database's migrations, in ratings/.
//
//go:embed ratings/*.sql
var Ratings embed.FS
