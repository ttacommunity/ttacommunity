package imagestore_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"ttacommunity/backend/internal/imagestore"
)

// Enough of each format's header for it to be recognised by content.
var (
	png  = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	jpeg = []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00")
	gif  = []byte("GIF89a\x01\x00\x01\x00")
	webp = []byte("RIFF\x24\x00\x00\x00WEBPVP8 ")
)

func hash12(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:12]
}

func TestCheckNamesAnImageByItsNameAndContent(t *testing.T) {
	tests := []struct {
		name, file string
		body       []byte
		wantKey    string
		wantType   string
	}{
		{"png", "blitz-bolts.png", png, "blitz-bolts-" + hash12(png) + ".png", "image/png"},
		{"jpeg", "photo.jpeg", jpeg, "photo-" + hash12(jpeg) + ".jpg", "image/jpeg"},
		{"gif", "spinner.gif", gif, "spinner-" + hash12(gif) + ".gif", "image/gif"},
		{"webp", "banner.webp", webp, "banner-" + hash12(webp) + ".webp", "image/webp"},
		{"the extension comes from the content", "really-a-jpeg.png", jpeg, "really-a-jpeg-" + hash12(jpeg) + ".jpg", "image/jpeg"},
		{"the name is cleaned", "  Blitz Bolts (v2)!.PNG", png, "blitz-bolts-v2-" + hash12(png) + ".png", "image/png"},
		{"a path is dropped", `C:\pics\..\Logo.png`, png, "logo-" + hash12(png) + ".png", "image/png"},
		{"no usable name", "ÄÖÜ.png", png, "image-" + hash12(png) + ".png", "image/png"},
		{"a long name is cut", strings.Repeat("a", 100) + ".png", png, strings.Repeat("a", 60) + "-" + hash12(png) + ".png", "image/png"},
		{"exactly the limit", "big.png", padded(png, imagestore.MaxSize), "big-" + hash12(padded(png, imagestore.MaxSize)) + ".png", "image/png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, err := imagestore.Check(tt.file, tt.body)
			if err != nil {
				t.Fatalf("Check(%q) error = %v", tt.file, err)
			}
			if img.Key != tt.wantKey || img.ContentType != tt.wantType {
				t.Errorf("Check(%q) = %q, %q, want %q, %q", tt.file, img.Key, img.ContentType, tt.wantKey, tt.wantType)
			}
			if !imagestore.ValidKey(img.Key) {
				t.Errorf("ValidKey(%q) = false for a key Check made", img.Key)
			}
		})
	}
}

func TestCheckRefusesWhatIsNotASmallEnoughImage(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want error
	}{
		{"empty", nil, imagestore.ErrNotImage},
		{"text", []byte("hello, world"), imagestore.ErrNotImage},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), imagestore.ErrNotImage},
		{"html", []byte("<!DOCTYPE html><html></html>"), imagestore.ErrNotImage},
		{"bmp", []byte("BM\x00\x00\x00\x00\x00\x00\x00\x00"), imagestore.ErrNotImage},
		{"one byte over the limit", padded(png, imagestore.MaxSize+1), imagestore.ErrTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := imagestore.Check("file.png", tt.body); !errors.Is(err, tt.want) {
				t.Errorf("Check(%s) error = %v, want %v", tt.name, err, tt.want)
			}
		})
	}
}

func TestValidKeyAcceptsOnlyKeysCheckWouldMake(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"blitz-bolts-0123456789ab.png", true},
		{"a-0123456789ab.webp", true},
		{"blitz-bolts.png", false},
		{"blitz-bolts-0123456789AB.png", false},
		{"../secret-0123456789ab.png", false},
		{"dir/x-0123456789ab.png", false},
		{"x-0123456789ab.svg", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := imagestore.ValidKey(tt.key); got != tt.want {
			t.Errorf("ValidKey(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func padded(head []byte, size int) []byte {
	return append(bytes.Clone(head), make([]byte, size-len(head))...)
}
