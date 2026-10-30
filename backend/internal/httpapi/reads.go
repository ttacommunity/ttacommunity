// What the public routes read: Ratings, made of each route file's reads.
package httpapi

import (
	"context"
	"time"

	"ttacommunity/backend/internal/challenges"
	"ttacommunity/backend/internal/store"
)

// Ratings is what the public ratings routes read: each route file's reads, together.
type Ratings interface {
	RunReads
	BoardReads
	PlayerReads
	YearReads
	AnalysisReads
	TournamentReads
	CatalogueReads
	BlitzReads
	ChallengeReads
}

// ChallengeReads are challenges.go's reads: the challenges, every entry not hidden, and the weekly challenge.
type ChallengeReads interface {
	Challenges(ctx context.Context) ([]store.ChallengeRow, error)
	ChallengeEntries(ctx context.Context) ([]store.ChallengeEntry, error)
	ChallengeWeeks(ctx context.Context) ([]challenges.Week, error)
}

// BlitzReads are the site's blitz tournament reads: the shown editions, and one with its active entrants.
type BlitzReads interface {
	BlitzEditions(ctx context.Context) ([]store.BlitzEditionRow, error)
	BlitzEdition(ctx context.Context, slug, edition string) (store.BlitzEdition, error)
}

// RunReads are the reads every area shares: whether the database answers, the run the site shows, a run's last game.
type RunReads interface {
	Ping(ctx context.Context) error
	CurrentRun(ctx context.Context) (store.PublishedRun, error)
	RunLastGame(ctx context.Context, run int64) (*time.Time, error)
}

// BoardReads are the leaderboard's reads, here and in csv.go.
type BoardReads interface {
	Leaderboard(ctx context.Context, run int64, model string, format int) ([]store.LeaderboardRow, error)
	History(ctx context.Context, run int64, model string, format int, at *time.Time) (time.Time, []store.HistoryRow, error)
}

// PlayerReads are player.go's reads.
type PlayerReads interface {
	Player(ctx context.Context, run int64, name string) (store.Player, error)
	PlayerHistory(ctx context.Context, run int64, name string, format int) (string, []store.HistoryPoint, error)
	PlayerMatches(ctx context.Context, run int64, name string, format, page int) (string, store.MatchPage, error)
	RatingsOf(ctx context.Context, run int64, model string, format int, names []string) (map[string]store.Rated, error)
	HistoricalRatingsOf(ctx context.Context, run int64, model string, format int, at []store.PlayerAt) (map[store.PlayerAt]store.Rated, error)
	HiddenNames(ctx context.Context, names []string) (map[string]bool, error)
	PlayerRivals(ctx context.Context, run int64, name string, format int) (string, []store.Rival, error)
	PlayerCareer(ctx context.Context, run int64, name string, format int, model string) (string, []store.CareerYear, *store.CareerPeak, error)
	Versus(ctx context.Context, run int64, name, other string) (string, string, []store.PlayerMatch, error)
	PlayerMatrix(ctx context.Context, run int64, name string, format int, year *int) (string, []store.MatrixCell, error)
	PlayerSeats(ctx context.Context, run int64, name string, unrated bool) (store.PlayerSeatStats, error)
}

// YearReads are year.go's reads, and csv.go's for one year.
type YearReads interface {
	YearBoard(ctx context.Context, run int64, year int, model string, format int) (store.YearBoard, error)
	Years(ctx context.Context, run int64) ([]store.Year, error)
}

// AnalysisReads are analysis.go's reads.
type AnalysisReads interface {
	Calibration(ctx context.Context, run int64) ([]store.CalibrationRow, error)
	CheckPublished(ctx context.Context, run int64) error
	AnalysisRatings(ctx context.Context, run int64, format int) ([]store.ModelRating, error)
	ActivePlayers(ctx context.Context, run int64) (map[string]bool, error)
	ComparedPlayers(ctx context.Context, run int64, format int, terms []string) ([]store.ComparedPlayer, error)
	PlayerNamesLike(ctx context.Context, run int64, term string, limit int) ([]string, error)
	Activity(ctx context.Context, run int64, inactiveOnOrBefore time.Time) (store.Activity, error)
	SeatAnalysis(ctx context.Context, run int64, format int, tier string) ([]store.SeatStatsGroup, error)
}

// TournamentReads are tournament.go's reads.
type TournamentReads interface {
	TournamentSeries(ctx context.Context, run int64, series string) (store.TournamentSeries, error)
	HallOfFame(ctx context.Context, run int64) (store.HallOfFame, error)
	PlayerAchievements(ctx context.Context, run int64, name string) (string, *store.AchievementSummary, []store.TournamentRecord, error)
	CommunityLeaderboard(ctx context.Context, run int64, edition string) ([]store.CommunityRow, error)
	CommunityLeaderboardLayout(ctx context.Context, run int64, slug string) (store.CommunityEditionLayout, error)
	CommunityEditions(ctx context.Context) ([]store.CommunityEdition, error)
}

// CatalogueReads are catalogue.go's reads.
type CatalogueReads interface {
	Catalogue(ctx context.Context, run int64) (store.CatalogueView, error)
	CatalogueTournament(ctx context.Context, run int64, slug string) (store.CatalogueTournament, error)
	CatalogueGames(ctx context.Context, run int64, slug string, f store.CatalogueGamesFilter) (store.CatalogueGames, error)
	RecentPodiums(ctx context.Context, run int64, played, shown int) ([]store.RecentPodium, error)
}
