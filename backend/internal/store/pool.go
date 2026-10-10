// Opening the api's connection pools.
package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OpenPool opens a pool for the api's short queries, with Postgres's JIT off: compiling them costs more than it saves
// (a second on admin's tournament list). The engine's long queries keep it. Every query is planned for its own
// values: a reused generic plan cannot see that an optional filter is empty, and reads every game.
func OpenPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("reading the database address: %w", err)
	}
	config.ConnConfig.RuntimeParams["jit"] = "off"
	config.ConnConfig.RuntimeParams["plan_cache_mode"] = "force_custom_plan"
	return pgxpool.NewWithConfig(ctx, config)
}
