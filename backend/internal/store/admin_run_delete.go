// Protecting rating runs from deletion, deleting the rest, and how much room each takes.

package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// The reasons a run cannot be deleted.
var (
	ErrRunProtected   = errors.New("the run is protected")
	ErrRunCurrent     = errors.New("the run is the one the site shows")
	ErrRunRunning     = errors.New("the run is still running")
	ErrRunNotDeleting = errors.New("the run is not marked for deletion")
)

// SetRunProtected protects a run from deletion, or stops protecting it. Protecting a run marked for
// deletion keeps it. It returns ErrRunNotFound.
func (a *AdminStore) SetRunProtected(ctx context.Context, run int64, protected bool) error {
	tag, err := a.pool.Exec(ctx, `
		UPDATE rating_runs SET protected = $2, deleting = deleting AND NOT $2
		WHERE run_id = $1 AND ranking_id = community_ranking()`, run, protected)
	if err != nil {
		return fmt.Errorf("protecting run %d: %w", run, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRunNotFound
	}
	return nil
}

// StartDeleteRun marks a run for deletion, for DeleteRun to remove. It refuses a protected run, the current
// one, and a running one while the engine is going (one the engine left running can go). It returns
// ErrRunNotFound, ErrRunProtected, ErrRunCurrent or ErrRunRunning.
func (a *AdminStore) StartDeleteRun(ctx context.Context, run int64, engineGoing bool) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		if err := deletable(ctx, tx, run, engineGoing); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE rating_runs SET deleting = true WHERE run_id = $1", run); err != nil {
			return fmt.Errorf("marking run %d for deletion: %w", run, err)
		}
		return nil
	})
}

// DeleteRun removes a run marked for deletion, and every result of it, in one transaction. It checks
// again, as the run may have been protected or published since it was marked; a run that can no longer
// go is unmarked. It returns ErrRunNotDeleting, ErrRunProtected or ErrRunCurrent.
func (a *AdminStore) DeleteRun(ctx context.Context, run int64) error {
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var deleting bool
		err := tx.QueryRow(ctx, "SELECT deleting FROM rating_runs WHERE run_id = $1 FOR UPDATE", run).Scan(&deleting)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !deleting) {
			return ErrRunNotDeleting
		}
		if err != nil {
			return fmt.Errorf("deleting run %d: %w", run, err)
		}
		if err := deletable(ctx, tx, run, false); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SELECT delete_run($1)", run); err != nil {
			return fmt.Errorf("deleting run %d: %w", run, err)
		}
		return nil
	})
	if errors.Is(err, ErrRunProtected) || errors.Is(err, ErrRunCurrent) {
		if _, uerr := a.pool.Exec(ctx, "UPDATE rating_runs SET deleting = false WHERE run_id = $1", run); uerr != nil {
			return errors.Join(err, fmt.Errorf("unmarking run %d: %w", run, uerr))
		}
	}
	return err
}

// deletable returns why a run cannot be deleted, or nil.
func deletable(ctx context.Context, tx pgx.Tx, run int64, engineGoing bool) error {
	var status string
	var protected, current bool
	err := tx.QueryRow(ctx, `
		SELECT status, protected, EXISTS (SELECT 1 FROM current_run c WHERE c.run_id = r.run_id)
		FROM rating_runs r WHERE run_id = $1 AND ranking_id = community_ranking()`, run).Scan(&status, &protected, &current)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrRunNotFound
	case err != nil:
		return fmt.Errorf("checking run %d: %w", run, err)
	case protected:
		return ErrRunProtected
	case current:
		return ErrRunCurrent
	case status == "running" && engineGoing:
		return ErrRunRunning
	}
	return nil
}

// RunSizes estimates how many bytes each run takes: each run table's size on disk, shared among the runs by
// their rows in it. A run with no rows takes 0.
func (a *AdminStore) RunSizes(ctx context.Context) (map[int64]int64, error) {
	tables, err := a.pool.Query(ctx, `
		SELECT k.conrelid::regclass::text, pg_total_relation_size(k.conrelid)
		FROM pg_constraint k WHERE k.confrelid = 'rating_runs'::regclass AND k.contype = 'f' AND k.confdeltype = 'c'`)
	if err != nil {
		return nil, fmt.Errorf("listing run tables: %w", err)
	}
	type table struct {
		name  string
		bytes int64
	}
	list, err := pgx.CollectRows(tables, func(row pgx.CollectableRow) (table, error) {
		var t table
		return t, row.Scan(&t.name, &t.bytes)
	})
	if err != nil {
		return nil, fmt.Errorf("listing run tables: %w", err)
	}
	sizes := map[int64]int64{}
	for _, t := range list {
		rows, err := a.pool.Query(ctx, "SELECT run_id, count(*) FROM "+pgx.Identifier{t.name}.Sanitize()+" GROUP BY run_id")
		if err != nil {
			return nil, fmt.Errorf("counting %s by run: %w", t.name, err)
		}
		type count struct{ run, rows int64 }
		counts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (count, error) {
			var c count
			return c, row.Scan(&c.run, &c.rows)
		})
		if err != nil {
			return nil, fmt.Errorf("counting %s by run: %w", t.name, err)
		}
		var total int64
		for _, c := range counts {
			total += c.rows
		}
		for _, c := range counts {
			sizes[c.run] += t.bytes * c.rows / total
		}
	}
	return sizes, nil
}
