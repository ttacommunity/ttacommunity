package challenges

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

// The bounds of an entry, as the old site and the database keep them.
const (
	MinDomination = -9999
	MaxDomination = 9999
)

// ImportedEntry is an entry from the old site, by its id there: the challenge, the name it was entered under, the
// domination, its proof link ("" for none), when it was entered, how ("sheet" or "web"), and admins' flags.
type ImportedEntry struct {
	OldID        int64
	Challenge    string
	Name         string
	Domination   int
	ProofURL     string
	At           time.Time
	Source       string
	Hidden       bool
	HiddenReason string
	Bugged       bool
	Checked      bool
}

// Refusal is an entry ReadEntries left out, by its id in the file (0 when it has none), and why.
type Refusal struct {
	OldID  int64
	Reason string
}

// EntriesRead is what ReadEntries found: the entries in the file, those it kept, and those it refused.
type EntriesRead struct {
	Rows    int
	Entries []ImportedEntry
	Refused []Refusal
}

// oldEntry is one entry as the old site's admin list sends it; its public list sends the first seven fields.
type oldEntry struct {
	ID           int64   `json:"id"`
	Challenge    string  `json:"c"`
	Player       string  `json:"p"`
	Domination   *int    `json:"s"`
	Proof        *string `json:"u"`
	At           string  `json:"t"`
	Bugged       int     `json:"b"`
	Hidden       bool    `json:"hidden"`
	HiddenReason *string `json:"hiddenReason"`
	Source       string  `json:"source"`
	Checked      bool    `json:"checked"`
}

// ReadEntries reads the old site's entries, a JSON list as its admin list or public list sends them, keeping each
// it could store and refusing the rest. A missing source is "web" and missing flags are off, as the old site's
// database gave a new entry. Names have their spaces collapsed; nothing else about a name is checked.
func ReadEntries(r io.Reader) (EntriesRead, error) {
	var raw []json.RawMessage
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return EntriesRead{}, fmt.Errorf("the entries are not a JSON list: %w", err)
	}
	got := EntriesRead{Rows: len(raw)}
	seen := map[int64]bool{}
	for _, msg := range raw {
		var o oldEntry
		if err := json.Unmarshal(msg, &o); err != nil {
			got.Refused = append(got.Refused, Refusal{OldID: o.ID, Reason: "not an entry: " + err.Error()})
			continue
		}
		e, problem := imported(o)
		if problem == "" && seen[o.ID] {
			problem = fmt.Sprintf("id %d is listed twice", o.ID)
		}
		if problem != "" {
			got.Refused = append(got.Refused, Refusal{OldID: o.ID, Reason: problem})
			continue
		}
		seen[o.ID] = true
		got.Entries = append(got.Entries, e)
	}
	return got, nil
}

// imported is o as an entry to store, or what about it cannot be stored.
func imported(o oldEntry) (ImportedEntry, string) {
	e := ImportedEntry{OldID: o.ID, Challenge: o.Challenge, Name: strings.Join(strings.Fields(o.Player), " "),
		Source: o.Source, Hidden: o.Hidden, Bugged: o.Bugged != 0, Checked: o.Checked}
	if e.Source == "" {
		e.Source = "web"
	}
	if o.Proof != nil {
		e.ProofURL = strings.TrimSpace(*o.Proof)
	}
	if o.HiddenReason != nil {
		e.HiddenReason = strings.TrimSpace(*o.HiddenReason)
	}
	at, err := time.Parse(time.RFC3339, o.At)
	switch {
	case e.Challenge == "":
		return e, "no challenge"
	case e.Name == "" || utf8.RuneCountInString(e.Name) > 100:
		return e, "the name is empty or over 100 characters"
	case o.Domination == nil:
		return e, "no domination"
	case *o.Domination < MinDomination || *o.Domination > MaxDomination:
		return e, fmt.Sprintf("the domination is not %d to %d", MinDomination, MaxDomination)
	case e.ProofURL != "" && (!strings.HasPrefix(e.ProofURL, "https://") || len(e.ProofURL) > 500):
		return e, "the proof is not an https link of at most 500 characters"
	case err != nil:
		return e, "the time is not an RFC 3339 time"
	case e.Source != "sheet" && e.Source != "web":
		return e, "the source is not sheet or web"
	case utf8.RuneCountInString(e.HiddenReason) > 500:
		return e, "the reason it is hidden is over 500 characters"
	}
	e.Domination, e.At = *o.Domination, at.UTC()
	return e, ""
}
