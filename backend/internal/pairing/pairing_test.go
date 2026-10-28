package pairing_test

import (
	"errors"
	"testing"

	"ttacommunity/backend/internal/pairing"
)

func TestPairRefusesWhatNoMethodCanPair(t *testing.T) {
	everyoneMet := meet(field(0, 0, 0, 0), [2]int64{1, 2}, [2]int64{1, 3}, [2]int64{1, 4}, [2]int64{2, 3}, [2]int64{2, 4}, [2]int64{3, 4})
	tests := []struct {
		name    string
		players []pairing.Player
		want    error
	}{
		{"one player", field(0), pairing.ErrTooFew},
		{"four players who have all met", everyoneMet, pairing.ErrNoPairing},
	}
	for _, m := range pairing.Methods {
		for _, tt := range tests {
			t.Run(string(m)+"/"+tt.name, func(t *testing.T) {
				if got, err := pairing.Pair(tt.players, m, pairing.ByRating, 1); !errors.Is(err, tt.want) {
					t.Errorf("Pair = %v, %v; want %v", pairs(got), err, tt.want)
				}
			})
		}
	}
}

func TestPairRefusesAnUnknownMethod(t *testing.T) {
	if _, err := pairing.Pair(field(0, 0), "swiss-ish", pairing.ByRating, 1); !errors.Is(err, pairing.ErrUnknownMethod) {
		t.Errorf("Pair(swiss-ish) = %v, want ErrUnknownMethod", err)
	}
}
