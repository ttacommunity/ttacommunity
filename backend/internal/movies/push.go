// YouTube's push notifications (WebSub): the topic we subscribe a channel by, the hub's signature on what it
// pushes, and the Atom it pushes for an upload, a changed title or description, or a deletion.
package movies

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"hash"
	"regexp"
	"strings"
	"time"
)

// Hub is YouTube's WebSub hub.
const Hub = "https://pubsubhubbub.appspot.com/subscribe"

const topicPrefix = "https://www.youtube.com/xml/feeds/videos.xml?channel_id="

var videoIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// PushedEntry is one upload, or a video whose title or description changed, as the hub pushed it.
type PushedEntry struct {
	VideoID     string
	ChannelID   string
	Title       string
	PublishedAt time.Time
}

// DeletedEntry is a video the hub says its channel deleted.
type DeletedEntry struct {
	VideoID   string
	ChannelID string
}

// Push is what one notification carries.
type Push struct {
	Entries []PushedEntry
	Deleted []DeletedEntry
}

// Topic is the feed we subscribe channelID's uploads by.
func Topic(channelID string) string { return topicPrefix + channelID }

// TopicChannel returns the channel id in topic, when topic is exactly a channel's feed as Topic writes it.
func TopicChannel(topic string) (string, bool) {
	id, ok := strings.CutPrefix(topic, topicPrefix)
	if !ok || !channelIDPattern.MatchString(id) {
		return "", false
	}
	return id, true
}

// VerifySignature reports whether header, WebSub's X-Hub-Signature ("sha1=<hex>" or a SHA-2 method), is
// body's HMAC by secret. An empty secret verifies nothing.
func VerifySignature(secret []byte, header string, body []byte) bool {
	if len(secret) == 0 {
		return false
	}
	method, sum, ok := strings.Cut(header, "=")
	if !ok {
		return false
	}
	var h func() hash.Hash
	switch method {
	case "sha1":
		h = sha1.New
	case "sha256":
		h = sha256.New
	case "sha384":
		h = sha512.New384
	case "sha512":
		h = sha512.New
	default:
		return false
	}
	got, err := hex.DecodeString(sum)
	if err != nil {
		return false
	}
	mac := hmac.New(h, secret)
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// ParsePush reads a notification, skipping any entry without a valid video id, channel id and publish time.
func ParsePush(data []byte) (Push, error) {
	var feed struct {
		Entries []struct {
			VideoID   string `xml:"http://www.youtube.com/xml/schemas/2015 videoId"`
			ChannelID string `xml:"http://www.youtube.com/xml/schemas/2015 channelId"`
			Title     string `xml:"title"`
			Published string `xml:"published"`
		} `xml:"entry"`
		Deleted []struct {
			Ref string `xml:"ref,attr"`
			By  struct {
				URI string `xml:"uri"`
			} `xml:"http://purl.org/atompub/tombstones/1.0 by"`
		} `xml:"http://purl.org/atompub/tombstones/1.0 deleted-entry"`
	}
	if err := xml.Unmarshal(data, &feed); err != nil {
		return Push{}, fmt.Errorf("parsing a push: %w", err)
	}
	var push Push
	for _, e := range feed.Entries {
		pub, err := time.Parse(time.RFC3339, strings.TrimSpace(e.Published))
		if err != nil || !videoIDPattern.MatchString(e.VideoID) || !channelIDPattern.MatchString(e.ChannelID) {
			continue
		}
		push.Entries = append(push.Entries, PushedEntry{
			VideoID: e.VideoID, ChannelID: e.ChannelID, Title: strings.TrimSpace(e.Title), PublishedAt: pub.UTC(),
		})
	}
	for _, d := range feed.Deleted {
		video, _ := strings.CutPrefix(d.Ref, "yt:video:")
		channel, ok := ParseChannelID(d.By.URI)
		if !videoIDPattern.MatchString(video) || !ok {
			continue
		}
		push.Deleted = append(push.Deleted, DeletedEntry{VideoID: video, ChannelID: channel})
	}
	return push, nil
}
