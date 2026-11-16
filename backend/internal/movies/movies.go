// Package movies reads YouTube channels' videos (pushed by YouTube's hub, and from the Data API) and decides
// which are about Through the Ages.
package movies

import (
	"regexp"
	"strings"
)

var (
	channelIDPattern = regexp.MustCompile(`^UC[A-Za-z0-9_-]{22}$`)
	channelInURL     = regexp.MustCompile(`/channel/(UC[A-Za-z0-9_-]{22})`)
	ttaWord          = regexp.MustCompile(`(?i)\btta\b`)
)

// ParseChannelID extracts a YouTube UC... channel id from a bare id or /channel/ URL.
func ParseChannelID(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if channelIDPattern.MatchString(s) {
		return s, true
	}
	if m := channelInURL.FindStringSubmatch(s); len(m) == 2 {
		return m[1], true
	}
	return "", false
}

// VideoText is what a video says about itself: its title, description and tags.
type VideoText struct {
	Title       string
	Description string
	Tags        []string
}

// Verdict is whether a video is about TTA, and why.
type Verdict struct {
	TTA    bool
	Reason string
}

// Classify decides whether a video is about TTA: every video of an includeAll channel is, and any other when
// its text mentions the game or names one of tournaments (a name of 4 letters or more).
func Classify(v VideoText, includeAll bool, tournaments []string) Verdict {
	if includeAll {
		return Verdict{true, "the channel's every video counts"}
	}
	haystack := strings.ToLower(v.Title + "\n" + v.Description + "\n" + strings.Join(v.Tags, "\n"))
	for _, needle := range []string{"#ttahub", "#throughtheages", "through the ages"} {
		if strings.Contains(haystack, needle) {
			return Verdict{true, "mentions Through the Ages"}
		}
	}
	if ttaWord.MatchString(haystack) {
		return Verdict{true, "mentions TTA"}
	}
	for _, name := range tournaments {
		n := strings.TrimSpace(strings.ToLower(name))
		if len(n) >= 4 && strings.Contains(haystack, n) {
			return Verdict{true, "names the tournament " + strings.TrimSpace(name)}
		}
	}
	return Verdict{false, "no mention of TTA"}
}

// Thumbnail is the address of videoID's thumbnail on YouTube's image host.
func Thumbnail(videoID string) string { return "https://i.ytimg.com/vi/" + videoID + "/hqdefault.jpg" }
