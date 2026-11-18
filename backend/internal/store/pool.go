// Opening the api's connection pools, and waiting for a new pod's first connection to be let through.
package store

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OpenPool opens a pool for the api's short queries, with Postgres's JIT off: compiling them costs more than it saves
// (a second on admin's tournament list). The engine's long queries keep it. Every query is planned for its own
// values: a reused generic plan cannot see that an optional filter is empty, and reads every game. Times are read in
// UTC whatever the machine's zone, so the api sends UTC instants.
func OpenPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("reading the database address: %w", err)
	}
	config.ConnConfig.RuntimeParams["jit"] = "off"
	config.ConnConfig.RuntimeParams["plan_cache_mode"] = "force_custom_plan"
	config.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		conn.TypeMap().RegisterType(&pgtype.Type{Name: "timestamptz", OID: pgtype.TimestamptzOID, Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC}})
		return nil
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := WaitForDatabase(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// How long a new pod waits for the network to let its first connection through: the first wait, and all of them.
const (
	firstWait     = 250 * time.Millisecond
	waitingAtMost = 10 * time.Second
)

// WaitForDatabase pings pool until it connects. A refused connection is tried again, waiting 250 ms, then twice as
// long each time, for waitingAtMost in all: a new pod's address is let through the network policy about a second
// after it starts. Any other error, or still refused after that, is returned.
func WaitForDatabase(ctx context.Context, pool *pgxpool.Pool) error {
	return waitForConnection(ctx, pool.Ping, func(ctx context.Context, d time.Duration) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
			return nil
		}
	})
}

// waitForConnection calls ping until it answers anything but refused, waiting with sleep between tries as
// WaitForDatabase does.
func waitForConnection(ctx context.Context, ping func(context.Context) error, sleep func(context.Context, time.Duration) error) error {
	wait, waited := firstWait, time.Duration(0)
	for {
		err := ping(ctx)
		if !refused(err) || waited >= waitingAtMost {
			if err != nil {
				return fmt.Errorf("connecting to the database: %w", err)
			}
			return nil
		}
		d := min(wait, waitingAtMost-waited)
		if err := sleep(ctx, d); err != nil {
			return fmt.Errorf("waiting for the database: %w", err)
		}
		waited += d
		wait *= 2
	}
}

// refused says whether err is a connection the other end refused.
func refused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED)
}
