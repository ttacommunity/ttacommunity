-- The four majors' format and regulations, which the site kept in its code,
-- become their admin-edited regulations: one bullet per line, as every
-- tournament's now are. One already written in admin is kept.

-- +goose Up
INSERT INTO tournament_regulations (tournament, text) VALUES
    ($r$international$r$, $r$7 players per division playing a standard seasonal schedule of 7 matches (4 four-player games and 3 three-player games).
Scoring System: 4-Player games award 6/3/1/0 points; 3-Player games award 5/2/0 points (ties split points equally).
Theoretical Seasonal Maximum: (4 x 6) + (3 x 5) = 39.0 points.
Divisional pyramid: Grandmaster, Master (1 & 2), Platinum (1-4), Gold (1-8), Silver (1-14), Bronze (1-14), Wood (1-10).
Top division crowns the official International Champion each season.$r$),
    ($r$intermezzo$r$, $r$5 players per division in a round-robin schedule totaling 10 matches per division.
Each player competes in exactly 6 games per season (3-player tables paired with every pair of division rivals).
Scoring System: 1st Place = 5 pts, 2nd Place = 2 pts, 3rd Place = 0 pts (ties split points equally).
Theoretical Seasonal Maximum: 6 games x 5.0 pts = 30.0 points.
Multi-divisional ladder structure with Master 1 / Diamond as the highest tier.$r$),
    ($r$royal_league$r$, $r$2-player head-to-head matches exclusively under medium async with 48h reserve per Age.
8 players per division playing 7 weekly match rounds against all group opponents (28 matches total; max 14.0 pts).
Points: 2 for win, 1 for draw, 0 for loss.
Tiebreaker Protocol: 1st tiebreaker is Head-to-Head (H2H) match results between tied players. If unresolved for critical promotion, relegation, or podium finishes, official staged playoff tiebreaker tournaments are run.
Fibonacci-pyramidal divisional hierarchy: Emperor, King, Prince, Duke, Marquess, Count, Viscount, Baron.$r$),
    ($r$worlds$r$, $r$Annual world championship staged across multi-round qualifying and playoff brackets.
Historically features progressive group stages (Stage 1 & 2) and upper/lower elimination playoff brackets (Stage 3 & 4).
Format regulations evolve over time with special structures planned for upcoming editions.
Winner receives the permanent World Champion title badge and exclusive Crown WC distinction.$r$)
ON CONFLICT (tournament) DO NOTHING;

-- +goose Down
-- irreversible: an admin may have edited these since; leaving them is harmless.
