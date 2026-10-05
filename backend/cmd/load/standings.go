package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/store"
)

// checkStandings compares CGE's standings tables with the games held, reports
// the counts, and writes each difference, players named, to file.
func checkStandings(file string) error {
	return withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
		check, err := store.NewAdminStore(pool).CheckStandings(ctx)
		if err != nil {
			return err
		}
		f, err := os.Create(file)
		if err != nil {
			return fmt.Errorf("creating %s: %w", file, err)
		}
		if err := writeDifferences(f, check); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("writing %s: %w", file, err)
		}
		reportStandings(os.Stdout, check, file)
		return nil
	})
}

// reportStandings writes how many players agree, the differences by kind, and
// each stage with one. It names no player.
func reportStandings(w io.Writer, c store.StandingsCheck, file string) {
	players, agree := 0, 0
	kinds := map[string]int{}
	for _, st := range c.Stages {
		players, agree = players+st.Players, agree+st.Agree
		for _, d := range st.Differences {
			kinds[d.Kind]++
		}
	}
	fmt.Fprintf(w, "stages checked: %d (%d only on CGE)\n", len(c.Stages), c.StagesOnlyOnCGE)
	share := 0.0
	if players > 0 {
		share = 100 * float64(agree) / float64(players)
	}
	fmt.Fprintf(w, "players: %d, agree: %d (%.1f%%)\n", players, agree, share)
	fmt.Fprintf(w, "differences:\n")
	names := make([]string, 0, len(kinds))
	for k := range kinds {
		names = append(names, k)
	}
	slices.Sort(names)
	for _, k := range names {
		fmt.Fprintf(w, "  %-25s %d\n", k, kinds[k])
	}
	fmt.Fprintf(w, "stages with a difference:\n")
	for _, st := range c.Stages {
		if len(st.Differences) > 0 {
			fmt.Fprintf(w, "  %d/%d %s: %d of %d agree\n", st.CGETournament, st.CGESeason, st.Name, st.Agree, st.Players)
		}
	}
	fmt.Fprintf(w, "differences written to %s\n", file)
}

// writeDifferences writes every difference as a CSV row, players named.
func writeDifferences(w io.Writer, c store.StandingsCheck) error {
	cw := csv.NewWriter(w)
	cw.Write([]string{"cge_tournament", "cge_season", "name", "cge_user", "player", "kind",
		"cge_games", "held_games", "cge_points", "cge_places", "held_places"})
	for _, st := range c.Stages {
		for _, d := range st.Differences {
			cw.Write([]string{
				strconv.Itoa(st.CGETournament), strconv.Itoa(st.CGESeason), st.Name, strconv.Itoa(d.CGEUser), d.Player, d.Kind,
				strconv.Itoa(d.CGEGames), strconv.Itoa(d.HeldGames), strconv.FormatFloat(d.CGEPoints, 'f', -1, 64),
				placesText(d.CGEPlaces), placesText(d.HeldPlaces),
			})
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("writing the differences: %w", err)
	}
	return nil
}

// placesText writes places as "2p 1 0; 3p 0 1 0", leaving out a size not played.
func placesText(p store.Places) string {
	var parts []string
	for _, s := range []struct {
		n      int
		counts []int
	}{{2, p.P2}, {3, p.P3}, {4, p.P4}} {
		if s.counts == nil {
			continue
		}
		part := fmt.Sprintf("%dp", s.n)
		for _, c := range s.counts {
			part += " " + strconv.Itoa(c)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "; ")
}
