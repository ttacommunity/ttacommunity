package runcache

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

type use struct {
	run int64
	key string
}

func TestGetLoadsEachRunAndKeyOnceWhileTheRunIsKept(t *testing.T) {
	tests := []struct {
		name  string
		uses  []use
		loads []bool
	}{
		{"a value is kept", []use{{1, "a"}, {1, "a"}}, []bool{true, false}},
		{"each key is kept on its own", []use{{1, "a"}, {1, "b"}, {1, "a"}, {1, "b"}}, []bool{true, true, false, false}},
		{"two runs are kept", []use{{1, "a"}, {2, "a"}, {1, "a"}, {2, "a"}}, []bool{true, true, false, false}},
		{"a third run drops the least recently used", []use{{1, "a"}, {2, "a"}, {3, "a"}, {1, "a"}}, []bool{true, true, true, true}},
		{"using a run keeps it", []use{{1, "a"}, {2, "a"}, {1, "b"}, {3, "a"}, {1, "a"}, {2, "a"}}, []bool{true, true, true, true, false, true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New[string, int64]()
			var got []bool
			for _, u := range tt.uses {
				loaded := false
				v, err := c.Get(u.run, u.key, func() (int64, error) {
					loaded = true
					return u.run, nil
				})
				if err != nil || v != u.run {
					t.Fatalf("Get(%d, %q) = %d, %v; want %d", u.run, u.key, v, err, u.run)
				}
				got = append(got, loaded)
			}
			if !slices.Equal(got, tt.loads) {
				t.Errorf("uses %v loaded %v, want %v", tt.uses, got, tt.loads)
			}
		})
	}
}

func TestGetKeepsNoFailure(t *testing.T) {
	c := New[string, int]()
	failed := errors.New("database gone")
	if _, err := c.Get(1, "a", func() (int, error) { return 0, failed }); !errors.Is(err, failed) {
		t.Fatalf("Get with a failing load = %v, want %v", err, failed)
	}
	loads := 0
	v, err := c.Get(1, "a", func() (int, error) { loads++; return 7, nil })
	if err != nil || v != 7 || loads != 1 {
		t.Errorf("Get after a failure = %d, %v with %d loads; want 7, nil with 1", v, err, loads)
	}
}

func TestGetLoadsOnceForCallersAskingTogether(t *testing.T) {
	c := New[string, int]()
	release := make(chan struct{})
	var mu sync.Mutex
	loads := 0
	load := func() (int, error) {
		mu.Lock()
		loads++
		mu.Unlock()
		<-release
		return 7, nil
	}
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			if v, err := c.Get(1, "a", load); err != nil || v != 7 {
				t.Errorf("Get = %d, %v; want 7, nil", v, err)
			}
		})
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	if loads != 1 {
		t.Errorf("5 callers together loaded %d times, want 1", loads)
	}
}

func TestGetDoesNotMakeOtherKeysWaitForALoad(t *testing.T) {
	c := New[string, int]()
	started, release := make(chan struct{}), make(chan struct{})
	go func() {
		_, _ = c.Get(1, "slow", func() (int, error) {
			close(started)
			<-release
			return 1, nil
		})
	}()
	<-started
	defer close(release)
	done := make(chan struct{})
	go func() {
		_, _ = c.Get(1, "fast", func() (int, error) { return 2, nil })
		_, _ = c.Get(2, "fast", func() (int, error) { return 2, nil })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("Get of other keys waited for another key's load")
	}
}
