// YouTube live streams: what the Data API says of a video's stream, and when to ask about it next, so a scheduled
// stream is seen starting and a live one ending without spending the day's quota.
package movies

import "time"

// How often a stream is checked: live for its end, near and just past its start every minute, later less often.
const (
	liveEvery     = 5 * time.Minute
	nearEvery     = time.Minute
	lateEvery     = 15 * time.Minute
	nearBefore    = 15 * time.Minute
	nearAfter     = time.Hour
	waitingAtMost = 24 * time.Hour
)

// Broadcast is a video's live stream as the Data API tells it: Content is live, upcoming or none (a video, or a
// stream that ended), with its scheduled start, actual start and end where YouTube gives them.
type Broadcast struct {
	Content                         string
	ScheduledAt, StartedAt, EndedAt *time.Time
}

// LiveOrUpcoming says whether the video is a stream that is live or still to come.
func (b Broadcast) LiveOrUpcoming() bool { return b.Content == "live" || b.Content == "upcoming" }

// NextCheck is when to ask about stream b next, noted at noted, at now, or nil to stop: ended or not a stream; live,
// in liveEvery; scheduled, nearBefore its start, then every minute to nearAfter it, then every lateEvery, giving up
// waitingAtMost after its start (after noted when it has none).
func NextCheck(b Broadcast, noted, now time.Time) *time.Time {
	next := func(t time.Time) *time.Time { return &t }
	switch {
	case b.EndedAt != nil:
		return nil
	case b.StartedAt != nil:
		return next(now.Add(liveEvery))
	case b.Content != "upcoming":
		return nil
	case b.ScheduledAt == nil:
		if now.After(noted.Add(waitingAtMost)) {
			return nil
		}
		return next(now.Add(lateEvery))
	}
	start := *b.ScheduledAt
	switch {
	case now.Before(start.Add(-nearBefore)):
		return next(start.Add(-nearBefore))
	case now.Before(start.Add(nearAfter)):
		return next(now.Add(nearEvery))
	case now.Before(start.Add(waitingAtMost)):
		return next(now.Add(lateEvery))
	}
	return nil
}
