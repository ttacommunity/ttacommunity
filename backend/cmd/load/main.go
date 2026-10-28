// Command load is the ratings database's data job: it applies migrations, loads match CSVs, CGE's extract and
// recorded facts, and exports, backs up and restores the inputs, one subcommand each. It checks every row first.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/backup"
	"ttacommunity/backend/internal/configbackup"
	"ttacommunity/backend/internal/countrycsv"
	"ttacommunity/backend/internal/matchcsv"
	"ttacommunity/backend/internal/store"
	"ttacommunity/backend/internal/tournamentcsv"
)

// A command is one of load's subcommands: its name, its arguments as usage shows them, what it does, and how it
// runs. run parses its own flags from fs and returns errUsage for a wrong command line.
type command struct {
	name, args, about string
	run               func(fs *flag.FlagSet, args []string) error
}

// errUsage is a command line its command cannot run: load prints the command's usage and exits 64. errBadFlag is
// a flag the command does not know or cannot read: the flag package has printed it, and load exits 2.
var (
	errUsage   = errors.New("usage")
	errBadFlag = errors.New("bad flag")
)

var commands = []command{
	{"matches", "[-reset] <directory of match CSVs>", "load every match CSV in the directory and place each game in the tournament catalogue", func(fs *flag.FlagSet, args []string) error {
		reset := fs.Bool("reset", false, "delete every match, refusal and rating run first")
		if err := parse(fs, args, 1); err != nil {
			return err
		}
		return loadMatches(*reset, fs.Arg(0))
	}},
	{"migrate", "", "only apply the ratings migrations, and load nothing", func(fs *flag.FlagSet, args []string) error {
		if err := parse(fs, args, 0); err != nil {
			return err
		}
		return withRatings(func(context.Context, *pgxpool.Pool) error {
			fmt.Println("ratings migrations applied")
			return nil
		})
	}},
	{"classify", "", "only place unplaced games in the tournament catalogue, by their names", func(fs *flag.FlagSet, args []string) error {
		if err := parse(fs, args, 0); err != nil {
			return err
		}
		return classifyOnly()
	}},
	{"countries", "<CSV file>...", "set players' countries, on their members, from CSV files with Player and Code columns, a later file winning; a player who has signed in keeps their own", func(fs *flag.FlagSet, args []string) error {
		if err := parseFlags(fs, args); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			return errUsage
		}
		return setCountries(fs.Args())
	}},
	{"facts", "<kind> <CSV file>", "replace every recorded tournament fact of one kind with the CSV file: " + strings.Join(slices.Sorted(maps.Keys(factKinds)), ", "), func(fs *flag.FlagSet, args []string) error {
		if err := parse(fs, args, 2); err != nil {
			return err
		}
		return replaceFacts(fs.Arg(0), fs.Arg(1))
	}},
	{"export", "<directory>", "write every input table to the directory (new or empty), one CSV each, with a manifest", func(fs *flag.FlagSet, args []string) error {
		if err := parse(fs, args, 1); err != nil {
			return err
		}
		return exportTo(fs.Arg(0), store.Export)
	}},
	{"backup", "<directory>", "write every input table but sign-in secrets to the directory (new or empty), one CSV each, with a manifest: it holds personal data", func(fs *flag.FlagSet, args []string) error {
		if err := parse(fs, args, 1); err != nil {
			return err
		}
		return exportTo(fs.Arg(0), store.Backup)
	}},
	{"backup-to", "<directory>", "write a backup, zipped and encrypted with age to BACKUP_AGE_RECIPIENT, to the directory as <time>.zip.age, for the backup job to upload", func(fs *flag.FlagSet, args []string) error {
		if err := parse(fs, args, 1); err != nil {
			return err
		}
		return backupToDir(fs.Arg(0), time.Now())
	}},
	{"backup-config-to", "<directory>", "write the namespace's Secrets and the Cloudflare settings of BACKUP_CONFIG_ZONE (read with CLOUDFLARE_API_TOKEN), each as JSON encrypted with age to BACKUP_AGE_RECIPIENT, to secrets/<time>.json.age and cloudflare/<time>.json.age under the directory, for the backup-config job to upload; touches no database", func(fs *flag.FlagSet, args []string) error {
		if err := parse(fs, args, 1); err != nil {
			return err
		}
		return backupConfigToDir(fs.Arg(0), time.Now())
	}},
	{"restore", "<directory>", "load an export or a backup from the directory into a new database, after applying the migrations", func(fs *flag.FlagSet, args []string) error {
		if err := parse(fs, args, 1); err != nil {
			return err
		}
		return restoreFrom(fs.Arg(0))
	}},
	{"export-upstream", "<file>", "write every played game to the new file, as the match CSV upstream's site loads", func(fs *flag.FlagSet, args []string) error {
		if err := parse(fs, args, 1); err != nil {
			return err
		}
		return exportForUpstream(fs.Arg(0))
	}},
	{"check-standings", "<file>", "compare CGE's standings tables with the games held: print the counts, and write each difference to the new CSV file", func(fs *flag.FlagSet, args []string) error {
		if err := parse(fs, args, 1); err != nil {
			return err
		}
		return checkStandings(fs.Arg(0))
	}},
	{"add-admin", "-name <name> -role admin|organiser <Discord id>", "add or update the admin with the Discord id", func(fs *flag.FlagSet, args []string) error {
		name := fs.String("name", "", "the admin's name as admin shows it")
		role := fs.String("role", "", "admin or organiser")
		if err := parse(fs, args, 1); err != nil {
			return err
		}
		if *name == "" || *role == "" {
			return errUsage
		}
		return addAdmin(store.Admin{DiscordID: fs.Arg(0), Name: *name, Role: *role})
	}},
	{"cge-extract", "-settings <directory> [-pairs <file>] [-apply] <directory>", "plan how CGE's extract in the directory meets our games; -apply writes it", func(fs *flag.FlagSet, args []string) error {
		settings := fs.String("settings", "", "the directory of CGE's tournament and stage settings")
		pairs := fs.String("pairs", "", "a CSV (share_code, game_id, reason) of CGE games matched to ours by hand")
		apply := fs.Bool("apply", false, "write the plan")
		if err := parse(fs, args, 1); err != nil {
			return err
		}
		if *settings == "" {
			return errUsage
		}
		return loadCGEExtract(fs.Arg(0), *settings, *pairs, *apply)
	}},
	{"input-tables", "", "print the tables that hold inputs, one per line, and touch no database", func(fs *flag.FlagSet, args []string) error {
		if err := parse(fs, args, 0); err != nil {
			return err
		}
		writeInputTables(os.Stdout)
		return nil
	}},
}

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

// run runs the command args name and returns load's exit status: 64 for a missing or unknown command or a wrong
// command line, 2 for a bad flag, 1 for a failed command. It writes usage and errors to stderr.
func run(args []string, stderr io.Writer) int {
	i := slices.IndexFunc(commands, func(c command) bool { return len(args) > 0 && c.name == args[0] })
	if i < 0 {
		usage(stderr)
		return 64
	}
	c := commands[i]
	fs := flag.NewFlagSet("load "+c.name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: load %s %s\n%s\n", c.name, c.args, c.about)
		fs.PrintDefaults()
	}
	switch err := c.run(fs, args[1:]); {
	case err == nil:
		return 0
	case errors.Is(err, flag.ErrHelp):
		return 0
	case errors.Is(err, errUsage):
		fs.Usage()
		return 64
	case errors.Is(err, errBadFlag):
		return 2
	default:
		fmt.Fprintln(stderr, "load:", err)
		return 1
	}
}

// usage writes every command's command line to w.
func usage(w io.Writer) {
	for i, c := range commands {
		prefix := "       "
		if i == 0 {
			prefix = "usage: "
		}
		fmt.Fprintln(w, strings.TrimRight(prefix+"load "+c.name+" "+c.args, " "))
	}
	fmt.Fprintln(w, "The database is RATINGS_DATABASE_URL. load <command> -h says what a command does.")
}

// parseFlags parses args into fs: a bad flag is errBadFlag, and -h is flag.ErrHelp.
func parseFlags(fs *flag.FlagSet, args []string) error {
	err := fs.Parse(args)
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		return errBadFlag
	}
	return err
}

// parse is parseFlags, refusing with errUsage any count of arguments but n.
func parse(fs *flag.FlagSet, args []string, n int) error {
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != n {
		return errUsage
	}
	return nil
}

// exportTo writes the ratings database's inputs to dir with write (store.Export or store.Backup) and reports each table's rows.
func exportTo(dir string, write func(context.Context, *pgxpool.Pool, string) (store.Manifest, error)) error {
	return withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
		m, err := write(ctx, pool, dir)
		if err != nil {
			return err
		}
		fmt.Printf("wrote schema version %d to %s:\n", m.SchemaVersion, dir)
		for _, t := range m.Tables {
			fmt.Printf("  %s: %d\n", t.Table, t.Rows)
		}
		return nil
	})
}

// backupToDir writes a backup taken at now, encrypted to BACKUP_AGE_RECIPIENT, to dir (backup.Write), and reports
// its file, size and each table's rows.
func backupToDir(dir string, now time.Time) error {
	r, err := backup.RecipientFrom(os.Getenv)
	if err != nil {
		return err
	}
	return withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
		var m store.Manifest
		path, size, err := backup.Write(dir, backup.Name(now), r, func(w io.Writer) error {
			var err error
			m, err = store.BackupZip(ctx, pool, w)
			return err
		})
		if err != nil {
			return err
		}
		fmt.Printf("backed up schema version %d to %s (%d bytes, encrypted):\n", m.SchemaVersion, path, size)
		for _, t := range m.Tables {
			fmt.Printf("  %s: %d\n", t.Table, t.Rows)
		}
		return nil
	})
}

// serviceAccountDir is where Kubernetes mounts a pod's token and the cluster's CA.
const serviceAccountDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// backupConfigToDir writes the namespace's Secrets and the Cloudflare settings, each encrypted to
// BACKUP_AGE_RECIPIENT, to dir/secrets and dir/cloudflare as <time>.json.age, and reports each file and size.
// It refuses to start outside a cluster or without CLOUDFLARE_API_TOKEN and BACKUP_CONFIG_ZONE.
func backupConfigToDir(dir string, now time.Time) error {
	r, err := backup.RecipientFrom(os.Getenv)
	if err != nil {
		return err
	}
	cluster, err := configbackup.ClusterFrom(os.Getenv, serviceAccountDir)
	if err != nil {
		return err
	}
	account := configbackup.Account{API: "https://api.cloudflare.com/client/v4", Token: os.Getenv("CLOUDFLARE_API_TOKEN"), Zone: os.Getenv("BACKUP_CONFIG_ZONE"), Client: &http.Client{Timeout: 60 * time.Second}}
	if account.Token == "" || account.Zone == "" {
		return errors.New("CLOUDFLARE_API_TOKEN and BACKUP_CONFIG_ZONE must both be set")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for _, part := range []struct {
		name string
		read func(context.Context) ([]byte, error)
	}{
		{"secrets", func(ctx context.Context) ([]byte, error) { return configbackup.Secrets(ctx, cluster) }},
		{"cloudflare", func(ctx context.Context) ([]byte, error) { return configbackup.Cloudflare(ctx, account) }},
	} {
		doc, err := part.read(ctx)
		if err != nil {
			return fmt.Errorf("backing up %s: %w", part.name, err)
		}
		sub := filepath.Join(dir, part.name)
		if err := os.MkdirAll(sub, 0o755); err != nil {
			return fmt.Errorf("backing up %s: %w", part.name, err)
		}
		path, size, err := backup.Write(sub, backup.Stamp(now)+".json.age", r, func(w io.Writer) error {
			_, err := w.Write(doc)
			return err
		})
		if err != nil {
			return fmt.Errorf("backing up %s: %w", part.name, err)
		}
		fmt.Printf("backed up %s to %s (%d bytes, encrypted)\n", part.name, path, size)
	}
	return nil
}

// restoreFrom applies the migrations, then loads the export in dir into the
// database, which must hold no inputs and no runs. Run the engine after it.
func restoreFrom(dir string) error {
	return withRatings(func(ctx context.Context, pool *pgxpool.Pool) error {
		if err := store.Restore(ctx, pool, dir); err != nil {
			return err
		}
		fmt.Println("restored from", dir, "- run the engine to compute ratings")
		return nil
	})
}

// addAdmin adds or updates the admin a, after applying any pending migration.
func addAdmin(a store.Admin) error {
	return withRatings(func(ctx context.Context, pool *pgxpool.Pool) error {
		if err := store.AddAdmin(ctx, pool, a); err != nil {
			return err
		}
		fmt.Printf("%s is now %s\n", a.Name, a.Role)
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
	return withRatings(func(ctx context.Context, pool *pgxpool.Pool) error {
		placed, err := store.ClassifyMatches(ctx, pool)
		if err != nil {
			return err
		}
		reportClassified(os.Stdout, placed)
		return nil
	})
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
	res, err := countrycsv.ReadFiles(os.DirFS("/"), absolute(files))
	if err != nil {
		return err
	}
	return withRatings(func(ctx context.Context, pool *pgxpool.Pool) error {
		set, err := store.SetCountries(ctx, pool, res.Countries)
		if err != nil {
			return err
		}
		reportCountries(os.Stdout, files, res, set)
		return nil
	})
}

// factKinds replaces every recorded fact of one kind from a file, by the kind's name.
var factKinds = map[string]func(kind, file string) error{
	"divisions":          facts(tournamentcsv.ReadDivisions, store.ReplaceDivisions),
	"placements":         facts(tournamentcsv.ReadPlacements, store.ReplacePlacements),
	"corrections":        facts(tournamentcsv.ReadCorrections, store.ReplaceCorrections),
	"dates":              facts(tournamentcsv.ReadFinishDates, store.ReplaceFinishDates),
	"honours":            facts(tournamentcsv.ReadHonours, store.ReplaceHonours),
	"rules":              facts(tournamentcsv.ReadRules, store.ReplaceRules),
	"podiums":            knownFacts(store.SeriesSlugs, tournamentcsv.ReadPodiums, store.ReplacePodiums),
	"cge_tournaments":    facts(tournamentcsv.ReadCGETournaments, store.ReplaceCGETournaments),
	"cge_stages":         knownFacts(store.SeriesSlugs, tournamentcsv.ReadCGEStages, store.ReplaceCGEStages),
	"cge_standings":      facts(tournamentcsv.ReadCGEStandings, store.ReplaceCGEStandings),
	"ladder_wins":        facts(tournamentcsv.ReadLadderWins, store.ReplaceLadderWins),
	"season_notes":       knownFacts(store.SeriesSlugs, tournamentcsv.ReadSeasonNotes, store.ReplaceSeasonNotes),
	"community":          knownFacts(store.CommunityEditionSlugs, tournamentcsv.ReadCommunityStandings, store.ReplaceCommunityStandings),
	"community_scores":   knownFacts(store.CommunityEditionSlugs, tournamentcsv.ReadCommunityScores, store.ReplaceCommunityScores),
	"community_points":   facts(tournamentcsv.ReadCommunityPoints, store.ReplaceCommunityPoints),
	"community_quarters": facts(tournamentcsv.ReadCommunityQuarters, store.ReplaceCommunityQuarters),
	"community_ladders":  facts(tournamentcsv.ReadCommunityLadders, store.ReplaceCommunityLadders),
}

// replaceFacts reads the file, then replaces every recorded fact of one kind
// in one transaction, after applying any pending migration. A file with any
// refused row writes nothing.
func replaceFacts(kind, file string) error {
	load, ok := factKinds[kind]
	if !ok {
		return fmt.Errorf("unknown kind of fact %q: want one of %s", kind, strings.Join(slices.Sorted(maps.Keys(factKinds)), ", "))
	}
	return load(kind, file)
}

// facts is loadFacts for one kind, read by read and written by replace.
func facts[T any](
	read func(string, io.Reader) (tournamentcsv.Result[T], error),
	replace func(context.Context, *pgxpool.Pool, []T) (store.FactsReplaced, error),
) func(kind, file string) error {
	return func(kind, file string) error { return loadFacts(kind, file, read, replace) }
}

// knownFacts is loadKnownFacts for one kind, whose slugs known lists.
func knownFacts[T any](
	known func(context.Context, *pgxpool.Pool) ([]string, error),
	read func(string, io.Reader, []string) (tournamentcsv.Result[T], error),
	replace func(context.Context, *pgxpool.Pool, []T) (store.FactsReplaced, error),
) func(kind, file string) error {
	return func(kind, file string) error { return loadKnownFacts(kind, file, known, read, replace) }
}

func loadFacts[T any](
	kind, file string,
	read func(string, io.Reader) (tournamentcsv.Result[T], error),
	replace func(context.Context, *pgxpool.Pool, []T) (store.FactsReplaced, error),
) error {
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("opening %s: %w", file, err)
	}
	res, err := read(file, f)
	f.Close()
	if err != nil {
		return err
	}
	// A replace drops every row it is not given, so a refused row would remove the fact it was meant to correct.
	if len(res.Refused) > 0 {
		reportFacts(os.Stdout, kind, file, res.Rows, res.Refused, store.FactsReplaced{})
		return fmt.Errorf("%s: %d of %d rows refused, so nothing was written", file, len(res.Refused), res.Rows)
	}
	return withRatings(func(ctx context.Context, pool *pgxpool.Pool) error {
		replaced, err := replace(ctx, pool, res.Facts)
		if err != nil {
			return err
		}
		reportFacts(os.Stdout, kind, file, res.Rows, res.Refused, replaced)
		return nil
	})
}

// loadKnownFacts is loadFacts for a file whose reader refuses a row naming a
// slug the database does not hold, such as a series or a community edition:
// known lists those slugs.
func loadKnownFacts[T any](
	kind, file string,
	known func(context.Context, *pgxpool.Pool) ([]string, error),
	read func(string, io.Reader, []string) (tournamentcsv.Result[T], error),
	replace func(context.Context, *pgxpool.Pool, []T) (store.FactsReplaced, error),
) error {
	var slugs []string
	err := withRatings(func(ctx context.Context, pool *pgxpool.Pool) (err error) {
		slugs, err = known(ctx, pool)
		return err
	})
	if err != nil {
		return err
	}
	return loadFacts(kind, file, func(f string, r io.Reader) (tournamentcsv.Result[T], error) {
		return read(f, r, slugs)
	}, replace)
}

// withRatings opens the ratings database, applies any pending migration, and
// runs fn with it.
func withRatings(fn func(context.Context, *pgxpool.Pool) error) error {
	return withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
		if err := store.MigrateRatings(ctx, pool); err != nil {
			return err
		}
		return fn(ctx, pool)
	})
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
	fmt.Fprintf(w, "signed in, kept their own: %d\n", set.Kept)
	fmt.Fprintf(w, "not a player: %d\n", set.Unknown)
	fmt.Fprintf(w, "no country: %d\n", res.None)
	fmt.Fprintf(w, "refused: %d\n", len(res.Refused))
	for _, r := range res.Refused {
		fmt.Fprintf(w, "  %s:%d %s\n", r.File, r.Line, r.Reason)
	}
}

// loadMatches checks every match CSV in dir, then loads them, places each game in the tournament catalogue and
// links members, after applying any pending migration or, with reset, emptying the database.
func loadMatches(reset bool, dir string) error {
	res, err := matchcsv.CheckFiles(os.DirFS(dir))
	if err != nil {
		return fmt.Errorf("checking %s: %w", dir, err)
	}
	return withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
		var err error
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
			return fmt.Errorf("loading: %w (to start again from empty: matches -reset, or --reset with scripts/ratings-db.sh)", err)
		}
		report(os.Stdout, res, loaded)
		placed, err := store.ClassifyMatches(ctx, pool)
		if err != nil {
			return fmt.Errorf("placing the loaded games: %w", err)
		}
		reportClassified(os.Stdout, placed)
		return linkMembers(ctx, pool)
	})
}

// linkMembers attaches the community's members to the accounts and players a load brought, and says how many.
func linkMembers(ctx context.Context, pool *pgxpool.Pool) error {
	linked, err := store.LinkMembers(ctx, pool)
	if err != nil {
		return fmt.Errorf("linking members: %w", err)
	}
	fmt.Fprintf(os.Stdout, "members linked: %d to a CGE account, %d to a player, %d renamed to their account's name\n", linked.Accounts, linked.Players, linked.Renamed)
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
