// Package imagestore keeps images in S3-compatible buckets (Cloudflare R2): every image checked the same way, and
// named by its content so that an address is never reused for another picture.
package imagestore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"strings"
)

// MaxSize is the largest image accepted, in bytes.
const MaxSize = 5 << 20

var (
	// ErrNotImage means the content is not a PNG, JPEG, GIF or WebP.
	ErrNotImage = errors.New("imagestore: not a PNG, JPEG, GIF or WebP image")
	// ErrTooLarge means the image is over MaxSize.
	ErrTooLarge = errors.New("imagestore: the image is over 5 MB")
)

// Image is a checked image and the key it is stored under.
type Image struct {
	Key, ContentType string
	Body             []byte
}

// extensions are the formats accepted, by the type net/http sniffs from the content.
var extensions = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp"}

// maxName is the longest name kept before the hash.
const maxName = 60

var (
	unsafeRun = regexp.MustCompile(`[^a-z0-9]+`)
	keyShape  = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*-[0-9a-f]{12}\.(png|jpg|gif|webp)$`)
)

// Check returns body as an Image keyed by file's cleaned name, 12 hex characters of its SHA-256 and the extension
// its content shows, or ErrNotImage or ErrTooLarge.
func Check(file string, body []byte) (Image, error) {
	if len(body) > MaxSize {
		return Image{}, ErrTooLarge
	}
	contentType := http.DetectContentType(body)
	ext, ok := extensions[contentType]
	if len(body) == 0 || !ok {
		return Image{}, ErrNotImage
	}
	sum := sha256.Sum256(body)
	return Image{Key: cleanName(file) + "-" + hex.EncodeToString(sum[:])[:12] + "." + ext, ContentType: contentType, Body: body}, nil
}

// cleanName returns file's base name without its extension, as lower-case letters, digits and single hyphens.
func cleanName(file string) string {
	name := file[strings.LastIndexAny(file, `/\`)+1:]
	if dot := strings.LastIndex(name, "."); dot > 0 {
		name = name[:dot]
	}
	name = strings.Trim(unsafeRun.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(name) > maxName {
		name = strings.TrimRight(name[:maxName], "-")
	}
	if name == "" {
		return "image"
	}
	return name
}

// ValidKey reports whether key has the shape Check gives keys.
func ValidKey(key string) bool { return keyShape.MatchString(key) }
