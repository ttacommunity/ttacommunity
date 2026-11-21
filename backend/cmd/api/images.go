// The image buckets, in Cloudflare R2: one R2 token for all, and each bucket's name and public address.
package main

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"ttacommunity/backend/internal/httpapi"
	"ttacommunity/backend/internal/imagestore"
	"ttacommunity/backend/internal/sigv4"
)

// imageBuckets is every bucket the API knows, each read from <NAME>_BUCKET and <NAME>_PUBLIC_URL.
var imageBuckets = []httpapi.ImageBucket{httpapi.Assets}

// r2Account is the shape of a Cloudflare account id, which names R2's endpoint host.
var r2Account = regexp.MustCompile(`^[0-9a-f]{32}$`)

// openImages returns the image buckets getenv sets up: R2_ACCOUNT_ID, R2_ACCESS_KEY_ID and R2_SECRET_ACCESS_KEY,
// and each bucket's <NAME>_BUCKET and <NAME>_PUBLIC_URL. With none of them set images are off; any part missing, or
// R2 with no bucket, is an error.
func openImages(getenv func(string) string) (map[httpapi.ImageBucket]httpapi.ImageStore, error) {
	shared := []string{"R2_ACCOUNT_ID", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY"}
	var perBucket []string
	for _, b := range imageBuckets {
		prefix := strings.ToUpper(string(b))
		perBucket = append(perBucket, prefix+"_BUCKET", prefix+"_PUBLIC_URL")
	}
	set := false
	for _, k := range append(shared, perBucket...) {
		set = set || getenv(k) != ""
	}
	if !set {
		return nil, nil
	}
	for _, k := range shared {
		if getenv(k) == "" {
			return nil, fmt.Errorf("%s is not set, and other image settings are", k)
		}
	}
	account := getenv("R2_ACCOUNT_ID")
	if !r2Account.MatchString(account) {
		return nil, errors.New("R2_ACCOUNT_ID is not a Cloudflare account id")
	}
	creds := sigv4.Credentials{AccessKeyID: getenv("R2_ACCESS_KEY_ID"), SecretAccessKey: getenv("R2_SECRET_ACCESS_KEY")}
	hc := &http.Client{Timeout: 30 * time.Second}
	buckets := map[httpapi.ImageBucket]httpapi.ImageStore{}
	for _, b := range imageBuckets {
		prefix := strings.ToUpper(string(b))
		name, public := getenv(prefix+"_BUCKET"), getenv(prefix+"_PUBLIC_URL")
		if name == "" && public == "" {
			continue
		}
		if name == "" || public == "" {
			return nil, fmt.Errorf("%s_BUCKET and %s_PUBLIC_URL are set together, or neither", prefix, prefix)
		}
		bucket, err := imagestore.New(imagestore.Config{
			Endpoint:    "https://" + account + ".r2.cloudflarestorage.com",
			Bucket:      name,
			PublicURL:   public,
			Credentials: creds,
		}, hc, time.Now)
		if err != nil {
			return nil, fmt.Errorf("%s_BUCKET and %s_PUBLIC_URL: %w", prefix, prefix, err)
		}
		buckets[b] = bucket
	}
	if len(buckets) == 0 {
		return nil, fmt.Errorf("R2 is set up but no bucket is: set %s", strings.Join(perBucket, " and "))
	}
	return buckets, nil
}
