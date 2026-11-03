// The summary of a CGE delivery's load: what it held and what it changed, as text or JSON, naming no player.
package main

import (
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"ttacommunity/backend/internal/store"
)

// delivery is what readDelivery found in a folder: the load's input and the files it was read from.
type delivery struct {
	Dir   string
	Files []deliveryFile
	In    store.CGEInput
}

// deliveryFile is one file read: its name, its rows, and a changes file's rows by change.
type deliveryFile struct {
	Name    string         `json:"name"`
	Rows    int            `json:"rows"`
	Changes map[string]int `json:"changes,omitempty"`
}

// cgeSummary is what a load of a delivery found and changed, by kind; ids and counts only.
type cgeSummary struct {
	Delivery    string         `json:"delivery"`
	DryRun      bool           `json:"dry_run"`
	Batch       int64          `json:"batch,omitempty"`
	Files       []deliveryFile `json:"files"`
	Refused     []refusedRow   `json:"refused"`
	Tournaments struct {
		InDelivery int   `json:"in_delivery"`
		Added      int   `json:"added"`
		Changed    int   `json:"changed"`
		Emptied    []int `json:"emptied"`
	} `json:"tournaments"`
	Stages struct {
		TakingCGE int                 `json:"taking_cge"`
		Changed   int                 `json:"changed"`
		Disagree  []stageDisagreement `json:"disagree"`
	} `json:"stages"`
	Games struct {
		InDelivery    int            `json:"in_delivery"`
		Added         map[string]int `json:"added"`
		Changed       int            `json:"changed"`
		Unchanged     int            `json:"unchanged"`
		StateChanges  map[string]int `json:"state_changes"`
		Removed       []int64        `json:"removed"`
		RemovedPlayed int            `json:"removed_played"`
		Missing       []int64        `json:"missing"`
		HeldBack      []string       `json:"held_back"`
		Partial       int            `json:"partial"`
		RunHere       int            `json:"run_here"`
	} `json:"games"`
	Seats struct {
		Added   int `json:"added"`
		Dropped int `json:"dropped"`
	} `json:"seats"`
	Accounts struct {
		Added    int `json:"added"`
		Relinked int `json:"relinked"`
	} `json:"accounts"`
	Players struct {
		Added   int `json:"added"`
		Renamed int `json:"renamed"`
	} `json:"players"`
	Members struct {
		Accounts int `json:"accounts"`
		Players  int `json:"players"`
		Renamed  int `json:"renamed"`
	} `json:"members_linked"`
	Groups struct {
		Linked     int `json:"linked"`
		GamesMoved int `json:"games_moved"`
	} `json:"groups"`
	Kept struct {
		Moderators map[int]int `json:"moderators"`
		Rules      map[int]int `json:"special_rules"`
	} `json:"kept_though_cge_no_longer_lists"`
	CopiesRemoved     int `json:"copies_removed"`
	TournamentsMerged int `json:"tournaments_merged"`
	SeriesAdded       int `json:"series_added"`
}

// refusedRow is a row the load did not use, by file and line.
type refusedRow struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Reason string `json:"reason"`
}

// stageDisagreement is a stage of ours whose CGE stages disagree on the settings named, which it keeps.
type stageDisagreement struct {
	StageID  int64    `json:"stage_id"`
	Stage    string   `json:"stage"`
	Settings []string `json:"settings"`
}

// newCGESummary sums up a load of d: the plan's, the stages' (PlanCGEStages') and what the load wrote (got), which
// was written with written, else rolled back.
func newCGESummary(d delivery, plan store.CGEPlan, stages []store.CGEStageWrite, got store.CGEApplied, written bool) cgeSummary {
	var s cgeSummary
	s.Delivery, s.DryRun, s.Files = filepath.Base(d.Dir), !written, d.Files
	if written {
		s.Batch = got.BatchID
	}
	for _, r := range d.In.Extract.Refused {
		s.Refused = append(s.Refused, refusedRow{File: filepath.Base(r.File), Line: r.Line, Reason: string(r.Reason)})
	}
	s.Tournaments.InDelivery, s.Tournaments.Added = len(d.In.Extract.Tournaments), got.TournamentsAdded
	s.Tournaments.Changed, s.Tournaments.Emptied = got.TournamentsUpdated, plan.TournamentsEmptied
	s.Stages.TakingCGE, s.Stages.Changed = len(stages), got.SettingsWritten
	for _, w := range stages {
		if len(w.Disagree) > 0 {
			s.Stages.Disagree = append(s.Stages.Disagree, stageDisagreement{StageID: w.StageID, Stage: w.Stage, Settings: w.Disagree})
		}
	}
	states, codes := map[int]string{}, map[int]string{}
	for _, g := range plan.Games {
		states[g.SessionID], codes[g.SessionID] = g.State, g.ShareCode
	}
	g := &s.Games
	g.InDelivery, g.Added = len(plan.Games), map[string]int{}
	for _, session := range plan.Matching.Added {
		g.Added[states[session]]++
	}
	g.Changed, g.Unchanged, g.StateChanges = got.GamesChanged, got.GamesUnchanged, got.StateChanges
	g.Removed, g.RemovedPlayed, g.Missing = plan.Removed, plan.RemovedPlayed, plan.Missing
	for _, session := range plan.Matching.HeldBack {
		g.HeldBack = append(g.HeldBack, codes[session])
	}
	g.Partial, g.RunHere = plan.Partial, plan.RunHere
	s.Seats.Added, s.Seats.Dropped = got.SeatsAdded, got.SeatsRemoved
	s.Accounts.Added, s.Accounts.Relinked = got.AccountsAdded, got.Relinked
	s.Players.Added, s.Players.Renamed = got.PlayersAdded, got.PlayersRenamed
	s.Members.Accounts, s.Members.Players, s.Members.Renamed = got.Members.Accounts, got.Members.Players, got.Members.Renamed
	s.Groups.Linked, s.Groups.GamesMoved = got.GroupsLinked, got.GamesMoved
	s.Kept.Moderators, s.Kept.Rules = got.ModeratorsKept, got.RulesKept
	s.CopiesRemoved, s.TournamentsMerged, s.SeriesAdded = got.CopiesRemoved, got.TournamentsMerged, got.SeriesAdded
	return s
}

// writeText writes the summary for a person to read.
func (s cgeSummary) writeText(w io.Writer) {
	line := func(format string, args ...any) { fmt.Fprintf(w, format+"\n", args...) }
	if s.DryRun {
		line("CGE delivery %s (a dry run: nothing written)", s.Delivery)
	} else {
		line("CGE delivery %s (written as batch %d)", s.Delivery, s.Batch)
	}
	var files []string
	for _, f := range s.Files {
		text := fmt.Sprintf("%s %d rows", f.Name, f.Rows)
		if len(f.Changes) > 0 {
			text += fmt.Sprintf(" (%d new, %d changed, %d removed)", f.Changes["new"], f.Changes["changed"], f.Changes["removed"])
		}
		files = append(files, text)
	}
	line("files: %s", strings.Join(files, "; "))
	line("refused rows: %d", len(s.Refused))
	for _, r := range s.Refused {
		line("  %s line %d: %s", r.File, r.Line, r.Reason)
	}
	t := s.Tournaments
	line("tournaments: %d in the delivery; %d added; %d changed", t.InDelivery, t.Added, t.Changed)
	if len(t.Emptied) > 0 {
		line("  every game removed (made private?): %s", joinInts(t.Emptied))
	}
	line("stages: %d take CGE's settings; %d changed", s.Stages.TakingCGE, s.Stages.Changed)
	line("  settings their CGE stages disagree on, kept: %d", len(s.Stages.Disagree))
	for _, d := range s.Stages.Disagree {
		line("    stage %d (%s): %s", d.StageID, d.Stage, strings.Join(d.Settings, ", "))
	}
	g := s.Games
	line("games: %d in the delivery", g.InDelivery)
	line("  added: %d%s", total(g.Added), byKind(g.Added))
	line("  changed: %d%s", g.Changed, byKind(g.StateChanges))
	line("  unchanged: %d", g.Unchanged)
	line("  removed by the changes file: %d (%d played)%s", len(g.Removed), g.RemovedPlayed, listed("games", joinInts(g.Removed)))
	line("  missing from the full file, kept: %d%s", len(g.Missing), listed("games", joinInts(g.Missing)))
	line("  held back as possible copies: %d%s", len(g.HeldBack), listed("", strings.Join(g.HeldBack, ", ")))
	line("  partial games we don't hold, left out: %d", g.Partial)
	line("  in seasons run on this site, left out: %d", g.RunHere)
	line("seats: %d added; %d dropped", s.Seats.Added, s.Seats.Dropped)
	line("accounts: %d added; %d relinked", s.Accounts.Added, s.Accounts.Relinked)
	line("players: %d added; %d renamed after their account", s.Players.Added, s.Players.Renamed)
	line("members linked: %d to an account, %d to a player, %d renamed", s.Members.Accounts, s.Members.Players, s.Members.Renamed)
	line("groups: %d linked to their CGE stage; %d games moved into their CGE group", s.Groups.Linked, s.Groups.GamesMoved)
	line("moderators CGE no longer lists, kept: %s", byTournament(s.Kept.Moderators))
	line("special rules CGE no longer lists, kept: %s", byTournament(s.Kept.Rules))
	if s.CopiesRemoved+s.TournamentsMerged+s.SeriesAdded > 0 {
		line("also: %d copies an earlier load added removed; %d split tournaments merged; %d hidden series added",
			s.CopiesRemoved, s.TournamentsMerged, s.SeriesAdded)
	}
}

// total is the sum of counts.
func total(counts map[string]int) int {
	n := 0
	for _, c := range counts {
		n += c
	}
	return n
}

// byKind lists counts as " (kind n, …)", sorted by kind, or "" for none.
func byKind(counts map[string]int) string {
	var parts []string
	for _, k := range slices.Sorted(maps.Keys(counts)) {
		parts = append(parts, fmt.Sprintf("%s %d", k, counts[k]))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

// byTournament gives counts by CGE tournament as "n (tournament id: n, …)", or "none".
func byTournament(counts map[int]int) string {
	n, parts := 0, []string{}
	for _, t := range slices.Sorted(maps.Keys(counts)) {
		n += counts[t]
		parts = append(parts, fmt.Sprintf("tournament %d: %d", t, counts[t]))
	}
	if n == 0 {
		return "none"
	}
	return fmt.Sprintf("%d (%s)", n, strings.Join(parts, ", "))
}

// joinInts joins ids with ", ".
func joinInts[T int | int64](ids []T) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprint(id)
	}
	return strings.Join(parts, ", ")
}

// listed is ": what list" for a list, else "".
func listed(what, list string) string {
	if list == "" {
		return ""
	}
	if what == "" {
		return ": " + list
	}
	return ": " + what + " " + list
}
