package main

import (
	"maps"
	"strings"
	"testing"

	"ttacommunity/backend/internal/httpapi"
)

// imagesEnv is a complete config for the assets bucket, with invented values.
var imagesEnv = map[string]string{
	"R2_ACCOUNT_ID":        "0123456789abcdef0123456789abcdef",
	"R2_ACCESS_KEY_ID":     "invented-key",
	"R2_SECRET_ACCESS_KEY": "invented-secret",
	"ASSETS_BUCKET":        "ttac-assets-test",
	"ASSETS_PUBLIC_URL":    "https://assets.example.test",
}

func TestImagesAreOffWithNoneOfTheirConfig(t *testing.T) {
	got, err := openImages(func(string) string { return "" })
	if err != nil || len(got) != 0 {
		t.Errorf("openImages with nothing set = %d buckets, %v; want none and no error", len(got), err)
	}
}

func TestImagesSetUpTheAssetsBucket(t *testing.T) {
	got, err := openImages(func(k string) string { return imagesEnv[k] })
	if err != nil {
		t.Fatalf("openImages with a full config: %v", err)
	}
	if _, ok := got[httpapi.Assets]; !ok || len(got) != 1 {
		t.Errorf("openImages = %d buckets, assets set up %v; want the assets bucket alone", len(got), ok)
	}
}

func TestImagesRefuseHalfTheirConfig(t *testing.T) {
	tests := []struct {
		name, key, value, want string
	}{
		{"no account", "R2_ACCOUNT_ID", "", "R2_ACCOUNT_ID"},
		{"an account that is not one", "R2_ACCOUNT_ID", "evil.example/x", "R2_ACCOUNT_ID"},
		{"no key id", "R2_ACCESS_KEY_ID", "", "R2_ACCESS_KEY_ID"},
		{"no secret", "R2_SECRET_ACCESS_KEY", "", "R2_SECRET_ACCESS_KEY"},
		{"no bucket", "ASSETS_BUCKET", "", "ASSETS_BUCKET"},
		{"no public url", "ASSETS_PUBLIC_URL", "", "ASSETS_PUBLIC_URL"},
		{"a plain-http public url", "ASSETS_PUBLIC_URL", "http://assets.example.test", "ASSETS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := maps.Clone(imagesEnv)
			env[tt.key] = tt.value
			_, err := openImages(func(k string) string { return env[k] })
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("openImages with %s = %v, want a refusal naming %s", tt.name, err, tt.want)
			}
		})
	}
}

func TestImagesRefuseR2WithoutABucket(t *testing.T) {
	env := maps.Clone(imagesEnv)
	delete(env, "ASSETS_BUCKET")
	delete(env, "ASSETS_PUBLIC_URL")
	if _, err := openImages(func(k string) string { return env[k] }); err == nil || !strings.Contains(err.Error(), "ASSETS_BUCKET") {
		t.Errorf("openImages with R2 and no bucket = %v, want a refusal naming ASSETS_BUCKET", err)
	}
}
