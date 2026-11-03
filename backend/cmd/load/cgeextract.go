package main

import (
	"cmp"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/cgeextract"
	"ttacommunity/backend/internal/store"
)

// loadCGEExtract reads a delivery of CGE's extract from dir (readDelivery's) and, if named, the pairs of CGE's and
// our games confirmed by hand from pairsFile; plans how it meets our games and prints the plan; with apply it also
// writes it, after applying any pending migration.
func loadCGEExtract(dir, pairsFile string, apply bool) error {
	in, err := readDelivery(dir)
	if err != nil {
		return err
	}
	var confirmed map[string]int64
	if pairsFile != "" {
		if found, err := readOptional(pairsFile, func(n string, r io.Reader) error {
			got, err := cgeextract.ReadConfirmedPairs(n, r)
			if err == nil && len(got.Refused) > 0 {
				err = fmt.Errorf("%s: %d rows refused, the first at line %d", n, len(got.Refused), got.Refused[0].Line)
			}
			confirmed = got.Pairs
			return err
		}); err != nil || !found {
			return cmp.Or(err, fmt.Errorf("%s: %w", pairsFile, fs.ErrNotExist))
		}
	}
	return withRatings(func(ctx context.Context, pool *pgxpool.Pool) error {
		plan, err := store.PlanCGEExtract(ctx, pool, in.Extract, confirmed)
		if err != nil {
			return err
		}
		stages, err := store.PlanCGEStages(ctx, pool, in.Stages)
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
		reportCGEExtract(os.Stdout, plan, stages, in.Extract.Refused, applied)
		if !apply {
			return nil
		}
		return linkMembers(ctx, pool)
	})
}

// reportCGEExtract writes how the extract meets our games and our stages (stages, PlanCGEStages'), and what was
// written if applied. It names no player.
func reportCGEExtract(w io.Writer, plan store.CGEPlan, stages []store.CGEStageWrite, refused []cgeextract.Refusal, applied *store.CGEApplied) {
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
	fmt.Fprintf(w, "removed by CGE: %d (%d played)\n", len(plan.Removed), plan.RemovedPlayed)
	for _, id := range plan.Removed {
		fmt.Fprintf(w, "  game %d\n", id)
	}
	var disagree []store.CGEStageWrite
	for _, s := range stages {
		if len(s.Disagree) > 0 {
			disagree = append(disagree, s)
		}
	}
	fmt.Fprintf(w, "stages of ours with CGE stages now: %d\n", len(stages))
	fmt.Fprintf(w, "settings their CGE stages disagree on, kept: %d\n", len(disagree))
	for _, s := range disagree {
		fmt.Fprintf(w, "  stage %d: %s\n", s.StageID, strings.Join(s.Disagree, ", "))
	}
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
		"%d games moved into their CGE group, %d players renamed after their account, %d games CGE removed deleted\n",
		a.BatchID, a.Added, a.Unplaced, a.SeatsAdded, a.AccountsAdded, a.PlayersAdded, a.Relinked, a.SeriesAdded,
		a.TournamentsMerged, a.CopiesRemoved, a.SettingsWritten, a.SeatsRemoved, a.GamesMoved, a.PlayersRenamed,
		a.GamesRemoved)
}

// readDelivery reads whichever of CGE's files dir holds, each as name.csv or name.csv.gz: the full extract, the
// changes since the last delivery (one of the two at least), the stages' settings and the special rules. A file
// it lacks gives nothing (Stages and Special nil), so what we hold of it stays.
func readDelivery(dir string) (store.CGEInput, error) {
	var in store.CGEInput
	var full, changes *cgeextract.Extract
	read := func(name string, fn func(string, io.Reader) error) (bool, error) {
		return readOptional(filepath.Join(dir, name), fn)
	}
	steps := []struct {
		name string
		read func(string, io.Reader) error
	}{
		{"tournaments_export.csv", func(n string, r io.Reader) error {
			x, err := cgeextract.Read(n, r)
			full = &x
			return err
		}},
		{"tournaments_export_changes.csv", func(n string, r io.Reader) error {
			x, err := cgeextract.ReadChanges(n, r)
			changes = &x
			return err
		}},
		{"tournaments_export_special_rules.csv", func(n string, r io.Reader) error {
			got, err := cgeextract.ReadSpecialRules(n, r)
			in.Special = got.Games
			return err
		}},
		{"tournaments_export_stages.csv", func(n string, r io.Reader) error {
			got, err := cgeextract.ReadStages(n, r)
			if err == nil && len(got.Refused) > 0 {
				err = fmt.Errorf("%s: %d rows refused, the first at line %d", n, len(got.Refused), got.Refused[0].Line)
			}
			in.Stages = append([]cgeextract.StageSettings{}, got.Settings...)
			return err
		}},
	}
	for i, s := range steps {
		if _, err := read(s.name, s.read); err != nil {
			return store.CGEInput{}, err
		}
		if i == 1 && full == nil && changes == nil {
			return store.CGEInput{}, fmt.Errorf("%s holds neither tournaments_export.csv nor tournaments_export_changes.csv "+
				"(either may be gzipped)", dir)
		}
	}
	in.Extract = cgeextract.Combine(full, changes)
	return in, nil
}

// readOptional reads the file named name, else name.gz unzipped, with fn; it reports false for neither.
func readOptional(name string, fn func(string, io.Reader) error) (bool, error) {
	for _, n := range []string{name, name + ".gz"} {
		f, err := os.Open(n)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, err
		}
		defer f.Close()
		var r io.Reader = f
		if strings.HasSuffix(n, ".gz") {
			z, err := gzip.NewReader(f)
			if err != nil {
				return false, fmt.Errorf("%s: %w", n, err)
			}
			defer z.Close()
			r = z
		}
		return true, fn(n, r)
	}
	return false, nil
}
