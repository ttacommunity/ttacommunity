// Deleting rating runs in the background: a delete that meets a busy lock, such as the engine's while it writes
// a new run, tries again; FinishDeletes finishes the runs still marked, as after a restart.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ttacommunity/backend/internal/store"
)

// deleteRetry is how long a delete that met a busy lock waits before it tries again.
const deleteRetry = 30 * time.Second

// runDeleter deletes a run marked for deletion.
type runDeleter interface {
	DeleteRun(ctx context.Context, run int64) error
}

// deleteRun deletes the marked run, calling wait and trying again while its tables are busy, until it is
// deleted, refused, or wait gives up.
func deleteRun(ctx context.Context, d runDeleter, run int64, wait func(context.Context) error) error {
	for {
		err := d.DeleteRun(ctx, run)
		if !errors.Is(err, store.ErrRunBusy) {
			return err
		}
		if err := wait(ctx); err != nil {
			return fmt.Errorf("deleting run %d, busy: %w", run, err)
		}
	}
}

// waitRetry waits deleteRetry, or until ctx ends.
func waitRetry(ctx context.Context) error {
	select {
	case <-time.After(deleteRetry):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// deleteInBackground deletes the marked run on its own, for up to deleteRunTimeout, logging a failure.
func deleteInBackground(d runDeleter, run int64, log *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), deleteRunTimeout)
	defer cancel()
	if err := deleteRun(ctx, d, run, waitRetry); err != nil {
		log.Error("deleting a run failed; it stays marked, to delete again", "run", run, "err", err)
	}
}

// FinishDeletes deletes, one at a time, every run marked for deletion and not yet deleted, as one whose delete
// was cut short by a restart.
func FinishDeletes(ctx context.Context, d interface {
	runDeleter
	RunsMarkedForDeletion(ctx context.Context) ([]int64, error)
}, log *slog.Logger) {
	runs, err := d.RunsMarkedForDeletion(ctx)
	if err != nil {
		log.Error("listing runs marked for deletion failed", "err", err)
		return
	}
	for _, run := range runs {
		deleteInBackground(d, run, log)
	}
}
