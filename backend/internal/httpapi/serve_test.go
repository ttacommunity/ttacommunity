package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServeAnswersThenStopsWhenContextIsCancelled(t *testing.T) {
	client := &http.Client{Timeout: 2 * time.Second}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, NewMux()) }()

	resp, err := client.Get("http://" + ln.Addr().String() + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz while serving: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /healthz while serving: code = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve after cancel: err = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return within 5s of cancel")
	}

	if _, err := client.Get("http://" + ln.Addr().String() + "/healthz"); err == nil {
		t.Error("GET /healthz after shutdown: err = nil, want connection refused")
	}
}

func TestServeReturnsListenerError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ln.Close()

	err = Serve(context.Background(), ln, NewMux())
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		t.Errorf("Serve on a closed listener: err = %v, want a listener error", err)
	}
}
