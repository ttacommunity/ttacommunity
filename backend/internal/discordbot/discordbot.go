// Package discordbot holds what the Discord bot answers and how it knows a request is Discord's: the signature
// check and the text of /rating's replies. It knows nothing of the database or HTTP.
package discordbot

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"
)

// ParsePublicKey returns the application's Ed25519 public key from the hex Discord shows.
func ParsePublicKey(hexKey string) (ed25519.PublicKey, error) {
	b, err := hex.DecodeString(hexKey)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("not an Ed25519 public key in hex (64 characters)")
	}
	return ed25519.PublicKey(b), nil
}

// Verify reports whether signature, in hex, is Discord's by key over timestamp followed by body.
func Verify(key ed25519.PublicKey, signature, timestamp string, body []byte) bool {
	sig, err := hex.DecodeString(signature)
	if err != nil || len(sig) != ed25519.SignatureSize || len(key) != ed25519.PublicKeySize || timestamp == "" {
		return false
	}
	return ed25519.Verify(key, append([]byte(timestamp), body...), sig)
}

// Summary is what /rating shows of a player in one format.
type Summary struct {
	Name, Title         string
	Format              int
	Rank                *int
	Rating, RD, CRating float64
	Games, Opponents    int
	WinRate             float64
	LastPlayed          *time.Time
}

var markdown = strings.NewReplacer(`\`, `\\`, "*", `\*`, "_", `\_`, "~", `\~`, "`", "\\`", "|", `\|`, ">", `\>`,
	"<", `\<`, "#", `\#`, "[", `\[`, "]", `\]`, "(", `\(`, ")", `\)`, "-", `\-`)

// EscapeMarkdown makes text show as written in a Discord message, whatever characters a name holds.
func EscapeMarkdown(text string) string { return markdown.Replace(text) }

// PlayerURL is the site's page for the player name.
func PlayerURL(site, name string) string {
	return strings.TrimSuffix(site, "/") + "/ratings/player/?" + url.Values{"name": {name}}.Encode()
}

// SearchTerm is what to ask the player search for, from what was typed so far: at most 100 characters.
func SearchTerm(typed string) string {
	t := []rune(strings.TrimSpace(typed))
	if len(t) > 100 {
		t = t[:100]
	}
	return string(t)
}

// FormatLabel names a format as the site does: 0 is every format together.
func FormatLabel(format int) string {
	if format == 0 {
		return "all formats"
	}
	return fmt.Sprintf("%d-player", format)
}

// RatingText is the reply for s, linking the player's page on site.
func RatingText(s Summary, site string) string {
	head := "**" + EscapeMarkdown(s.Name) + "**"
	if s.Title != "" {
		head += " [" + s.Title + "]"
	}
	rank := "Not ranked"
	if s.Rank != nil {
		rank = fmt.Sprintf("Rank #%d", *s.Rank)
	}
	last := ""
	if s.LastPlayed != nil {
		last = " · last game " + s.LastPlayed.Format(time.DateOnly)
	}
	return fmt.Sprintf("%s · %s, Glicko-D\n%s · Rating %.0f ± %.0f · C-Rating %.0f\n%d games · %d opponents · %.1f%% won%s\n<%s>",
		head, FormatLabel(s.Format), rank, math.Round(s.Rating), math.Round(s.RD), math.Round(s.CRating),
		s.Games, s.Opponents, s.WinRate, last, PlayerURL(site, s.Name))
}

// OwnRatingProblem says why a Discord account in state cannot be shown its own rating, or "" when it can.
func OwnRatingProblem(state, site string) string {
	account := strings.TrimSuffix(site, "/") + "/account/"
	switch state {
	case "found":
		return ""
	case "no_player":
		return "Your TTA name is linked, but it has no rated games yet."
	case "not_linked":
		return "Your Discord account is not linked to a TTA name yet: ask for yours on " + account +
			", or look up a name with `/rating <name>`"
	}
	return "Sign in with Discord on " + account + " and link your TTA name, or look up a name with `/rating <name>`"
}
