package movies

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

const pushFeed = `<?xml version='1.0' encoding='UTF-8'?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns="http://www.w3.org/2005/Atom">
  <link rel="hub" href="https://pubsubhubbub.appspot.com"/>
  <title>YouTube video feed</title>
  <updated>2026-11-15T09:00:00.000000+00:00</updated>
  <entry>
    <id>yt:video:vid00000001</id>
    <yt:videoId>vid00000001</yt:videoId>
    <yt:channelId>UC0000000000000000000001</yt:channelId>
    <title>Through the Ages final</title>
    <link rel="alternate" href="https://www.youtube.com/watch?v=vid00000001"/>
    <author><name>Alice Casts</name><uri>https://www.youtube.com/channel/UC0000000000000000000001</uri></author>
    <published>2026-11-15T08:00:00+00:00</published>
    <updated>2026-11-15T08:30:00.000000+00:00</updated>
  </entry>
</feed>`

const deletedFeed = `<?xml version='1.0' encoding='UTF-8'?>
<feed xmlns:at="http://purl.org/atompub/tombstones/1.0" xmlns="http://www.w3.org/2005/Atom">
  <at:deleted-entry ref="yt:video:vid00000001" when="2026-11-15T10:00:00.000000+00:00">
    <link href="https://www.youtube.com/watch?v=vid00000001"/>
    <at:by><name>Alice Casts</name><uri>https://www.youtube.com/channel/UC0000000000000000000001</uri></at:by>
  </at:deleted-entry>
</feed>`

func TestParsePushReadsAnUpload(t *testing.T) {
	push, err := ParsePush([]byte(pushFeed))
	if err != nil {
		t.Fatalf("ParsePush: %v", err)
	}
	want := PushedEntry{
		VideoID: "vid00000001", ChannelID: "UC0000000000000000000001", Title: "Through the Ages final",
		PublishedAt: time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC),
	}
	if len(push.Entries) != 1 || push.Entries[0] != want {
		t.Errorf("entries = %+v, want [%+v]", push.Entries, want)
	}
	if len(push.Deleted) != 0 {
		t.Errorf("deleted = %+v, want none", push.Deleted)
	}
}

func TestParsePushReadsADeletion(t *testing.T) {
	push, err := ParsePush([]byte(deletedFeed))
	if err != nil {
		t.Fatalf("ParsePush: %v", err)
	}
	want := DeletedEntry{VideoID: "vid00000001", ChannelID: "UC0000000000000000000001"}
	if len(push.Deleted) != 1 || push.Deleted[0] != want {
		t.Errorf("deleted = %+v, want [%+v]", push.Deleted, want)
	}
	if len(push.Entries) != 0 {
		t.Errorf("entries = %+v, want none", push.Entries)
	}
}

func TestParsePushSkipsWhatItCannotUse(t *testing.T) {
	tests := []struct {
		name  string
		entry string
	}{
		{"a bad video id", `<yt:videoId>short</yt:videoId><yt:channelId>UC0000000000000000000001</yt:channelId><published>2026-11-15T08:00:00+00:00</published>`},
		{"a bad channel id", `<yt:videoId>vid00000001</yt:videoId><yt:channelId>someone</yt:channelId><published>2026-11-15T08:00:00+00:00</published>`},
		{"no publish time", `<yt:videoId>vid00000001</yt:videoId><yt:channelId>UC0000000000000000000001</yt:channelId>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feed := `<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns="http://www.w3.org/2005/Atom"><entry>` + tt.entry + `</entry></feed>`
			push, err := ParsePush([]byte(feed))
			if err != nil {
				t.Fatalf("ParsePush: %v", err)
			}
			if len(push.Entries) != 0 {
				t.Errorf("entries = %+v, want none", push.Entries)
			}
		})
	}
}

func TestParsePushRefusesWhatIsNotXML(t *testing.T) {
	if _, err := ParsePush([]byte("not xml <")); err == nil {
		t.Error("ParsePush(not xml) = nil error, want one")
	}
}

func TestTopicChannel(t *testing.T) {
	tests := []struct {
		name  string
		topic string
		want  string
		ok    bool
	}{
		{"our topic", "https://www.youtube.com/xml/feeds/videos.xml?channel_id=UC0000000000000000000001", "UC0000000000000000000001", true},
		{"another host", "https://example.com/xml/feeds/videos.xml?channel_id=UC0000000000000000000001", "", false},
		{"plain http", "http://www.youtube.com/xml/feeds/videos.xml?channel_id=UC0000000000000000000001", "", false},
		{"another path", "https://www.youtube.com/feeds/videos.xml?channel_id=UC0000000000000000000001", "", false},
		{"a bad channel id", "https://www.youtube.com/xml/feeds/videos.xml?channel_id=someone", "", false},
		{"an extra parameter", "https://www.youtube.com/xml/feeds/videos.xml?channel_id=UC0000000000000000000001&x=1", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := TopicChannel(tt.topic)
			if got != tt.want || ok != tt.ok {
				t.Errorf("TopicChannel(%q) = (%q, %v), want (%q, %v)", tt.topic, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestTopicRoundTrips(t *testing.T) {
	if got, ok := TopicChannel(Topic("UC0000000000000000000001")); !ok || got != "UC0000000000000000000001" {
		t.Errorf("TopicChannel(Topic(id)) = (%q, %v), want the id back", got, ok)
	}
}

func TestVerifySignature(t *testing.T) {
	secret := []byte("invented-hub-secret")
	body := []byte(pushFeed)
	sign := func(name string, sum []byte) string { return name + "=" + hex.EncodeToString(sum) }
	mac1 := hmac.New(sha1.New, secret)
	mac1.Write(body)
	mac256 := hmac.New(sha256.New, secret)
	mac256.Write(body)
	other := hmac.New(sha1.New, []byte("another-secret"))
	other.Write(body)
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{"sha1 by our secret", sign("sha1", mac1.Sum(nil)), true},
		{"sha256 by our secret", sign("sha256", mac256.Sum(nil)), true},
		{"by another secret", sign("sha1", other.Sum(nil)), false},
		{"a method we do not take", sign("md5", mac1.Sum(nil)), false},
		{"the right sum under another method", sign("sha256", mac1.Sum(nil)), false},
		{"not hex", "sha1=zz", false},
		{"no method", hex.EncodeToString(mac1.Sum(nil)), false},
		{"missing", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VerifySignature(secret, tt.header, body); got != tt.want {
				t.Errorf("VerifySignature(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

func TestVerifySignatureRefusesAnEmptySecret(t *testing.T) {
	mac := hmac.New(sha1.New, nil)
	mac.Write([]byte(pushFeed))
	if VerifySignature(nil, "sha1="+hex.EncodeToString(mac.Sum(nil)), []byte(pushFeed)) {
		t.Error("VerifySignature with no secret = true, want false")
	}
}
