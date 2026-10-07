package cgeextract

import (
	"slices"
	"testing"
)

func finished(standing int, culture float64) Seat {
	return Seat{FinishStatus: Finished, Standing: &standing, Score: &culture}
}

func resigned(standing int, mark float64) Seat {
	return Seat{FinishStatus: Resigned, Standing: &standing, Score: &mark}
}

func timedOut() Seat { return Seat{FinishStatus: TimedOut} }

func TestPlacesOrderFinishersThenResignersThenTimeouts(t *testing.T) {
	tests := []struct {
		name  string
		seats []Seat
		want  []int
	}{
		{"finishers by CGE's standing", []Seat{finished(2, 150), finished(1, 200), finished(3, 90)}, []int{2, 1, 3}},
		{"a tie CGE shares stays shared", []Seat{finished(1, 150), finished(1, 150), finished(3, 90)}, []int{1, 1, 3}},
		{"a tie CGE splits stays split", []Seat{finished(2, 150), finished(1, 150)}, []int{2, 1}},
		{"standings skipping a timed-out seat", []Seat{timedOut(), finished(2, 200), finished(3, 90)}, []int{3, 1, 2}},
		{"the last to resign places above the first", []Seat{finished(1, 200), resigned(4, -9), resigned(3, -8), finished(2, 120)}, []int{1, 4, 3, 2}},
		{"timeouts last, tied", []Seat{finished(1, 200), timedOut(), timedOut(), finished(2, 100)}, []int{1, 3, 3, 2}},
		{"a resigner above two timeouts wins", []Seat{timedOut(), resigned(3, -9), timedOut()}, []int{2, 1, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Places(tt.seats); !slices.Equal(got, tt.want) {
				t.Errorf("Places = %v, want %v", got, tt.want)
			}
		})
	}
}
