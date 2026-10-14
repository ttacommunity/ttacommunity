package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"ttacommunity/backend/internal/store"
)

func (f *fakeAdmins) findMember(id int64) (*store.Member, error) {
	for i := range f.members {
		if f.members[i].ID == id {
			return &f.members[i], nil
		}
	}
	return nil, store.ErrNoMember
}

func (f *fakeAdmins) Members(context.Context) ([]store.Member, error) {
	return append([]store.Member{}, f.members...), nil
}

func (f *fakeAdmins) AddMember(_ context.Context, m store.Member) (int64, error) {
	m.Name = strings.TrimSpace(m.Name)
	if m.Name == "" || m.Zone == "" || (m.Country != "" && len(m.Country) != 2) {
		return 0, store.ErrBadMember
	}
	for _, x := range f.members {
		if strings.EqualFold(x.Name, m.Name) {
			return 0, &store.MemberExistsError{Name: x.Name}
		}
	}
	m.ID, m.Country, m.CreatedAt = int64(101+len(f.members)), strings.ToUpper(m.Country), time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	if strings.EqualFold(m.Name, "bob") {
		cge := 501
		m.Name, m.Player, m.CGEUser = "bob", "Bob", &cge
	}
	f.members = append(f.members, m)
	f.notes = append(f.notes, "add member "+m.Name)
	return m.ID, nil
}

func (f *fakeAdmins) SetMember(_ context.Context, id int64, c store.MemberChange) error {
	m, err := f.findMember(id)
	if err != nil {
		return err
	}
	if c.CGEUser != nil && *c.CGEUser == 999 {
		return store.ErrNoCGEAccount
	}
	if c.CGEUser != nil && *c.CGEUser == 501 {
		return store.ErrCGEAccountTaken
	}
	if c.Name != nil {
		for _, x := range f.members {
			if x.ID != id && strings.EqualFold(x.Name, *c.Name) {
				return &store.MemberExistsError{Name: x.Name}
			}
		}
		m.Name = *c.Name
	}
	var changed []string
	for _, f := range []struct {
		what  string
		field *string
	}{{"discord", c.DiscordName}, {"email", c.Email}, {"country", c.Country}, {"zone", c.Zone}} {
		if f.field != nil {
			changed = append(changed, f.what+"="+*f.field)
		}
	}
	if c.DiscordName != nil {
		m.DiscordName = *c.DiscordName
	}
	if c.Email != nil {
		m.Email = *c.Email
	}
	if c.Country != nil {
		m.Country = strings.ToUpper(*c.Country)
	}
	if c.Zone != nil {
		m.Zone = *c.Zone
	}
	f.notes = append(f.notes, fmt.Sprintf("set member %d %s", id, strings.Join(changed, " ")))
	return nil
}

func (f *fakeAdmins) BanMember(_ context.Context, id int64, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return store.ErrBadMember
	}
	m, err := f.findMember(id)
	if err != nil {
		return err
	}
	at := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	m.Banned, m.BanReason, m.BannedAt = true, strings.TrimSpace(reason), &at
	f.notes = append(f.notes, fmt.Sprintf("ban member %d: %s", id, m.BanReason))
	return nil
}

func (f *fakeAdmins) UnbanMember(_ context.Context, id int64) error {
	m, err := f.findMember(id)
	if err != nil {
		return err
	}
	m.Banned, m.BanReason, m.BannedAt = false, "", nil
	f.notes = append(f.notes, fmt.Sprintf("unban member %d", id))
	return nil
}

func (f *fakeAdmins) RemoveMember(_ context.Context, id int64) error {
	for i, m := range f.members {
		if m.ID != id {
			continue
		}
		if m.Entries > 0 {
			return store.ErrMemberInUse
		}
		f.members = append(f.members[:i], f.members[i+1:]...)
		f.notes = append(f.notes, fmt.Sprintf("remove member %d", id))
		return nil
	}
	return store.ErrNoMember
}

func TestAnOrganiserAddsListsAndChangesMembers(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	do := func(method, path, body string) *httptest.ResponseRecorder {
		return adminDoAs(t, f, org, method, path, site, body)
	}
	rec := do(http.MethodPost, "/v1/admin/members", `{"name": " BOB ", "email": "bob@example.test", "country": "us", "zone": "America/New_York"}`)
	var added MemberAdded
	if err := json.Unmarshal(rec.Body.Bytes(), &added); err != nil || rec.Code != http.StatusCreated || added.Id != 101 {
		t.Fatalf("POST members = %d %s, want 201 with id 101", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPost, "/v1/admin/members", `{"name": "Ann"}`); rec.Code != http.StatusCreated {
		t.Fatalf("POST members (Ann, zone unknown) = %d %s, want 201", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPatch, "/v1/admin/members/102", `{"discord_name": "ann#1", "country": "de"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH member = %d %s, want 204", rec.Code, rec.Body)
	}

	rec = do(http.MethodGet, "/v1/admin/members", "")
	var members []AdminMember
	if err := json.Unmarshal(rec.Body.Bytes(), &members); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET members = %d %s", rec.Code, rec.Body)
	}
	us, de, cge := "US", "DE", 501
	want := []AdminMember{
		{Id: 101, Name: "bob", Player: "Bob", CgeUser: &cge, Email: "bob@example.test", Country: &us, Zone: "America/New_York", UtcOffset: "-04:00", BanReason: "", CreatedAt: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)},
		{Id: 102, Name: "Ann", DiscordName: "ann#1", Country: &de, Zone: "Unknown", UtcOffset: "", CreatedAt: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)},
	}
	if diff := cmp.Diff(want, members); diff != "" {
		t.Errorf("members (-want +got):\n%s", diff)
	}
	for _, step := range []struct {
		method, path string
	}{{http.MethodPut, "/v1/admin/members/102/ban"}, {http.MethodDelete, "/v1/admin/members/102/ban"}, {http.MethodDelete, "/v1/admin/members/102"}} {
		if rec := do(step.method, step.path, `{"reason": "x"}`); rec.Code != http.StatusForbidden {
			t.Errorf("an organiser's %s %s = %d, want 403", step.method, step.path, rec.Code)
		}
	}
	if want := "add member bob|add member Ann|set member 102 discord=ann#1 country=de"; strings.Join(f.notes, "|") != want {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	for i, w := range []string{"member.add 101", "member.add 102", "member.set 102"} {
		if i >= len(f.audits) || f.audits[i] != org+" "+w {
			t.Errorf("audit %d = %q, want %q", i, f.audits, org+" "+w)
		}
	}
}

func TestAnAdminBansAndRemovesMembers(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	f := &fakeAdmins{members: []store.Member{{ID: 101, Name: "bob", Zone: "UTC", Entries: 1}, {ID: 102, Name: "Ann", Zone: "UTC"}}}
	do := func(method, path, body string) *httptest.ResponseRecorder {
		return adminDoAs(t, f, owner, method, path, site, body)
	}
	for _, step := range []struct {
		method, path, body string
		want               int
	}{
		{http.MethodPut, "/v1/admin/members/102/ban", `{"reason": " Abusive "}`, http.StatusNoContent},
		{http.MethodDelete, "/v1/admin/members/102/ban", "", http.StatusNoContent},
		{http.MethodDelete, "/v1/admin/members/101", "", http.StatusConflict},
		{http.MethodDelete, "/v1/admin/members/102", "", http.StatusNoContent},
	} {
		if rec := do(step.method, step.path, step.body); rec.Code != step.want {
			t.Errorf("%s %s = %d %s, want %d", step.method, step.path, rec.Code, rec.Body, step.want)
		}
	}
	if want := "ban member 102: Abusive|unban member 102|remove member 102"; strings.Join(f.notes, "|") != want {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	for i, w := range []string{"member.ban 102", "member.unban 102", "member.remove 102"} {
		if i >= len(f.audits) || f.audits[i] != owner+" "+w {
			t.Errorf("audit %d = %q, want %q", i, f.audits, owner+" "+w)
		}
	}
}

func TestMemberChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	tests := []struct {
		name, method, path, origin, body string
		want                             int
		wantSaid                         string
	}{
		{"another site", http.MethodPost, "/v1/admin/members", "https://evil.example", `{"name": "x"}`, http.StatusForbidden, ""},
		{"no name", http.MethodPost, "/v1/admin/members", site, `{"name": " "}`, http.StatusBadRequest, ""},
		{"a name already a member", http.MethodPost, "/v1/admin/members", site, `{"name": "ANN"}`, http.StatusConflict, "Ann"},
		{"a zone that is not an IANA name", http.MethodPost, "/v1/admin/members", site, `{"name": "x", "zone": "EST"}`, http.StatusBadRequest, "EST"},
		{"a country that is not a code", http.MethodPost, "/v1/admin/members", site, `{"name": "x", "country": "D"}`, http.StatusBadRequest, ""},
		{"nothing to change", http.MethodPatch, "/v1/admin/members/102", site, `{}`, http.StatusBadRequest, ""},
		{"changing an unknown member", http.MethodPatch, "/v1/admin/members/999", site, `{"zone": "UTC"}`, http.StatusNotFound, ""},
		{"a CGE id the data does not know", http.MethodPatch, "/v1/admin/members/102", site, `{"cge_user": 999}`, http.StatusBadRequest, ""},
		{"a CGE id another member has", http.MethodPatch, "/v1/admin/members/102", site, `{"cge_user": 501}`, http.StatusConflict, ""},
		{"renaming to a name taken", http.MethodPatch, "/v1/admin/members/102", site, `{"name": "bob"}`, http.StatusConflict, "bob"},
		{"a ban without a reason", http.MethodPut, "/v1/admin/members/102/ban", site, `{"reason": " "}`, http.StatusBadRequest, ""},
		{"banning an unknown member", http.MethodPut, "/v1/admin/members/999/ban", site, `{"reason": "x"}`, http.StatusNotFound, ""},
		{"removing an unknown member", http.MethodDelete, "/v1/admin/members/999", site, "", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{members: []store.Member{{ID: 101, Name: "bob", Zone: "UTC"}, {ID: 102, Name: "Ann", Zone: "UTC"}}}
			rec := adminDoAs(t, f, owner, tt.method, tt.path, tt.origin, tt.body)
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if tt.wantSaid != "" && !strings.Contains(rec.Body.String(), tt.wantSaid) {
				t.Errorf("%s %s said %s, want it to name %q", tt.method, tt.path, rec.Body, tt.wantSaid)
			}
			if len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
}
