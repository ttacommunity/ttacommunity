package main

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/cgeextract"
	"ttacommunity/backend/internal/store"
)

// loadCGEExtract reads CGE's extract from dir, its settings from settingsDir and, if named, the pairs of CGE's
// and our games confirmed by hand from pairsFile; plans how it meets our games and prints the plan; with apply
// it also writes it, after applying any pending migration.
func loadCGEExtract(dir, settingsDir, pairsFile string, apply bool) error {
	read := func(name string, read func(string, io.Reader) error) error {
		f, err := os.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		return read(name, f)
	}
	var in store.CGEInput
	var confirmed map[string]int64
	steps := []struct {
		file string
		read func(string, io.Reader) error
	}{
		{filepath.Join(dir, "tournaments_export.csv"), func(n string, r io.Reader) (err error) {
			in.Extract, err = cgeextract.Read(n, r)
			return err
		}},
		{filepath.Join(dir, "tournaments_export_special_rules.csv"), func(n string, r io.Reader) error {
			got, err := cgeextract.ReadSpecialRules(n, r)
			in.Special = got.Games
			return err
		}},
		{filepath.Join(settingsDir, "tournament_settings.csv"), func(n string, r io.Reader) error {
			got, err := cgeextract.ReadTournamentSettings(n, r)
			in.Settings = append(in.Settings, got.Settings...)
			return err
		}},
		{filepath.Join(settingsDir, "stage_settings.csv"), func(n string, r io.Reader) error {
			got, err := cgeextract.ReadStageSettings(n, r)
			in.Settings = append(in.Settings, got.Settings...)
			return err
		}},
	}
	if pairsFile != "" {
		steps = append(steps, struct {
			file string
			read func(string, io.Reader) error
		}{pairsFile, func(n string, r io.Reader) error {
			got, err := cgeextract.ReadConfirmedPairs(n, r)
			if err == nil && len(got.Refused) > 0 {
				err = fmt.Errorf("%s: %d rows refused, the first at line %d", n, len(got.Refused), got.Refused[0].Line)
			}
			confirmed = got.Pairs
			return err
		}})
	}
	for _, s := range steps {
		if err := read(s.file, s.read); err != nil {
			return err
		}
	}
	return withRatings(func(ctx context.Context, pool *pgxpool.Pool) error {
		plan, err := store.PlanCGEExtract(ctx, pool, in.Extract, confirmed)
		if err != nil {
			return err
		}
		var applied *store.CGEApplied
		if apply {
			got, err := store.ApplyCGEExtract(ctx, pool, plan, in)
			if err != nil {
				return err
			}
			applied = &got
		}
		reportCGEExtract(os.Stdout, plan, in.Extract.Refused, applied)
		if !apply {
			return nil
		}
		return linkMembers(ctx, pool)
	})
}

// reportCGEExtract writes how the extract meets our games, and what was written if applied. It names no player.
func reportCGEExtract(w io.Writer, plan store.CGEPlan, refused []cgeextract.Refusal, applied *store.CGEApplied) {
	states := map[int]string{}
	codes := map[int]string{}
	for _, g := range plan.Games {
		states[g.SessionID], codes[g.SessionID] = g.State, g.ShareCode
	}
	byState := map[string]int{}
	for _, s := range plan.Matching.Added {
		byState[states[s]]++
	}
	fmt.Fprintf(w, "CGE games: %d\n", len(plan.Games))
	fmt.Fprintf(w, "left out, in seasons run on this site: %d\n", plan.RunHere)
	fmt.Fprintf(w, "matched to ours: %d\n", len(plan.Matching.Matched))
	fmt.Fprintf(w, "only CGE has: %d (", len(plan.Matching.Added))
	for i, state := range slices.Sorted(maps.Keys(byState)) {
		if i > 0 {
			fmt.Fprint(w, ", ")
		}
		fmt.Fprintf(w, "%s %d", state, byState[state])
	}
	fmt.Fprintln(w, ")")
	fmt.Fprintf(w, "held back as possible copies: %d\n", len(plan.Matching.HeldBack))
	for _, s := range plan.Matching.HeldBack {
		fmt.Fprintf(w, "  %s\n", codes[s])
	}
	fmt.Fprintf(w, "ours only: %d\n", len(plan.Matching.OursOnly))
	fmt.Fprintf(w, "refused: %d\n", len(refused))
	for _, r := range refused {
		fmt.Fprintf(w, "  line %d %s\n", r.Line, r.Reason)
	}
	if applied == nil {
		fmt.Fprintln(w, "nothing written: add -apply to write it")
		return
	}
	a := *applied
	fmt.Fprintf(w, "batch %d: %d games added (%d with no place to go), %d seats added, %d accounts and %d players added, "+
		"%d accounts relinked to the player their seats show, %d hidden series added, %d split tournaments merged, "+
		"%d copies an earlier load added removed, %d settings rows checked, %d seats of players CGE removed dropped, "+
		"%d games moved into their CGE group, %d players renamed after their account\n",
		a.BatchID, a.Added, a.Unplaced, a.SeatsAdded, a.AccountsAdded, a.PlayersAdded, a.Relinked, a.SeriesAdded,
		a.TournamentsMerged, a.CopiesRemoved, a.SettingsWritten, a.SeatsRemoved, a.GamesMoved, a.PlayersRenamed)
}
