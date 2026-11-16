// Asking YouTube's hub to push a channel's videos to our callback, signed with our hub secret.
package movies

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MinHubSecret is the shortest hub secret taken, in bytes.
const MinHubSecret = 32

// HubPath is the api's callback the hub checks requests at and pushes to.
const HubPath = "/v1/movies/hub"

// HubClient asks a WebSub hub for subscriptions to channels' feeds, for callback, signed with secret.
type HubClient struct {
	client   *http.Client
	hub      string
	callback string
	secret   []byte
}

// NewHubClient asks hub (Hub, or a test server) to push to callback through client (a 15-second default when
// nil), signed with secret.
func NewHubClient(client *http.Client, hub, callback string, secret []byte) *HubClient {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &HubClient{client: client, hub: hub, callback: callback, secret: secret}
}

// String names the type without its secret.
func (h *HubClient) String() string { return "HubClient{secret: redacted}" }

// GoString names the type without its secret, for %#v.
func (h *HubClient) GoString() string { return h.String() }

// Subscribe asks the hub to push channelID's new and changed videos; the hub then checks the request at the
// callback. A refusal is an error naming the hub's status.
func (h *HubClient) Subscribe(ctx context.Context, channelID string) error {
	if !channelIDPattern.MatchString(channelID) {
		return errors.New("invalid channel id")
	}
	form := url.Values{
		"hub.mode":     {"subscribe"},
		"hub.topic":    {Topic(channelID)},
		"hub.callback": {h.callback},
		"hub.verify":   {"async"},
		"hub.secret":   {string(h.secret)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.hub, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("asking the hub to subscribe: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("the hub answered HTTP %d to a subscribe", resp.StatusCode)
	}
	return nil
}
