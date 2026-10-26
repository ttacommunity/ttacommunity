package webanalytics_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/webanalytics"
)

// asked is what one request to the stub carried.
type asked struct {
	auth      string
	query     string
	variables map[string]string
}

// stub is Cloudflare's GraphQL API answering every request with status and reply, and keeping what it was asked.
func stub(t *testing.T, status int, reply string) (*httptest.Server, *[]asked) {
	t.Helper()
	var got []asked
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query     string            `json:"query"`
			Variables map[string]string `json:"variables"`
		}
		if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&body) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		got = append(got, asked{auth: r.Header.Get("Authorization"), query: body.Query, variables: body.Variables})
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(s.Close)
	return s, &got
}

func newClient(t *testing.T, s *httptest.Server) *webanalytics.Client {
	t.Helper()
	c, err := webanalytics.New(webanalytics.Config{Token: "test-token", Account: "test-account", Host: "site.example.test", URL: s.URL}, s.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

var (
	from = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	to   = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
)

func TestVisitsAsksForPeoplesVisitsToTheSiteInTheWindow(t *testing.T) {
	s, got := stub(t, http.StatusOK, `{"data":{"viewer":{"accounts":[{"rumPageloadEventsAdaptiveGroups":[{"sum":{"visits":190}}]}]}},"errors":null}`)
	if _, err := newClient(t, s).Visits(context.Background(), from, to.In(time.FixedZone("AEDT", 11*3600))); err != nil {
		t.Fatalf("Visits: %v", err)
	}
	if len(*got) != 1 {
		t.Fatalf("asked %d times, want once", len(*got))
	}
	a := (*got)[0]
	if a.auth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want the token", a.auth)
	}
	want := map[string]string{"account": "test-account", "host": "site.example.test", "from": "2026-09-29T12:00:00Z", "to": "2026-10-27T12:00:00Z"}
	for k, v := range want {
		if a.variables[k] != v {
			t.Errorf("variable %s = %q, want %q", k, a.variables[k], v)
		}
	}
	if !strings.Contains(a.query, "bot: 0") {
		t.Errorf("query = %q, want bots left out", a.query)
	}
}

func TestVisitsReadsCloudflaresAnswer(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		reply   string
		want    int
		wantErr bool
	}{
		{"visits counted", http.StatusOK, `{"data":{"viewer":{"accounts":[{"rumPageloadEventsAdaptiveGroups":[{"sum":{"visits":190}}]}]}},"errors":null}`, 190, false},
		{"no visits", http.StatusOK, `{"data":{"viewer":{"accounts":[{"rumPageloadEventsAdaptiveGroups":[]}]}},"errors":null}`, 0, false},
		{"an error in the answer", http.StatusOK, `{"data":null,"errors":[{"message":"not authorized for that account"}]}`, 0, true},
		{"an account the token cannot see", http.StatusOK, `{"data":{"viewer":{"accounts":[]}},"errors":null}`, 0, true},
		{"refused", http.StatusUnauthorized, `{"errors":[{"message":"bad token"}]}`, 0, true},
		{"down", http.StatusBadGateway, `bad gateway`, 0, true},
		{"not JSON", http.StatusOK, `<html>`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := stub(t, tt.status, tt.reply)
			got, err := newClient(t, s).Visits(context.Background(), from, to)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("Visits = %d, %v; want %d, an error: %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestNewRefusesAConfigThatCannotBeUsed(t *testing.T) {
	full := webanalytics.Config{Token: "t", Account: "a", Host: "site.example.test"}
	tests := []struct {
		name string
		edit func(*webanalytics.Config)
	}{
		{"no token", func(c *webanalytics.Config) { c.Token = "" }},
		{"no account", func(c *webanalytics.Config) { c.Account = "" }},
		{"no host", func(c *webanalytics.Config) { c.Host = "" }},
		{"an API somewhere else", func(c *webanalytics.Config) { c.URL = "https://example.test/graphql" }},
		{"Cloudflare's API over plain http", func(c *webanalytics.Config) { c.URL = "http://api.cloudflare.com/client/v4/graphql" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := full
			tt.edit(&cfg)
			if _, err := webanalytics.New(cfg, http.DefaultClient); err == nil {
				t.Errorf("New(%s) = nil error, want a refusal", tt.name)
			}
		})
	}
}

func TestConfigKeepsTheTokenOutOfItsString(t *testing.T) {
	if s := (webanalytics.Config{Token: "the-token", Account: "a", Host: "site.example.test"}).String(); strings.Contains(s, "the-token") {
		t.Errorf("Config.String() = %q, want the token left out", s)
	}
}

// source is a stand-in for Cloudflare: it answers visits or err, and keeps the window it was last asked for.
type source struct {
	visits   int
	err      error
	from, to time.Time
}

func (s *source) Visits(_ context.Context, from, to time.Time) (int, error) {
	s.from, s.to = from, to
	return s.visits, s.err
}

func TestACounterHasNothingToShowBeforeItCounts(t *testing.T) {
	c := webanalytics.NewCounter(&source{visits: 190}, func() time.Time { return to })
	if got, ok := c.Latest(); ok {
		t.Errorf("Latest before Refresh = %+v, want nothing", got)
	}
}

func TestACounterCountsTheLast28DaysUpToNow(t *testing.T) {
	src := &source{visits: 190}
	c := webanalytics.NewCounter(src, func() time.Time { return to })
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !src.from.Equal(to.AddDate(0, 0, -28)) || !src.to.Equal(to) {
		t.Errorf("asked for %v to %v, want %v to %v", src.from, src.to, to.AddDate(0, 0, -28), to)
	}
	got, ok := c.Latest()
	want := webanalytics.Count{Visits: 190, Days: 28, AsOf: to}
	if !ok || got != want {
		t.Errorf("Latest = %+v, %v; want %+v", got, ok, want)
	}
}

func TestACounterKeepsItsLastCountWhenARefreshFails(t *testing.T) {
	now := to
	src := &source{visits: 190}
	c := webanalytics.NewCounter(src, func() time.Time { return now })
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	now, src.visits, src.err = to.Add(time.Hour), 0, errors.New("Cloudflare is down")
	if err := c.Refresh(context.Background()); err == nil {
		t.Errorf("Refresh with Cloudflare down = nil error, want its error")
	}
	if got, ok := c.Latest(); !ok || got.Visits != 190 || !got.AsOf.Equal(to) {
		t.Errorf("Latest = %+v, %v; want the count from %v", got, ok, to)
	}
}

func TestACounterShowsNoCountOlderThanADay(t *testing.T) {
	tests := []struct {
		name   string
		age    time.Duration
		wantOK bool
	}{
		{"just counted", 0, true},
		{"a day old", 24 * time.Hour, true},
		{"over a day old", 24*time.Hour + time.Second, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := to
			c := webanalytics.NewCounter(&source{visits: 190}, func() time.Time { return now })
			if err := c.Refresh(context.Background()); err != nil {
				t.Fatalf("Refresh: %v", err)
			}
			now = to.Add(tt.age)
			if _, ok := c.Latest(); ok != tt.wantOK {
				t.Errorf("Latest %v after counting: shown = %v, want %v", tt.age, ok, tt.wantOK)
			}
		})
	}
}
