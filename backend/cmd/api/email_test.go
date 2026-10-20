package main

import (
	"maps"
	"strings"
	"testing"
)

// emailEnv is a complete production config for email sign-in, with invented values.
var emailEnv = map[string]string{
	"API_ADDR":         ":8080",
	"RESEND_API_KEY":   "re_invented",
	"EMAIL_FROM":       "TTA Community <noreply@example.test>",
	"EMAIL_SUPPORT":    "support@example.test",
	"TURNSTILE_SECRET": "0x4AAAAAAinvented",
}

func TestEmailSignInIsOffWithNoneOfItsConfig(t *testing.T) {
	got, err := openEmail(func(string) string { return "" }, "https://site.example.test")
	if err != nil || got.ready() {
		t.Errorf("openEmail with nothing set = ready %v, %v; want off and no error", got.ready(), err)
	}
}

func TestEmailSignInStartsWithAFullConfigCheckingTheSitesHostname(t *testing.T) {
	got, err := openEmail(func(k string) string { return emailEnv[k] }, "https://site.example.test")
	if err != nil || !got.ready() {
		t.Fatalf("openEmail with a full config = ready %v, %v; want ready", got.ready(), err)
	}
	if got.hostname != "site.example.test" {
		t.Errorf("Turnstile hostname = %q, want the site's", got.hostname)
	}
}

func TestEmailSignInRefusesHalfItsConfig(t *testing.T) {
	tests := []struct {
		name, key, value, want string
	}{
		{"no Resend key", "RESEND_API_KEY", "", "RESEND_API_KEY"},
		{"no sender", "EMAIL_FROM", "", "EMAIL_FROM"},
		{"no support address", "EMAIL_SUPPORT", "", "EMAIL_SUPPORT"},
		{"a support address that is not one", "EMAIL_SUPPORT", "support", "EMAIL_SUPPORT"},
		{"no Turnstile secret", "TURNSTILE_SECRET", "", "TURNSTILE_SECRET"},
		{"Turnstile's always-pass test secret", "TURNSTILE_SECRET", "1x0000000000000000000000000000000AA", "TURNSTILE_SECRET"},
		{"Turnstile's other test secrets", "TURNSTILE_SECRET", "3x0000000000000000000000000000000AA", "TURNSTILE_SECRET"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := maps.Clone(emailEnv)
			env[tt.key] = tt.value
			_, err := openEmail(func(k string) string { return env[k] }, "https://site.example.test")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("openEmail with %s=%q = %v, want a refusal naming %s", tt.key, tt.value, err, tt.want)
			}
		})
	}
}

func TestTheDevMailerIsOnlyForThisMachine(t *testing.T) {
	local := map[string]string{
		"API_ADDR":         "127.0.0.1:8080",
		"EMAIL_DEV_MAIL":   "1",
		"EMAIL_SUPPORT":    "support@example.test",
		"TURNSTILE_SECRET": "1x0000000000000000000000000000000AA",
	}
	if got, err := openEmail(func(k string) string { return local[k] }, "http://127.0.0.1:4321"); err != nil || !got.ready() || got.hostname != "" {
		t.Errorf("openEmail with the dev mailer here = ready %v, hostname %q, %v; want ready for any hostname", got.ready(), got.hostname, err)
	}
	tests := []struct {
		name, key, value, site string
	}{
		{"listening beyond this machine", "API_ADDR", ":8080", "http://127.0.0.1:4321"},
		{"in Kubernetes", "KUBERNETES_SERVICE_HOST", "10.0.0.1", "http://127.0.0.1:4321"},
		{"a site that is not on this machine", "API_ADDR", "127.0.0.1:8080", "https://site.example.test"},
		{"a Resend key as well", "RESEND_API_KEY", "re_invented", "http://127.0.0.1:4321"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := maps.Clone(local)
			env[tt.key] = tt.value
			_, err := openEmail(func(k string) string { return env[k] }, tt.site)
			if err == nil || !strings.Contains(err.Error(), "EMAIL_DEV_MAIL") {
				t.Errorf("openEmail with %s=%q = %v, want a refusal naming EMAIL_DEV_MAIL", tt.key, tt.value, err)
			}
		})
	}
}

func TestTheSignInEmailGivesTheLinkItsHourAndWhereToWrite(t *testing.T) {
	m := signInMail("player@example.test", "https://site.example.test/account/email/#TOKEN", "support@example.test")
	if m.To != "player@example.test" || m.Subject == "" || m.HTML != "" {
		t.Errorf("mail = to %q, subject %q, HTML %q; want the player, a subject and plain text only", m.To, m.Subject, m.HTML)
	}
	for _, want := range []string{"https://site.example.test/account/email/#TOKEN", "hour", "support@example.test"} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("mail text lacks %q:\n%s", want, m.Text)
		}
	}
}
