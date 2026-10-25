// Admin's data: a summary of what is loaded, the export, and the standings check against CGE.
package httpapi

import (
	"context"
	"errors"
	"io"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"ttacommunity/backend/internal/store"
)

func (s *server) GetAdminData(ctx context.Context, _ GetAdminDataRequestObject) (GetAdminDataResponseObject, error) {
	d, err := s.admins.DataSummary(ctx)
	if err != nil {
		return nil, err
	}
	body := DataSummary{
		Games: d.Games, NeverPlayed: d.NeverPlayed, Unplaced: d.Unplaced, Players: d.Players,
		LastLoad: d.LastLoad, LastLoadRefused: d.LastLoadRefused, DatabaseBytes: d.DatabaseBytes, RunsDiskBytes: d.RunsDiskBytes,
	}
	if d.LatestGame != nil {
		body.LatestGame = &openapi_types.Date{Time: *d.LatestGame}
	}
	return GetAdminData200JSONResponse{Body: body, Headers: GetAdminData200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) GetAdminExport(ctx context.Context, _ GetAdminExportRequestObject) (GetAdminExportResponseObject, error) {
	me, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		return tx.Audit(ctx, me.DiscordID, "data.export", "all", map[string]any{})
	})
	if err != nil {
		return nil, err
	}
	r, w := io.Pipe()
	go func() {
		<-ctx.Done() // a download given up stops the export: its next write fails
		r.CloseWithError(ctx.Err())
	}()
	go func() {
		if err := s.admins.ExportZip(ctx, w); err != nil {
			s.log.Error("exporting the data failed", "err", err)
			w.CloseWithError(err)
			return
		}
		w.Close()
	}()
	return GetAdminExport200ApplicationzipResponse{
		Body: r,
		Headers: GetAdminExport200ResponseHeaders{
			CacheControl: cacheNone, ContentDisposition: `attachment; filename="ttacommunity-data.zip"`,
		},
	}, nil
}

func (s *server) GetAdminStandingsCheck(ctx context.Context, _ GetAdminStandingsCheckRequestObject) (GetAdminStandingsCheckResponseObject, error) {
	c, err := s.admins.CheckStandings(ctx)
	if err != nil {
		return nil, err
	}
	body := StandingsCheck{Stages: []StageCheck{}, StagesOnlyOnCge: c.StagesOnlyOnCGE}
	for _, st := range c.Stages {
		out := StageCheck{CgeTournament: st.CGETournament, CgeSeason: st.CGESeason, Name: st.Name,
			Players: st.Players, Agree: st.Agree, Differences: []StandingDifference{}}
		for _, d := range st.Differences {
			out.Differences = append(out.Differences, StandingDifference{
				CgeUser: d.CGEUser, Player: d.Player, Kind: StandingDifferenceKind(d.Kind),
				CgeGames: d.CGEGames, HeldGames: d.HeldGames, HeldNeverPlayed: d.HeldNeverPlayed, CgePoints: d.CGEPoints,
				CgePlaces: places(d.CGEPlaces), HeldPlaces: places(d.HeldPlaces),
			})
		}
		body.Stages = append(body.Stages, out)
	}
	return GetAdminStandingsCheck200JSONResponse{Body: body, Headers: GetAdminStandingsCheck200ResponseHeaders{CacheControl: cacheNone}}, nil
}

// places turns a player's places into the API's, null for a size not played.
func places(p store.Places) Places {
	size := func(counts []int) *[]int {
		if counts == nil {
			return nil
		}
		return &counts
	}
	return Places{P2: size(p.P2), P3: size(p.P3), P4: size(p.P4)}
}
