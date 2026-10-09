// Opening the api's connection pools.
package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OpenPool opens a pool for the api's short queries, with Postgres's JIT off: compiling them costs more than it saves
// (a second on admin's tournament list). The engine's long queries keep it.
func OpenPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("reading the database address: %w", err)
	}
	config.ConnConfig.RuntimeParams["jit"] = "off"
	return pgxpool.NewWithConfig(ctx, config)
}
