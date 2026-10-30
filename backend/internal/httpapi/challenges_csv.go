// Every challenge entry not hidden as a CSV file, each with its challenge beside it.
package httpapi

import (
	"bytes"
	"context"
	"encoding/csv"
	"strconv"
	"time"

	"ttacommunity/backend/internal/store"
)

var challengeCSVHeader = []string{
	"id", "entered_at", "player", "set", "group", "challenge_id", "challenge", "players", "domination", "verified", "proof_url", "bugged",
}

// GetChallengeEntriesCsv returns the entries GetChallengeEntries lists as a CSV file, with each entry's challenge
// named; an entry of a challenge not in the list is left out.
func (s *server) GetChallengeEntriesCsv(ctx context.Context, _ GetChallengeEntriesCsvRequestObject) (GetChallengeEntriesCsvResponseObject, error) {
	list, err := s.ratings.Challenges(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := s.ratings.ChallengeEntries(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]store.ChallengeRow, len(list))
	for _, c := range list {
		byID[c.ID] = c
	}
	var body bytes.Buffer
	body.WriteString("\ufeff") // a byte-order mark, so spreadsheets read UTF-8 names correctly
	w := csv.NewWriter(&body)
	w.UseCRLF = true
	if err := w.Write(challengeCSVHeader); err != nil {
		return nil, err
	}
	for _, e := range entries {
		c, ok := byID[e.Challenge]
		if !ok {
			continue
		}
		if err := w.Write(challengeCSVRow(e, c)); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return GetChallengeEntriesCsv200TextcsvResponse{
		Body:          &body,
		ContentLength: int64(body.Len()),
		Headers: GetChallengeEntriesCsv200ResponseHeaders{
			CacheControl:       cacheCurrent,
			ContentDisposition: `attachment; filename="tta_challenge_entries.csv"`,
		},
	}, nil
}

// challengeCSVRow writes one entry's row, its name guarded against spreadsheet formulas.
func challengeCSVRow(e store.ChallengeEntry, c store.ChallengeRow) []string {
	return []string{
		strconv.FormatInt(e.ID, 10), e.At.UTC().Format(time.RFC3339), text(e.Player), c.Set, c.Group, c.ID, text(c.Name),
		strconv.Itoa(c.Players), strconv.Itoa(e.Domination), yesNo(e.ProofURL != ""), e.ProofURL, yesNo(e.Bugged),
	}
}

// yesNo writes a flag as the old site's download did.
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
