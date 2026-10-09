//go:build integration

package store

import (
	"context"
	"testing"
)

func TestOpenPoolTurnsJITOff(t *testing.T) {
	ctx := context.Background()
	pool, err := OpenPool(ctx, migrated(t).Config().ConnString())
	if err != nil {
		t.Fatalf("OpenPool: %v", err)
	}
	defer pool.Close()
	var jit string
	if err := pool.QueryRow(ctx, "SHOW jit").Scan(&jit); err != nil {
		t.Fatal(err)
	}
	if jit != "off" {
		t.Errorf("jit = %q, want off", jit)
	}
}
