package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"

	"ttacommunity/backend/internal/store"
)

// deleter answers each DeleteRun with the next of its answers (nil once they run out), and records the runs asked.
type deleter struct {
	answers []error
	deleted []int64
	marked  []int64
}

func (d *deleter) DeleteRun(_ context.Context, run int64) error {
	d.deleted = append(d.deleted, run)
	if len(d.answers) == 0 {
		return nil
	}
	err := d.answers[0]
	d.answers = d.answers[1:]
	return err
}

func (d *deleter) RunsMarkedForDeletion(context.Context) ([]int64, error) { return d.marked, nil }

func TestDeletingARunTriesAgainWhileItIsBusy(t *testing.T) {
	stop := errors.New("given up")
	tests := []struct {
		name      string
		answers   []error
		waitErr   error
		want      error
		tries     int
		waitsLeft int
	}{
		{"deleted at once", nil, nil, nil, 1, 0},
		{"busy twice, then deleted", []error{store.ErrRunBusy, store.ErrRunBusy}, nil, nil, 3, 2},
		{"refused, not tried again", []error{store.ErrRunProtected}, nil, store.ErrRunProtected, 1, 0},
		{"busy until the wait gives up", []error{store.ErrRunBusy, store.ErrRunBusy}, stop, stop, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &deleter{answers: tt.answers}
			waits := 0
			err := deleteRun(context.Background(), d, 60, func(context.Context) error {
				waits++
				return tt.waitErr
			})
			if !errors.Is(err, tt.want) {
				t.Errorf("deleteRun = %v, want %v", err, tt.want)
			}
			if len(d.deleted) != tt.tries || waits != tt.waitsLeft {
				t.Errorf("tried %d times and waited %d, want %d and %d", len(d.deleted), waits, tt.tries, tt.waitsLeft)
			}
		})
	}
}

func TestFinishDeletesDeletesEveryRunStillMarked(t *testing.T) {
	d := &deleter{marked: []int64{58, 60}}
	FinishDeletes(context.Background(), d, slog.New(slog.DiscardHandler))
	if !slices.Equal(d.deleted, []int64{58, 60}) {
		t.Errorf("deleted = %v, want [58 60]", d.deleted)
	}
}
