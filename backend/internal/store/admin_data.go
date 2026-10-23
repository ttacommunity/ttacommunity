// The data at a glance, for admin: how many games and players are loaded, and when.

package store

import (
	"context"
	"fmt"
	"io"
	"time"
)

// DataSummary is what admin's Data card shows. Games are played games; NeverPlayed are kept but not rated;
// Unplaced are games in no tournament, which stop a rating run; LastLoad is the load job's latest import (games
// entered in admin or by players aside), and LastLoadRefused the rows it refused. The times are nil before anything
// is loaded. DatabaseBytes is the database's size on disk, and RunsDiskBytes what the runs take of it, as each
// recorded as it ended.
type DataSummary struct {
	Games, NeverPlayed, Unplaced, Players int
	LatestGame                            *time.Time
	LastLoad                              *time.Time
	LastLoadRefused                       int
	DatabaseBytes, RunsDiskBytes          int64
}

// DataSummary counts the loaded games and players, when they were last loaded, and the room the database and
// its runs take.
func (a *AdminStore) DataSummary(ctx context.Context) (DataSummary, error) {
	var d DataSummary
	err := a.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE state = 'played'),
		       count(*) FILTER (WHERE state <> 'played'),
		       count(*) FILTER (WHERE group_id IS NULL),
		       (SELECT count(*) FROM players),
		       max(date) FILTER (WHERE state = 'played'),
		       (SELECT max(created_at) FROM import_batches WHERE source = 'load'),
		       (SELECT count(*) FROM refused_rows WHERE batch_id = (SELECT max(batch_id) FROM import_batches WHERE source = 'load')),
		       pg_database_size(current_database()),
		       (SELECT coalesce(sum(disk_bytes), 0) FROM rating_runs)
		FROM games`).Scan(&d.Games, &d.NeverPlayed, &d.Unplaced, &d.Players, &d.LatestGame, &d.LastLoad, &d.LastLoadRefused,
		&d.DatabaseBytes, &d.RunsDiskBytes)
	if err != nil {
		return DataSummary{}, fmt.Errorf("summarising the data: %w", err)
	}
	return d, nil
}

// ExportZip writes every input table, as the load job's export, as a zip to w, through the admin login.
func (a *AdminStore) ExportZip(ctx context.Context, w io.Writer) error {
	_, err := ExportZip(ctx, a.snapshot, w)
	return err
}
