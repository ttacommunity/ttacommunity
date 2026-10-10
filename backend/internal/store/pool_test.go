//go:build integration

package store

import (
	"context"
	"testing"
)

func TestOpenPoolSetsTheApisPlannerSettings(t *testing.T) {
	ctx := context.Background()
	pool, err := OpenPool(ctx, migrated(t).Config().ConnString())
	if err != nil {
		t.Fatalf("OpenPool: %v", err)
	}
	defer pool.Close()
	tests := []struct {
		setting, want string
	}{
		{"jit", "off"},
		{"plan_cache_mode", "force_custom_plan"},
	}
	for _, tt := range tests {
		t.Run(tt.setting, func(t *testing.T) {
			var got string
			if err := pool.QueryRow(ctx, "SHOW "+tt.setting).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("%s = %q, want %q", tt.setting, got, tt.want)
			}
		})
	}
}
