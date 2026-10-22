// Package jwks keeps a provider's RSA signing keys, read from its published key set (JWKS): fetched when a token
// names a key not seen yet, at most once every few minutes.
package jwks

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// refetchAfter limits how often an unknown key id makes the keys be fetched again.
const refetchAfter = 5 * time.Minute

// Keys is one provider's signing keys, fetched from its key set when needed.
type Keys struct {
	url  string
	http *http.Client
	now  func() time.Time

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

// New returns the keys published at url, fetched through hc, with now timing the fetches.
func New(url string, hc *http.Client, now func() time.Time) *Keys {
	return &Keys{url: url, http: hc, now: now, keys: map[string]*rsa.PublicKey{}}
}

// Key returns the signing key with this id, fetching the keys when it is unknown and they were not fetched lately.
func (k *Keys) Key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if key, ok := k.keys[kid]; ok {
		return key, nil
	}
	if !k.fetched.IsZero() && k.now().Sub(k.fetched) < refetchAfter {
		return nil, errors.New("unknown signing key")
	}
	keys, err := k.fetch(ctx)
	k.fetched = k.now()
	if err != nil {
		return nil, err
	}
	k.keys = keys
	if key, ok := keys[kid]; ok {
		return key, nil
	}
	return nil, errors.New("unknown signing key")
}

// fetch reads the RSA keys in the key set.
func (k *Keys) fetch(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.url, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching signing keys: %w", err)
	}
	resp, err := k.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching signing keys: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching signing keys: status %d", resp.StatusCode)
	}
	var body struct {
		Keys []struct {
			Kid, Kty, N, E string
		} `json:"keys"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("reading signing keys: %w", err)
	}
	keys := map[string]*rsa.PublicKey{}
	for _, key := range body.Keys {
		if key.Kty != "RSA" || key.Kid == "" {
			continue
		}
		n, errN := base64.RawURLEncoding.DecodeString(key.N)
		e, errE := base64.RawURLEncoding.DecodeString(key.E)
		if errN != nil || errE != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		keys[key.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	if len(keys) == 0 {
		return nil, errors.New("fetching signing keys: none usable")
	}
	return keys, nil
}
