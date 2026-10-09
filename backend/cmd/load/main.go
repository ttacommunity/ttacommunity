// Command load seeds the ratings database from a directory of match CSVs and
// places each game in the tournament catalogue, sets players' countries from
// CSVs with Player and Code columns, or replaces one kind of recorded
// tournament fact from one CSV.
// It checks every row first, and writes no matches if the database has some.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/countrycsv"
	"ttacommunity/backend/internal/matchcsv"
	"ttacommunity/backend/internal/store"
	"ttacommunity/backend/internal/tournamentcsv"
)

func main() {
	reset := flag.Bool("reset", false, "delete every match, refusal and rating run first")
	migrate := flag.Bool("migrate", false, "only apply the ratings migrations, and load nothing")
	countries := flag.Bool("countries", false, "set players' countries from the CSV files named, a later file winning")
	classify := flag.Bool("classify", false, "only place unplaced games in the tournament catalogue, by their names")
	facts := flag.String("facts", "", "replace every recorded tournament fact of one kind (divisions, placements, corrections, dates, honours, rules, podiums, cge_tournaments, cge_stages, cge_standings, ladder_wins, season_notes, community, community_scores, community_points, community_quarters or community_ladders) with the CSV file named")
	export := flag.String("export", "", "write every input table to the directory named (new or empty), one CSV each, with a manifest")
	restore := flag.String("restore", "", "load an export from the directory named into a new database, after applying the migrations")
	addAdmin := flag.String("add-admin", "", "add or update the admin with the Discord id named, with -name and -role")
	adminName := flag.String("name", "", "with -add-admin: the admin's name as admin shows it")
	adminRole := flag.String("role", "", "with -add-admin: admin or organiser")
	standings := flag.String("check-standings", "", "compare CGE's standings tables with the games held: print the counts, and write each difference to the new CSV file named")
	cgeExtract := flag.String("cge-extract", "", "plan how CGE's extract in the directory named meets our games, with -cge-settings; -apply writes it")
	cgeSettings := flag.String("cge-settings", "", "with -cge-extract: the directory of CGE's tournament and stage settings")
	cgePairs := flag.String("cge-pairs", "", "with -cge-extract: a CSV (share_code, game_id, reason) of CGE games matched to ours by hand")
	apply := flag.Bool("apply", false, "with -cge-extract: write the plan")
	inputTables := flag.Bool("input-tables", false, "print the tables that hold inputs, one per line, and touch no database")
	upstream := flag.String("export-upstream", "", "write every played game to the new file named, as the match CSV upstream's site loads")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: load [-reset] <directory of match CSVs>")
		fmt.Fprintln(os.Stderr, "       load -export <directory>")
		fmt.Fprintln(os.Stderr, "       load -restore <directory>")
		fmt.Fprintln(os.Stderr, "       load -export-upstream <file>")
		fmt.Fprintln(os.Stderr, "       load -check-standings <file>")
		fmt.Fprintln(os.Stderr, "       load -add-admin <Discord id> -name <name> -role admin|organiser")
		fmt.Fprintln(os.Stderr, "       load -cge-extract <directory> -cge-settings <directory> [-cge-pairs <file>] [-apply]")
		fmt.Fprintln(os.Stderr, "       load -migrate")
		fmt.Fprintln(os.Stderr, "       load -input-tables")
		fmt.Fprintln(os.Stderr, "       load -classify")
		fmt.Fprintln(os.Stderr, "       load -countries <CSV file>...")
		fmt.Fprintln(os.Stderr, "       load -facts divisions|placements|corrections|dates|honours|rules|podiums|cge_tournaments|cge_stages|cge_standings|ladder_wins|season_notes|community|community_scores|community_points|community_quarters|community_ladders <CSV file>")
		fmt.Fprintln(os.Stderr, "The database is RATINGS_DATABASE_URL.")
		flag.PrintDefaults()
	}
	flag.Parse()
	one := !*reset && btoi(*migrate)+btoi(*classify)+btoi(*countries)+btoi(*facts != "")+btoi(*export != "")+btoi(*restore != "")+btoi(*upstream != "")+btoi(*addAdmin != "")+btoi(*standings != "")+btoi(*cgeExtract != "")+btoi(*inputTables) == 1
	var err error
	switch {
	case *inputTables && one && flag.NArg() == 0:
		writeInputTables(os.Stdout)
	case *migrate && one && flag.NArg() == 0:
		err = migrateOnly()
	case *classify && one && flag.NArg() == 0:
		err = classifyOnly()
	case *countries && one && flag.NArg() > 0:
		err = setCountries(flag.Args())
	case *facts != "" && one && flag.NArg() == 1:
		err = replaceFacts(*facts, flag.Arg(0))
	case *export != "" && one && flag.NArg() == 0:
		err = exportTo(*export)
	case *restore != "" && one && flag.NArg() == 0:
		err = restoreFrom(*restore)
	case *addAdmin != "" && one && flag.NArg() == 0:
		err = withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
			if err := store.MigrateRatings(ctx, pool); err != nil {
				return err
			}
			if err := store.AddAdmin(ctx, pool, store.Admin{DiscordID: *addAdmin, Name: *adminName, Role: *adminRole}); err != nil {
				return err
			}
			fmt.Printf("%s is now %s\n", *adminName, *adminRole)
			return nil
		})
	case *upstream != "" && one && flag.NArg() == 0:
		err = exportForUpstream(*upstream)
	case *standings != "" && one && flag.NArg() == 0:
		err = checkStandings(*standings)
	case *cgeExtract != "" && *cgeSettings != "" && one && flag.NArg() == 0:
		err = loadCGEExtract(*cgeExtract, *cgeSettings, *cgePairs, *apply)
	case !*migrate && !*classify && !*countries && *facts == "" && *export == "" && *restore == "" && *upstream == "" && *addAdmin == "" && *standings == "" && *cgeExtract == "" && !*inputTables && flag.NArg() == 1:
		err = run(*reset, flag.Arg(0))
	default:
		flag.Usage()
		os.Exit(64)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
}

// migrateOnly applies every ratings migration not yet applied.
func migrateOnly() error {
	dbURL := os.Getenv("RATINGS_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("RATINGS_DATABASE_URL is not set")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("opening the ratings database: %w", err)
	}
	defer pool.Close()
	if err := store.MigrateRatings(ctx, pool); err != nil {
		return err
	}
	fmt.Println("ratings migrations applied")
	return nil
}

// exportTo writes an export of the ratings database's inputs to dir and reports each table's rows.
func exportTo(dir string) error {
	return withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
		m, err := store.Export(ctx, pool, dir)
		if err != nil {
			return err
		}
		fmt.Printf("exported at schema version %d to %s:\n", m.SchemaVersion, dir)
		for _, t := range m.Tables {
			fmt.Printf("  %s: %d\n", t.Table, t.Rows)
		}
		return nil
	})
}

// restoreFrom applies the migrations, then loads the export in dir into the
// database, which must hold no inputs and no runs. Run the engine after it.
func restoreFrom(dir string) error {
	return withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
		if err := store.MigrateRatings(ctx, pool); err != nil {
			return err
		}
		if err := store.Restore(ctx, pool, dir); err != nil {
			return err
		}
		fmt.Println("restored from", dir, "- run the engine to compute ratings")
		return nil
	})
}

// exportForUpstream writes every played game to file, which must not exist yet, as upstream's match CSV.
func exportForUpstream(file string) error {
	return withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
		f, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		n, err := store.ExportForUpstream(ctx, pool, f)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		fmt.Printf("wrote %d played games to %s\n", n, file)
		return nil
	})
}

// withPool opens RATINGS_DATABASE_URL and calls f with it, stopping on an interrupt.
func withPool(f func(context.Context, *pgxpool.Pool) error) error {
	dbURL := os.Getenv("RATINGS_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("RATINGS_DATABASE_URL is not set")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("opening the ratings database: %w", err)
	}
	defer pool.Close()
	return f(ctx, pool)
}

// classifyOnly places every unplaced game in the tournament catalogue, after
// applying any pending migration.
func classifyOnly() error {
	dbURL := os.Getenv("RATINGS_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("RATINGS_DATABASE_URL is not set")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("opening the ratings database: %w", err)
	}
	defer pool.Close()
	if err := store.MigrateRatings(ctx, pool); err != nil {
		return err
	}
	placed, err := store.ClassifyMatches(ctx, pool)
	if err != nil {
		return err
	}
	reportClassified(os.Stdout, placed)
	return nil
}

// reportClassified writes how many games were placed and each tournament
// name no rule reads. It names tournaments, never a player.
func reportClassified(w io.Writer, c store.Classified) {
	fmt.Fprintf(w, "placed in the catalogue: %d\n", c.Placed)
	if len(c.Unplaced) == 0 {
		return
	}
	fmt.Fprintf(w, "names no rule reads: %d (their games stay unplaced)\n", len(c.Unplaced))
	for _, n := range c.Unplaced {
		fmt.Fprintf(w, "  %s\n", n)
	}
}

// setCountries reads every file first, then sets the countries of known
// players in one statement, after applying any pending migration.
func setCountries(files []string) error {
	dbURL := os.Getenv("RATINGS_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("RATINGS_DATABASE_URL is not set")
	}
	res, err := countrycsv.ReadFiles(os.DirFS("/"), absolute(files))
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("opening the ratings database: %w", err)
	}
	defer pool.Close()
	if err := store.MigrateRatings(ctx, pool); err != nil {
		return err
	}
	set, err := store.SetCountries(ctx, pool, res.Countries)
	if err != nil {
		return err
	}
	reportCountries(os.Stdout, files, res, set)
	return nil
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// replaceFacts reads the file first, then replaces every recorded fact of
// one kind in one transaction, after applying any pending migration.
func replaceFacts(kind, file string) error {
	switch kind {
	case "divisions":
		return loadFacts(kind, file, tournamentcsv.ReadDivisions, store.ReplaceDivisions)
	case "placements":
		return loadFacts(kind, file, tournamentcsv.ReadPlacements, store.ReplacePlacements)
	case "corrections":
		return loadFacts(kind, file, tournamentcsv.ReadCorrections, store.ReplaceCorrections)
	case "dates":
		return loadFacts(kind, file, tournamentcsv.ReadFinishDates, store.ReplaceFinishDates)
	case "honours":
		return loadFacts(kind, file, tournamentcsv.ReadHonours, store.ReplaceHonours)
	case "rules":
		return loadFacts(kind, file, tournamentcsv.ReadRules, store.ReplaceRules)
	case "podiums":
		return loadFacts(kind, file, tournamentcsv.ReadPodiums, store.ReplacePodiums)
	case "cge_tournaments":
		return loadFacts(kind, file, tournamentcsv.ReadCGETournaments, store.ReplaceCGETournaments)
	case "cge_stages":
		return loadFacts(kind, file, tournamentcsv.ReadCGEStages, store.ReplaceCGEStages)
	case "cge_standings":
		return loadFacts(kind, file, tournamentcsv.ReadCGEStandings, store.ReplaceCGEStandings)
	case "ladder_wins":
		return loadFacts(kind, file, tournamentcsv.ReadLadderWins, store.ReplaceLadderWins)
	case "season_notes":
		return loadFacts(kind, file, tournamentcsv.ReadSeasonNotes, store.ReplaceSeasonNotes)
	case "community":
		return loadFacts(kind, file, tournamentcsv.ReadCommunityStandings, store.ReplaceCommunityStandings)
	case "community_scores":
		return loadFacts(kind, file, tournamentcsv.ReadCommunityScores, store.ReplaceCommunityScores)
	case "community_points":
		return loadFacts(kind, file, tournamentcsv.ReadCommunityPoints, store.ReplaceCommunityPoints)
	case "community_quarters":
		return loadFacts(kind, file, tournamentcsv.ReadCommunityQuarters, store.ReplaceCommunityQuarters)
	case "community_ladders":
		return loadFacts(kind, file, tournamentcsv.ReadCommunityLadders, store.ReplaceCommunityLadders)
	}
	return fmt.Errorf("unknown kind of fact %q: want divisions, placements, corrections, dates, honours, rules, podiums, cge_tournaments, cge_stages, cge_standings, ladder_wins, season_notes, community, community_scores, community_points, community_quarters or community_ladders", kind)
}

func loadFacts[T any](
	kind, file string,
	read func(string, io.Reader) (tournamentcsv.Result[T], error),
	replace func(context.Context, *pgxpool.Pool, []T) (store.FactsReplaced, error),
) error {
	dbURL := os.Getenv("RATINGS_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("RATINGS_DATABASE_URL is not set")
	}
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("opening %s: %w", file, err)
	}
	res, err := read(file, f)
	f.Close()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("opening the ratings database: %w", err)
	}
	defer pool.Close()
	if err := store.MigrateRatings(ctx, pool); err != nil {
		return err
	}
	replaced, err := replace(ctx, pool, res.Facts)
	if err != nil {
		return err
	}
	reportFacts(os.Stdout, kind, file, res.Rows, res.Refused, replaced)
	return nil
}

// reportFacts writes what was read and written. It names the file, lines
// and reasons, never a player.
func reportFacts(w io.Writer, kind, file string, rows int, refused []tournamentcsv.Refusal, replaced store.FactsReplaced) {
	fmt.Fprintf(w, "file: %s\n", file)
	fmt.Fprintf(w, "rows read: %d\n", rows)
	fmt.Fprintf(w, "%s written: %d\n", kind, replaced.Written)
	fmt.Fprintf(w, "not a player: %d\n", replaced.Unknown)
	if replaced.Kept > 0 {
		fmt.Fprintf(w, "kept as set in admin: %d\n", replaced.Kept)
	}
	fmt.Fprintf(w, "refused: %d\n", len(refused))
	for _, r := range refused {
		fmt.Fprintf(w, "  %s:%d %s\n", r.File, r.Line, r.Reason)
	}
}

// absolute turns each path into one os.DirFS("/") can open.
func absolute(files []string) []string {
	out := make([]string, len(files))
	for i, f := range files {
		abs, err := filepath.Abs(f)
		if err != nil {
			abs = f
		}
		out[i] = strings.TrimPrefix(abs, "/")
	}
	return out
}

// reportCountries writes what was read and set. It names files, lines and
// reasons, never a player.
func reportCountries(w io.Writer, files []string, res countrycsv.Result, set store.CountriesSet) {
	fmt.Fprintf(w, "files: %s\n", strings.Join(files, ", "))
	fmt.Fprintf(w, "rows read: %d\n", res.Rows)
	fmt.Fprintf(w, "countries set: %d\n", set.Set)
	fmt.Fprintf(w, "not a player: %d\n", set.Unknown)
	fmt.Fprintf(w, "no country: %d\n", res.None)
	fmt.Fprintf(w, "refused: %d\n", len(res.Refused))
	for _, r := range res.Refused {
		fmt.Fprintf(w, "  %s:%d %s\n", r.File, r.Line, r.Reason)
	}
}

func run(reset bool, dir string) error {
	dbURL := os.Getenv("RATINGS_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("RATINGS_DATABASE_URL is not set")
	}
	res, err := matchcsv.CheckFiles(os.DirFS(dir))
	if err != nil {
		return fmt.Errorf("checking %s: %w", dir, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("opening the ratings database: %w", err)
	}
	defer pool.Close()

	if reset {
		err = store.ResetRatings(ctx, pool)
	} else {
		err = store.MigrateRatings(ctx, pool)
	}
	if err != nil {
		return err
	}
	loaded, err := store.LoadMatches(ctx, pool, res.Matches, res.Refused)
	if err != nil {
		return fmt.Errorf("loading: %w (to start again from empty: -reset, or --reset with scripts/ratings-db.sh)", err)
	}
	report(os.Stdout, res, loaded)
	placed, err := store.ClassifyMatches(ctx, pool)
	if err != nil {
		return fmt.Errorf("placing the loaded games: %w", err)
	}
	reportClassified(os.Stdout, placed)
	return nil
}

// report writes what was read, loaded and refused. It names files and
// reasons, never a player.
func report(w io.Writer, res matchcsv.Result, loaded store.Loaded) {
	fmt.Fprintf(w, "files: %d (%s)\n", len(res.Files), strings.Join(res.Files, ", "))
	fmt.Fprintf(w, "rows read: %d\n", res.Rows)
	fmt.Fprintf(w, "loaded: %d matches, %d players (batch %d)\n", loaded.Matches, loaded.Players, loaded.BatchID)
	fmt.Fprintf(w, "refused: %d\n", loaded.Refused)

	byReason := map[matchcsv.Reason]int{}
	for _, r := range res.Refused {
		byReason[r.Reason]++
	}
	reasons := make([]string, 0, len(byReason))
	for r := range byReason {
		reasons = append(reasons, string(r))
	}
	slices.Sort(reasons)
	for _, r := range reasons {
		fmt.Fprintf(w, "  %-16s %d\n", r, byReason[matchcsv.Reason(r)])
	}
	if loaded.Refused > 0 {
		fmt.Fprintln(w, "Each refused row, with its file, line and reason, is in the refused_rows table.")
	}
}

// writeInputTables writes the tables that hold inputs, one per line: store.ExportTables, then store.NotExported.
// Every other table but goose's holds a run's results.
func writeInputTables(w io.Writer) {
	for _, t := range append(slices.Clone(store.ExportTables), store.NotExported...) {
		fmt.Fprintln(w, t)
	}
}
