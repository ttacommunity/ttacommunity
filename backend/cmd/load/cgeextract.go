package main

import (
	"cmp"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/cgeextract"
	"ttacommunity/backend/internal/store"
)

// loadCGEExtract reads a delivery of CGE's extract from dir (readDelivery's) and, if named, the pairs of CGE's and
// our games confirmed by hand from pairsFile; loads it, after applying any pending migration, and writes a summary
// of what changed to w, as JSON with asJSON. Without apply it rolls the load back, so nothing is written.
func loadCGEExtract(w io.Writer, dir, pairsFile string, apply, asJSON bool) error {
	d, err := readDelivery(dir)
	if err != nil {
		return err
	}
	in := d.In
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
		load := store.DryRunCGEExtract
		if apply {
			load = store.ApplyCGEExtract
		}
		got, err := load(ctx, pool, plan, in)
		if err != nil {
			return err
		}
		summary := newCGESummary(d, plan, stages, got, apply)
		if asJSON {
			return json.NewEncoder(w).Encode(summary)
		}
		summary.writeText(w)
		return nil
	})
}

// readDelivery reads whichever of CGE's files dir holds, each as name.csv or name.csv.gz: the full extract, the
// changes since the last delivery (one of the two at least), the stages' settings and the special rules. A file
// it lacks gives nothing (Stages and Special nil), so what we hold of it stays.
func readDelivery(dir string) (delivery, error) {
	d := delivery{Dir: dir}
	var full, changes *cgeextract.Extract
	read := func(name string, fn func(string, io.Reader) (deliveryFile, error)) error {
		_, err := readOptional(filepath.Join(dir, name), func(n string, r io.Reader) error {
			f, err := fn(n, r)
			f.Name = filepath.Base(n)
			d.Files = append(d.Files, f)
			return err
		})
		return err
	}
	if err := read("tournaments_export.csv", func(n string, r io.Reader) (deliveryFile, error) {
		x, err := cgeextract.Read(n, r)
		full = &x
		return deliveryFile{Rows: x.Rows}, err
	}); err != nil {
		return delivery{}, err
	}
	if err := read("tournaments_export_changes.csv", func(n string, r io.Reader) (deliveryFile, error) {
		x, err := cgeextract.ReadChanges(n, r)
		changes = &x
		return deliveryFile{Rows: x.Rows, Changes: x.Changes}, err
	}); err != nil {
		return delivery{}, err
	}
	if full == nil && changes == nil {
		return delivery{}, fmt.Errorf("%s holds neither tournaments_export.csv nor tournaments_export_changes.csv "+
			"(either may be gzipped)", dir)
	}
	if err := read("tournaments_export_special_rules.csv", func(n string, r io.Reader) (deliveryFile, error) {
		got, err := cgeextract.ReadSpecialRules(n, r)
		d.In.Special = got.Games
		return deliveryFile{Rows: len(got.Games)}, err
	}); err != nil {
		return delivery{}, err
	}
	if err := read("tournaments_export_stages.csv", func(n string, r io.Reader) (deliveryFile, error) {
		got, err := cgeextract.ReadStages(n, r)
		if err == nil && len(got.Refused) > 0 {
			err = fmt.Errorf("%s: %d rows refused, the first at line %d", n, len(got.Refused), got.Refused[0].Line)
		}
		d.In.Stages = append([]cgeextract.StageSettings{}, got.Settings...)
		return deliveryFile{Rows: len(got.Settings)}, err
	}); err != nil {
		return delivery{}, err
	}
	d.In.Extract = cgeextract.Combine(full, changes)
	return d, nil
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
