package main

import (
	"context"
	"strings"
	"testing"
)

func TestMigrateRefusesWithoutTheOwnersURL(t *testing.T) {
	err := migrate(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "RATINGS_DATABASE_URL") {
		t.Errorf("migrate without a URL = %v, want a refusal naming RATINGS_DATABASE_URL", err)
	}
}
