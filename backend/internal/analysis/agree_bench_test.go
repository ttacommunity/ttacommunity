package analysis

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"testing"

	"ttacommunity/backend/internal/store"
)

// syntheticRatings rates n players in every page model, each with 20 opponents.
func syntheticRatings(n int) []store.ModelRating {
	rng := rand.New(rand.NewPCG(1, 2))
	var out []store.ModelRating
	for i := range n {
		for _, m := range store.PlayerPageModels {
			out = append(out, store.ModelRating{Player: fmt.Sprintf("player%d", i), Model: m, Rating: 1200 + rng.Float64()*800, Opponents: 20})
		}
	}
	return out
}

// BenchmarkAgree sizes the sample like a real run's, where every rated player counts.
func BenchmarkAgree(b *testing.B) {
	for _, n := range []int{500, 1000, 3200} {
		ratings := syntheticRatings(n)
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			for b.Loop() {
				Agree(ratings)
			}
		})
	}
}
