package jwks_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"ttacommunity/backend/internal/jwks"
)

var now = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

var key = newKey()

func newKey() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}

// published is key in a key set as id.
func published(id string) map[string]string {
	return map[string]string{
		"kid": id, "kty": "RSA", "use": "sig",
		"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
	}
}

// server publishes whatever body returns, counting its fetches.
func server(t *testing.T, body func() (int, any)) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var fetches atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		status, b := body()
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(b)
	}))
	t.Cleanup(s.Close)
	return s, &fetches
}

func TestKeyReturnsAPublishedKeyFetchingTheSetOnce(t *testing.T) {
	s, fetches := server(t, func() (int, any) {
		return http.StatusOK, map[string]any{"keys": []map[string]string{published("key-1"), published("key-2")}}
	})
	keys := jwks.New(s.URL, s.Client(), func() time.Time { return now })
	for _, id := range []string{"key-1", "key-2", "key-1"} {
		got, err := keys.Key(context.Background(), id)
		if err != nil {
			t.Fatalf("Key(%s): %v", id, err)
		}
		if !got.Equal(&key.PublicKey) {
			t.Errorf("Key(%s) is not the published key", id)
		}
	}
	if got := fetches.Load(); got != 1 {
		t.Errorf("key set fetched %d times, want 1", got)
	}
}

func TestAnUnknownKeyFetchesTheSetAgainAtMostEveryFiveMinutes(t *testing.T) {
	ids := []string{"key-1"}
	s, fetches := server(t, func() (int, any) {
		set := []map[string]string{}
		for _, id := range ids {
			set = append(set, published(id))
		}
		return http.StatusOK, map[string]any{"keys": set}
	})
	clock := now
	keys := jwks.New(s.URL, s.Client(), func() time.Time { return clock })
	if _, err := keys.Key(context.Background(), "key-1"); err != nil {
		t.Fatalf("Key(key-1): %v", err)
	}
	ids = []string{"key-1", "rotated"}
	if _, err := keys.Key(context.Background(), "rotated"); err == nil {
		t.Error("Key(rotated) a moment after a fetch: nil error, want refused until five minutes pass")
	}
	clock = now.Add(6 * time.Minute)
	if _, err := keys.Key(context.Background(), "rotated"); err != nil {
		t.Errorf("Key(rotated) after five minutes: %v", err)
	}
	if got := fetches.Load(); got != 2 {
		t.Errorf("key set fetched %d times, want 2", got)
	}
}

func TestKeyIsRefused(t *testing.T) {
	with := func(edit func(map[string]string)) map[string]any {
		k := published("key-1")
		edit(k)
		return map[string]any{"keys": []map[string]string{k}}
	}
	tests := []struct {
		name, kid string
		status    int
		body      any
	}{
		{"the set not found", "key-1", http.StatusNotFound, with(func(map[string]string) {})},
		{"a set that is not JSON", "key-1", http.StatusOK, "not a key set"},
		{"no keys", "key-1", http.StatusOK, map[string]any{"keys": []any{}}},
		{"a key that is not RSA", "key-1", http.StatusOK, with(func(k map[string]string) { k["kty"] = "EC" })},
		{"a key with no id, for a token with none", "", http.StatusOK, with(func(k map[string]string) { k["kid"] = "" })},
		{"a key with no exponent", "key-1", http.StatusOK, with(func(k map[string]string) { k["e"] = "" })},
		{"only another key", "key-1", http.StatusOK, map[string]any{"keys": []map[string]string{published("key-2")}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := server(t, func() (int, any) { return tt.status, tt.body })
			if got, err := jwks.New(s.URL, s.Client(), func() time.Time { return now }).Key(context.Background(), tt.kid); err == nil {
				t.Errorf("Key(%q) = %v, want an error", tt.kid, got)
			}
		})
	}
}

func TestAFailedFetchIsNotRetriedAtOnce(t *testing.T) {
	status := http.StatusServiceUnavailable
	s, fetches := server(t, func() (int, any) {
		return status, map[string]any{"keys": []map[string]string{published("key-1")}}
	})
	keys := jwks.New(s.URL, s.Client(), func() time.Time { return now })
	if _, err := keys.Key(context.Background(), "key-1"); err == nil {
		t.Fatal("Key(key-1) while the set is unavailable: nil error, want an error")
	}
	status = http.StatusOK
	if _, err := keys.Key(context.Background(), "key-1"); err == nil {
		t.Error("Key(key-1) a moment after a failed fetch: nil error, want refused until five minutes pass")
	}
	if got := fetches.Load(); got != 1 {
		t.Errorf("key set fetched %d times, want 1", got)
	}
}

func TestASetWithNothingUsableKeepsTheKeysAlreadyFetched(t *testing.T) {
	body := map[string]any{"keys": []map[string]string{published("key-1")}}
	s, _ := server(t, func() (int, any) { return http.StatusOK, body })
	clock := now
	keys := jwks.New(s.URL, s.Client(), func() time.Time { return clock })
	if _, err := keys.Key(context.Background(), "key-1"); err != nil {
		t.Fatalf("Key(key-1): %v", err)
	}
	body = map[string]any{"keys": []any{}}
	clock = now.Add(6 * time.Minute)
	if _, err := keys.Key(context.Background(), "key-2"); err == nil {
		t.Fatal("Key(key-2) from a set with no keys: nil error, want an error")
	}
	if _, err := keys.Key(context.Background(), "key-1"); err != nil {
		t.Errorf("Key(key-1) after an empty set: %v, want the key fetched before", err)
	}
}
