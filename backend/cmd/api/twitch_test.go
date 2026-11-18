package main

import (
	"strings"
	"testing"
)

func TestTwitchIsSetUpWholeOrNotAtAll(t *testing.T) {
	full := map[string]string{
		"TWITCH_CLIENT_ID":       "invented-client-id",
		"TWITCH_CLIENT_SECRET":   "invented-client-secret",
		"TWITCH_EVENTSUB_SECRET": "an-invented-eventsub-secret",
		"API_ORIGIN":             "https://api.example",
	}
	tests := []struct {
		name    string
		change  map[string]string
		wantOn  bool
		wantErr string
	}{
		{"all set", nil, true, ""},
		{"none set", map[string]string{"TWITCH_CLIENT_ID": "", "TWITCH_CLIENT_SECRET": "", "TWITCH_EVENTSUB_SECRET": ""}, false, ""},
		{"no client secret", map[string]string{"TWITCH_CLIENT_SECRET": ""}, false, "TWITCH_CLIENT_SECRET"},
		{"no EventSub secret", map[string]string{"TWITCH_EVENTSUB_SECRET": ""}, false, "TWITCH_EVENTSUB_SECRET"},
		{"an EventSub secret too short", map[string]string{"TWITCH_EVENTSUB_SECRET": "short"}, false, "TWITCH_EVENTSUB_SECRET"},
		{"an EventSub secret too long", map[string]string{"TWITCH_EVENTSUB_SECRET": strings.Repeat("x", 101)}, false, "TWITCH_EVENTSUB_SECRET"},
		{"a callback Twitch cannot reach", map[string]string{"API_ORIGIN": "http://127.0.0.1:8080"}, false, "API_ORIGIN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{}
			for k, v := range full {
				env[k] = v
			}
			for k, v := range tt.change {
				env[k] = v
			}
			tw, err := openTwitch(func(k string) string { return env[k] }, nil)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("openTwitch: %v, want an error naming %s", err, tt.wantErr)
				}
				return
			}
			if err != nil || (tw.client != nil) != tt.wantOn || (tw.hub != nil) != tt.wantOn {
				t.Errorf("openTwitch = %+v, %v; want on %t", tw, err, tt.wantOn)
			}
			if tt.wantOn && tw.callback != "https://api.example/v1/twitch/eventsub" {
				t.Errorf("callback = %q, want the api's EventSub path", tw.callback)
			}
		})
	}
}
