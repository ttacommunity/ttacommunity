// Package ratingdelta repeats the ratings site's estimate of what one game is
// worth to a player: a single Glicko-2 update from pre-month baseline ratings.
// Update copies the engine's math; testdata pins it to it.
package ratingdelta

import (
	"math"

	"ttacommunity/backend/internal/round"
)

const (
	defaultRating = 1500.0
	scale         = 173.7178
	maxRD         = 350.0
	minRD         = 20.0

	// What the player page uses for every game, whatever the model.
	pageSigma = 0.03
	pageTau   = 0.3
)

// goldenWeights are GlickoD's weights for a game by player count.
var goldenWeights = map[int]float64{2: 1, 3: math.Phi / 2, 4: math.Phi * math.Phi / 3}

// Rating is a Glicko-2 rating on the display scale.
type Rating struct {
	Rating, RD, Sigma float64
}

// Game is one opponent faced in a rating update.
type Game struct {
	Opponent    Rating
	Outcome     float64
	Weight      float64
	PlayerCount int
}

// Seat is one player's score in a game.
type Seat struct {
	Name  string
	Score float64
}

func (r Rating) mu() float64  { return (r.Rating - defaultRating) / scale }
func (r Rating) phi() float64 { return r.RD / scale }

func g(phi float64) float64 { return 1 / math.Sqrt(1+3*phi*phi/(math.Pi*math.Pi)) }

func expected(mu, muJ, phiJ float64) float64 { return 1 / (1 + math.Exp(-g(phiJ)*(mu-muJ))) }

func inactive(r Rating) Rating {
	phi := math.Sqrt(r.phi()*r.phi() + r.Sigma*r.Sigma)
	return Rating{Rating: r.Rating, RD: math.Min(phi*scale, maxRD), Sigma: r.Sigma}
}

// Update returns player's rating after games, as the engine's update_rating.
// Weights count only when weighted is true.
func Update(player Rating, games []Game, tau float64, weighted bool) Rating {
	return update(player, games, tau, func(mu float64, gm Game) (float64, float64) {
		w := 1.0
		if weighted {
			w = gm.Weight
		}
		return w, expected(mu, gm.Opponent.mu(), gm.Opponent.phi())
	})
}

// update is the Glicko-2 step, as the engine's update_rating.
func update(player Rating, games []Game, tau float64, term func(mu float64, gm Game) (w, e float64)) Rating {
	if len(games) == 0 {
		return inactive(player)
	}
	mu, phi, sigma := player.mu(), player.phi(), player.Sigma

	var vInv, deltaSum float64
	for _, gm := range games {
		w, e := term(mu, gm)
		gv := g(gm.Opponent.phi())
		vInv += w * gv * gv * e * (1 - e)
		deltaSum += w * gv * (gm.Outcome - e)
	}
	if vInv <= 0 {
		return inactive(player)
	}
	v := 1 / vInv
	delta := v * deltaSum

	a := math.Log(sigma * sigma)
	f := func(x float64) float64 {
		ex := math.Exp(x)
		d := phi*phi + v + ex
		return ex*(delta*delta-phi*phi-v-ex)/(2*d*d) - (x-a)/(tau*tau)
	}
	A := a
	var B float64
	if delta*delta > phi*phi+v {
		B = math.Log(delta*delta - phi*phi - v)
	} else {
		k := 1.0
		for f(a-k*tau) < 0 {
			k++
		}
		B = a - k*tau
	}
	fA, fB := f(A), f(B)
	for i := 0; math.Abs(B-A) > 1e-6 && i < 100; i++ {
		C := A + (A-B)*fA/(fB-fA)
		fC := f(C)
		if fC*fB < 0 {
			A, fA = B, fB
		} else {
			fA /= 2
		}
		B, fB = C, fC
	}

	newSigma := math.Exp(B / 2)
	phiStar := math.Sqrt(phi*phi + newSigma*newSigma)
	newPhi := 1 / math.Sqrt(1/(phiStar*phiStar)+1/v)
	newMu := mu + newPhi*newPhi*deltaSum
	rd := math.Max(newPhi*scale, minRD)
	return Rating{Rating: newMu*scale + defaultRating, RD: rd, Sigma: newSigma}
}

// Delta returns what one game would change me's rating by from pre-month baseline ratings,
// rounded to 0.1 as the page shows it, with ratings from rating (unrated players count as
// 1500 ± 350). Weights follow the match's playerCount: GlickoD's golden weights, none for
// standard Glicko-2. It returns false for a game me did not play against anyone.
func Delta(engine, me string, playerCount int, seats []Seat, rating func(name string) (Rating, bool)) (float64, bool) {
	at := func(name string) Rating {
		r, ok := rating(name)
		if !ok {
			r = Rating{Rating: defaultRating, RD: maxRD}
		}
		r.Sigma = pageSigma
		return r
	}
	var mine *Seat
	for i := range seats {
		if seats[i].Name == me {
			mine = &seats[i]
		}
	}
	if mine == nil || len(seats) < 2 || playerCount < 2 {
		return 0, false
	}

	weight := 1.0
	if engine != "glicko2_std" {
		engine = "glicko2_daneo"
		if w, ok := goldenWeights[playerCount]; ok {
			weight = w
		}
	}
	games := make([]Game, 0, len(seats)-1)
	for _, s := range seats {
		if s.Name == me {
			continue
		}
		outcome := 0.5
		if mine.Score > s.Score {
			outcome = 1
		} else if mine.Score < s.Score {
			outcome = 0
		}
		games = append(games, Game{Opponent: at(s.Name), Outcome: outcome, Weight: weight, PlayerCount: playerCount})
	}

	before := at(me)
	after := Update(before, games, pageTau, engine != "glicko2_std")
	return round.Tenth(after.Rating - before.Rating), true
}
