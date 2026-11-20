// Players' reports: what a player may do with their own match's result. The database decides; this tells the site
// which buttons to show.
package blitz

// PlayerMatch is a match as one of its players sees it: their side (a or b), whether it has a result, where a
// player's report stands ("" for none) and which side reported it, whether it is a bye, and whether the next round
// is paired.
type PlayerMatch struct {
	Me, ReportedBy, Report string
	Decided, Bye, Closed   bool
}

// PlayerActions returns what the player may do now: report (or correct their own report), or confirm or flag the
// opponent's; nil once players can no longer change it.
func PlayerActions(m PlayerMatch) []string {
	switch {
	case m.Bye || m.Closed:
		return nil
	case !m.Decided:
		return []string{"report"}
	case m.Report != "reported":
		return nil
	case m.ReportedBy == m.Me:
		return []string{"report"}
	}
	return []string{"confirm", "flag"}
}

// BestOfMatch is a match as one of its players sees it for best of 3: their side, the side proposing it ("" for
// none), whether the round offers it, the games agreed (1 or 3) and recorded, and whether the match is decided (by its
// games or a ruling), a bye, or in a round the next round followed.
type BestOfMatch struct {
	Me, ProposedBy       string
	Offered              bool
	BestOf, Played       int
	Decided, Bye, Closed bool
}

// BestOfActions returns what the player may do about best of 3 now: propose it, withdraw their proposal, or accept or
// decline the opponent's; nil once it is settled (agreed, or game 1 or a ruling in) or not offered.
func BestOfActions(m BestOfMatch) []string {
	switch {
	case !m.Offered || m.BestOf > 1 || m.Played > 0 || m.Decided || m.Bye || m.Closed:
		return nil
	case m.ProposedBy == "":
		return []string{"propose"}
	case m.ProposedBy == m.Me:
		return []string{"withdraw"}
	}
	return []string{"accept", "decline"}
}

// NextGame returns the number of the game the player may report next, 0 when the match takes no more: decided, a bye,
// closed, or its agreed games all played.
func NextGame(m BestOfMatch) int {
	if m.Decided || m.Bye || m.Closed || m.Played >= m.BestOf {
		return 0
	}
	return m.Played + 1
}
