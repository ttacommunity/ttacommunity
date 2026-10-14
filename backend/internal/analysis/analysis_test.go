package analysis

import (
	"fmt"
	"slices"
	"testing"

	"ttacommunity/backend/internal/store"
)

// fixture is five players rated in all ten page models, each model shifted a
// little so they disagree, plus two WHR-only players at the histogram's edges.
// The expected numbers were computed with numpy, as upstream computes them.
func fixture() []store.ModelRating {
	var rows []store.ModelRating
	for i, m := range store.PlayerPageModels {
		ratings := make([]float64, 5)
		for p := range 5 {
			ratings[p] = float64(1400 + p*120 + (p*37*(i+1))%211)
		}
		for p := range 5 {
			rank := 1
			for q := range 5 {
				if ratings[q] > ratings[p] {
					rank++
				}
			}
			rows = append(rows, store.ModelRating{Player: fmt.Sprint("p", p), Model: m, Rating: ratings[p], Rank: &rank, Opponents: 20})
		}
	}
	rows = append(rows,
		store.ModelRating{Player: "edge", Model: "whr", Rating: 2100, Opponents: 20},
		store.ModelRating{Player: "below", Model: "whr", Rating: 999, Opponents: 20})
	return rows
}

func TestDistributeCountsAndAveragesAsNumpy(t *testing.T) {
	got := Distribute(fixture(), map[string]bool{"p1": true, "p3": true})
	if len(got.Labels) != 11 || got.Labels[0] != "1000-1099" || got.Labels[10] != "2000-2099" {
		t.Errorf("labels = %v", got.Labels)
	}
	want := map[string]struct {
		counts       []int
		mean, active float64
	}{
		"glicko2_daneo": {[]int{0, 0, 0, 0, 1, 1, 0, 1, 1, 0, 1}, 1714.0, 1714.0},
		"glicko2_std":   {[]int{0, 0, 0, 0, 1, 0, 2, 0, 1, 1, 0}, 1693.2, 1756.5},
		"glicko2_mp":    {[]int{0, 0, 0, 0, 1, 0, 0, 2, 1, 1, 0}, 1756.8, 1799.0},
		"glicko2_adapt": {[]int{0, 0, 0, 0, 1, 1, 0, 1, 0, 1, 1}, 1736.0, 1736.0},
		"whr":           {[]int{0, 0, 0, 0, 1, 0, 2, 0, 0, 2, 1}, 1667.9, 1778.5},
	}
	for m, w := range want {
		if !slices.Equal(got.Counts[m], w.counts) || got.Means[m] != w.mean || got.ActiveMeans[m] != w.active {
			t.Errorf("%s: counts %v, mean %v, active mean %v; want %v, %v, %v",
				m, got.Counts[m], got.Means[m], got.ActiveMeans[m], w.counts, w.mean, w.active)
		}
	}
}

func TestDistributeDefaultsTo1500WithoutRatings(t *testing.T) {
	got := Distribute(nil, nil)
	if got.Means["whr"] != 1500 || got.ActiveMeans["glicko2_std"] != 1500 || !slices.Equal(got.Counts["whr"], make([]int, 11)) {
		t.Errorf("empty distribution = %+v", got)
	}
}

func TestAgreeRanksPairsAsUpstream(t *testing.T) {
	got := Agree(fixture())
	if got.Sample != 5 {
		t.Errorf("sample = %d, want the 5 players rated in every model", got.Sample)
	}
	top := []Pair{
		{"glicko2_daneo", "glicko2_std", 0.9601, 1.0}, {"glicko2_daneo", "glicko2_std_softer", 0.9582, 1.0},
		{"glicko2_daneo", "glicko2_mp", 0.9552, 1.0}, {"glicko2_daneo", "glicko2_mp_softer", 1.0, 1.0},
	}
	weak := []Pair{
		{"glicko2_mp_softer", "glicko2_adapt_softer", 0.9683, 0.9}, {"glicko2_adapt", "glicko2_adapt_softer", 0.9683, 0.9},
		{"glicko2_adapt_softer", "whr", 0.9138, 0.9}, {"glicko2_adapt_softer", "whr_softer", 0.9558, 0.9},
	}
	if !slices.Equal(got.Top, top) || !slices.Equal(got.Weakest, weak) {
		t.Errorf("top %v\nwant %v\nweakest %v\nwant %v", got.Top, top, got.Weakest, weak)
	}
}

func TestAgreeLeavesOutPlayersUnderFifteenOpponentsInAnyModel(t *testing.T) {
	rows := fixture()
	rows[0].Opponents = 14 // p0 in GlickoD
	if got := Agree(rows); got.Sample != 4 {
		t.Errorf("sample = %d, want 4 once p0 has 14 opponents in one model", got.Sample)
	}
	if got := Agree(rows[:5]); got.Sample != 0 || len(got.Top) != 0 {
		t.Errorf("one model only: %+v, want no one rated in all ten", got)
	}
}

func TestAgreeRanksWithinSampleRegardlessOfStoredRank(t *testing.T) {
	// Every model orders a, b, c alike; whr's stored ranks have gaps, as when players outside the sample sit between.
	var rows []store.ModelRating
	for _, m := range store.PlayerPageModels {
		for i, p := range []string{"a", "b", "c"} {
			rank := i + 1
			if m == "whr" {
				rank = 1 + 4*i
			}
			rows = append(rows, store.ModelRating{Player: p, Model: m, Rating: 1600 - 50*float64(i), Rank: &rank, Opponents: 20})
		}
	}
	got := Agree(rows)
	for _, pair := range slices.Concat(got.Top, got.Weakest) {
		if pair.Spearman != 1 {
			t.Errorf("%s v %s Spearman = %v, want 1 for models that order the sample alike", pair.A, pair.B, pair.Spearman)
		}
	}
}

func TestAgreeTiedRatingsShareTheirRank(t *testing.T) {
	// Three players: a and b tie in every model but one, where c moves between them.
	var rows []store.ModelRating
	for _, m := range store.PlayerPageModels {
		ratings := map[string]float64{"a": 1600, "b": 1600, "c": 1500}
		if m == "whr" {
			ratings = map[string]float64{"a": 1600, "b": 1500, "c": 1550}
		}
		for p, r := range ratings {
			rows = append(rows, store.ModelRating{Player: p, Model: m, Rating: r, Opponents: 20})
		}
	}
	got := Agree(rows)
	// Ranks 1, 1, 3 against 1, 3, 2: d² = 0 + 4 + 1 = 5, so Spearman is 1 - 6·5/(3·8) = -0.25.
	if len(got.Weakest) == 0 || got.Weakest[len(got.Weakest)-1].Spearman != -0.25 {
		t.Errorf("weakest = %+v, want the pair with whr at Spearman -0.25 (ties sharing rank 1)", got.Weakest)
	}
	if got.Top[0].Spearman != 1 {
		t.Errorf("top = %+v, want models with identical ranks at Spearman 1", got.Top)
	}
}
