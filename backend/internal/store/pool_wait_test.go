package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// refusedThen is a ping refused n times, then answering err.
func refusedThen(n int, err error) func(context.Context) error {
	return func(context.Context) error {
		if n > 0 {
			n--
			return fmt.Errorf("dial: %w", syscall.ECONNREFUSED)
		}
		return err
	}
}

func TestWaitForConnection(t *testing.T) {
	other := errors.New("password authentication failed")
	tests := []struct {
		name      string
		ping      func(context.Context) error
		wantWaits []time.Duration
		wantErr   error
	}{
		{"connects at once", refusedThen(0, nil), nil, nil},
		{"refused while the network admits the pod", refusedThen(3, nil),
			[]time.Duration{250 * time.Millisecond, 500 * time.Millisecond, time.Second}, nil},
		{"another error fails at once", refusedThen(0, other), nil, other},
		{"refused throughout", refusedThen(100, nil),
			[]time.Duration{250 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second, 2250 * time.Millisecond},
			syscall.ECONNREFUSED},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var waits []time.Duration
			sleep := func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
			err := waitForConnection(context.Background(), tt.ping, sleep)
			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Errorf("waitForConnection = %v, want %v", err, tt.wantErr)
			}
			if !slices.Equal(waits, tt.wantWaits) {
				t.Errorf("waits = %v, want %v", waits, tt.wantWaits)
			}
		})
	}
}

func TestWaitForConnectionStopsWithItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sleep := func(context.Context, time.Duration) error { cancel(); return context.Canceled }
	if err := waitForConnection(ctx, refusedThen(100, nil), sleep); !errors.Is(err, context.Canceled) {
		t.Errorf("waitForConnection = %v, want it to stop with its context", err)
	}
}

func TestARefusedDialIsTakenForRefused(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=2")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(context.Background()); !refused(err) {
		t.Errorf("a ping to a closed port = %v, want it taken for refused", err)
	}
}
