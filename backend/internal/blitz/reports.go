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
