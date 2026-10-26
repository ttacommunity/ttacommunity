package discordbot_test

import (
	"crypto/ed25519"
	"encoding/hex"
	"strings"
	"testing"

	"ttacommunity/backend/internal/discordbot"
)

// testKey is an invented application key, made from a fixed seed so every run signs alike.
func testKey(seed byte) (ed25519.PublicKey, ed25519.PrivateKey) {
	priv := ed25519.NewKeyFromSeed([]byte(strings.Repeat(string(rune(seed)), ed25519.SeedSize)))
	return priv.Public().(ed25519.PublicKey), priv
}

func TestVerify(t *testing.T) {
	pub, priv := testKey('a')
	other, _ := testKey('b')
	const ts, body = "1760000000", `{"type":1}`
	sig := hex.EncodeToString(ed25519.Sign(priv, []byte(ts+body)))
	tests := []struct {
		name          string
		key           ed25519.PublicKey
		sig, ts, body string
		want          bool
	}{
		{"Discord's signature", pub, sig, ts, body, true},
		{"no signature", pub, "", ts, body, false},
		{"no timestamp", pub, sig, "", body, false},
		{"a signature that is not hex", pub, "zz" + sig[2:], ts, body, false},
		{"a signature cut short", pub, sig[:64], ts, body, false},
		{"another key", other, sig, ts, body, false},
		{"an altered body", pub, sig, ts, `{"type":2}`, false},
		{"another timestamp", pub, sig, "1760000001", body, false},
		{"no key", nil, sig, ts, body, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := discordbot.Verify(tt.key, tt.sig, tt.ts, []byte(tt.body)); got != tt.want {
				t.Errorf("Verify = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParsePublicKey(t *testing.T) {
	pub, _ := testKey('a')
	tests := []struct {
		name, hex string
		wantErr   bool
	}{
		{"the hex Discord shows", hex.EncodeToString(pub), false},
		{"upper case", strings.ToUpper(hex.EncodeToString(pub)), false},
		{"too short", hex.EncodeToString(pub[:31]), true},
		{"not hex", strings.Repeat("zz", 32), true},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := discordbot.ParsePublicKey(tt.hex)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParsePublicKey(%q) error = %v, want an error: %v", tt.hex, err, tt.wantErr)
			}
			if !tt.wantErr && !key.Equal(pub) {
				t.Errorf("ParsePublicKey(%q) = %x, want %x", tt.hex, key, pub)
			}
		})
	}
}
