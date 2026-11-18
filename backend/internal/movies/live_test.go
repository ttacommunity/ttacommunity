package movies

import (
	"testing"
	"time"
)

func TestNextCheck(t *testing.T) {
	now := time.Date(2026, 11, 14, 14, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { v := now.Add(d); return &v }
	tests := []struct {
		name  string
		b     Broadcast
		noted time.Time
		want  *time.Time
	}{
		{name: "ended", b: Broadcast{Content: "none", StartedAt: at(-time.Hour), EndedAt: at(-time.Minute)}, noted: now, want: nil},
		{name: "live", b: Broadcast{Content: "live", StartedAt: at(-time.Hour)}, noted: now, want: at(5 * time.Minute)},
		{name: "scheduled for tomorrow", b: Broadcast{Content: "upcoming", ScheduledAt: at(24 * time.Hour)}, noted: now,
			want: at(24*time.Hour - 15*time.Minute)},
		{name: "starting in ten minutes", b: Broadcast{Content: "upcoming", ScheduledAt: at(10 * time.Minute)}, noted: now, want: at(time.Minute)},
		{name: "half an hour late", b: Broadcast{Content: "upcoming", ScheduledAt: at(-30 * time.Minute)}, noted: now, want: at(time.Minute)},
		{name: "three hours late", b: Broadcast{Content: "upcoming", ScheduledAt: at(-3 * time.Hour)}, noted: now, want: at(15 * time.Minute)},
		{name: "a day late", b: Broadcast{Content: "upcoming", ScheduledAt: at(-25 * time.Hour)}, noted: now, want: nil},
		{name: "upcoming, no time, noted an hour ago", b: Broadcast{Content: "upcoming"}, noted: now.Add(-time.Hour), want: at(15 * time.Minute)},
		{name: "upcoming, no time, noted two days ago", b: Broadcast{Content: "upcoming"}, noted: now.Add(-48 * time.Hour), want: nil},
		{name: "not a stream", b: Broadcast{Content: "none"}, noted: now, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextCheck(tt.b, tt.noted, now)
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("NextCheck(%+v) = %v, want no more checks", tt.b, *got)
			case tt.want != nil && (got == nil || !got.Equal(*tt.want)):
				t.Errorf("NextCheck(%+v) = %v, want %v", tt.b, got, *tt.want)
			}
		})
	}
}

func TestLiveOrUpcoming(t *testing.T) {
	tests := []struct {
		content string
		want    bool
	}{{"live", true}, {"upcoming", true}, {"none", false}, {"", false}}
	for _, tt := range tests {
		if got := (Broadcast{Content: tt.content}).LiveOrUpcoming(); got != tt.want {
			t.Errorf("Broadcast{Content: %q}.LiveOrUpcoming() = %v, want %v", tt.content, got, tt.want)
		}
	}
}
