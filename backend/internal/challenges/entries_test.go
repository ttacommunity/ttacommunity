package challenges_test

import (
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/challenges"
)

func TestReadEntriesTakesTheOldSitesAdminList(t *testing.T) {
	file := `[
	  {"id": 11, "c": "1ai", "p": " Ann  Example ", "s": 120, "u": "https://discord.com/channels/1/2/3", "t": "2025-03-01T10:00:00.000Z",
	   "b": 0, "hidden": true, "hiddenReason": "a duplicate", "source": "web", "ipHash": "ab12", "checked": true},
	  {"id": 12, "c": "epic1_exp", "p": "Bob", "s": -4, "u": null, "t": "2024-07-23T16:12:16.000Z", "b": 1, "hidden": false,
	   "hiddenReason": null, "source": "sheet", "ipHash": null, "checked": false}
	]`
	got, err := challenges.ReadEntries(strings.NewReader(file))
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if got.Rows != 2 || len(got.Entries) != 2 || len(got.Refused) != 0 {
		t.Fatalf("ReadEntries = %d rows, %d entries, refused %+v; want 2, 2, none", got.Rows, len(got.Entries), got.Refused)
	}
	want := challenges.ImportedEntry{OldID: 11, Challenge: "1ai", Name: "Ann Example", Domination: 120,
		ProofURL: "https://discord.com/channels/1/2/3", At: time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC),
		Source: "web", Hidden: true, HiddenReason: "a duplicate", Checked: true}
	if got.Entries[0] != want {
		t.Errorf("first entry = %+v, want %+v", got.Entries[0], want)
	}
	if e := got.Entries[1]; !e.Bugged || e.ProofURL != "" || e.Domination != -4 || e.Source != "sheet" || e.HiddenReason != "" {
		t.Errorf("second entry = %+v, want bugged, unproven, -4, from the sheet", e)
	}
}

func TestReadEntriesTakesThePublicListAsVisibleEntriesFromTheWeb(t *testing.T) {
	got, err := challenges.ReadEntries(strings.NewReader(`[{"id": 3, "c": "1ai", "p": "Ann", "s": 50, "u": null, "t": "2025-01-01T00:00:00Z", "b": 0}]`))
	if err != nil || len(got.Entries) != 1 {
		t.Fatalf("ReadEntries = %+v, %v; want one entry", got, err)
	}
	if e := got.Entries[0]; e.Source != "web" || e.Hidden || e.Checked {
		t.Errorf("entry = %+v, want a visible, unchecked entry from the web", e)
	}
}

func TestReadEntriesRefusesAnEntryItCannotStore(t *testing.T) {
	valid := `{"id": 1, "c": "1ai", "p": "Ann", "s": 50, "u": null, "t": "2025-01-01T00:00:00Z", "b": 0, "source": "web"}`
	tests := []struct{ name, entry, want string }{
		{"no challenge", strings.Replace(valid, `"c": "1ai"`, `"c": ""`, 1), "challenge"},
		{"no name", strings.Replace(valid, `"p": "Ann"`, `"p": "  "`, 1), "name"},
		{"a name over 100 characters", strings.Replace(valid, `"p": "Ann"`, `"p": "`+strings.Repeat("n", 101)+`"`, 1), "name"},
		{"no domination", strings.Replace(valid, `"s": 50, `, ``, 1), "domination"},
		{"a domination out of range", strings.Replace(valid, `"s": 50`, `"s": 10000`, 1), "domination"},
		{"a proof that is not https", strings.Replace(valid, `"u": null`, `"u": "http://discord.com/channels/1/2/3"`, 1), "proof"},
		{"a time that is not a time", strings.Replace(valid, `2025-01-01T00:00:00Z`, `yesterday`, 1), "time"},
		{"an unknown source", strings.Replace(valid, `"source": "web"`, `"source": "mail"`, 1), "source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := challenges.ReadEntries(strings.NewReader("[" + tt.entry + "]"))
			if err != nil {
				t.Fatalf("ReadEntries: %v", err)
			}
			if len(got.Entries) != 0 || len(got.Refused) != 1 || got.Refused[0].OldID != 1 || !strings.Contains(got.Refused[0].Reason, tt.want) {
				t.Errorf("ReadEntries(%s) = entries %+v, refused %+v; want entry 1 refused for its %s", tt.name, got.Entries, got.Refused, tt.want)
			}
		})
	}
}

func TestReadEntriesRefusesAnIDTwiceAndAFileThatIsNotAList(t *testing.T) {
	entry := `{"id": 1, "c": "1ai", "p": "Ann", "s": 50, "u": null, "t": "2025-01-01T00:00:00Z", "b": 0}`
	got, err := challenges.ReadEntries(strings.NewReader("[" + entry + "," + entry + "]"))
	if err != nil || len(got.Entries) != 1 || len(got.Refused) != 1 || !strings.Contains(got.Refused[0].Reason, "twice") {
		t.Errorf("ReadEntries with id 1 twice = %+v, %v; want the second refused", got, err)
	}
	if _, err := challenges.ReadEntries(strings.NewReader(`{"entries": []}`)); err == nil {
		t.Error("ReadEntries of an object = nil error, want a refusal")
	}
}
