package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/enginejob"
)

func TestAnAdminListsRuns(t *testing.T) {
	rec := adminDoAs(t, &fakeAdmins{}, "111111111111111111", http.MethodGet, "/v1/admin/runs", "", "")
	var got []AdminRun
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET runs = %d %s", rec.Code, rec.Body)
	}
	if len(got) != 6 || got[0].Id != 9 || got[0].Progress != "[3/10] models 2p" || !got[2].Current || got[2].PublishedAt == nil || got[1].PublishedAt != nil {
		t.Errorf("runs = %+v", got)
	}
}

func TestAnAdminStartsAndPublishesRunsAndEachIsAudited(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	f := &fakeAdmins{}
	rec := adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/runs", site, "")
	var started StartedRun
	if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil || rec.Code != http.StatusAccepted || started.Job != "engine-dev-admin-x7k2p" {
		t.Fatalf("POST runs = %d %s, want 202 with the job", rec.Code, rec.Body)
	}
	if rec := adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/runs/8/publish", site, ""); rec.Code != http.StatusNoContent {
		t.Errorf("publish 8 = %d %s, want 204", rec.Code, rec.Body)
	}
	if want := "start|publish 8"; strings.Join(f.notes, "|") != want {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	if want := owner + " run.start engine-dev-admin-x7k2p|" + owner + " run.publish 8"; strings.Join(f.audits, "|") != want {
		t.Errorf("audits = %q, want %q", f.audits, want)
	}
}

func TestPublishingARunPrewarmsItsPlayerPages(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	f := &fakeAdmins{prewarmed: make(chan int64, 1)}
	if rec := adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/runs/9/publish", site, ""); rec.Code != http.StatusConflict {
		t.Fatalf("publish 9 = %d %s, want 409", rec.Code, rec.Body)
	}
	if rec := adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/runs/8/publish", site, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("publish 8 = %d %s, want 204", rec.Code, rec.Body)
	}
	select {
	case got := <-f.prewarmed:
		if got != 8 {
			t.Errorf("the background prewarm loaded run %d, want 8 (the refused 9 is never loaded)", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the background prewarm never ran")
	}
}

func TestRunChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site, owner, org = "https://ttac-admin.example", "111111111111111111", "222222222222222222"
	tests := []struct {
		name, who, method, path, origin string
		startErr                        error // also what a stop returns
		want                            int
	}{
		{"an organiser lists runs", org, http.MethodGet, "/v1/admin/runs", "", nil, http.StatusForbidden},
		{"an organiser starts a run", org, http.MethodPost, "/v1/admin/runs", site, nil, http.StatusForbidden},
		{"an organiser publishes", org, http.MethodPost, "/v1/admin/runs/8/publish", site, nil, http.StatusForbidden},
		{"an organiser compares", org, http.MethodGet, "/v1/admin/runs/8/changes", "", nil, http.StatusForbidden},
		{"a start from another site", owner, http.MethodPost, "/v1/admin/runs", "https://evil.example", nil, http.StatusForbidden},
		{"a start while a run is going", owner, http.MethodPost, "/v1/admin/runs", site, enginejob.ErrRunning, http.StatusConflict},
		{"a start where runs are not set up", owner, http.MethodPost, "/v1/admin/runs", site, enginejob.ErrNotSetUp, http.StatusServiceUnavailable},
		{"publishing a running run", owner, http.MethodPost, "/v1/admin/runs/9/publish", site, nil, http.StatusConflict},
		{"publishing no run", owner, http.MethodPost, "/v1/admin/runs/99/publish", site, nil, http.StatusNotFound},
		{"publishing from another site", owner, http.MethodPost, "/v1/admin/runs/8/publish", "https://evil.example", nil, http.StatusForbidden},
		{"an organiser stops a run", org, http.MethodPost, "/v1/admin/runs/9/stop", site, nil, http.StatusForbidden},
		{"a stop from another site", owner, http.MethodPost, "/v1/admin/runs/9/stop", "https://evil.example", nil, http.StatusForbidden},
		{"a stop of a run that is not the one going", owner, http.MethodPost, "/v1/admin/runs/8/stop", site, nil, http.StatusConflict},
		{"a stop of no run", owner, http.MethodPost, "/v1/admin/runs/99/stop", site, nil, http.StatusConflict},
		{"a stop with nothing going", owner, http.MethodPost, "/v1/admin/runs/9/stop", site, enginejob.ErrNotRunning, http.StatusConflict},
		{"a stop where runs are not set up", owner, http.MethodPost, "/v1/admin/runs/9/stop", site, enginejob.ErrNotSetUp, http.StatusServiceUnavailable},
		{"an organiser deletes a run", org, http.MethodDelete, "/v1/admin/runs/8", site, nil, http.StatusForbidden},
		{"a delete from another site", owner, http.MethodDelete, "/v1/admin/runs/8", "https://evil.example", nil, http.StatusForbidden},
		{"deleting a protected run", owner, http.MethodDelete, "/v1/admin/runs/4", site, nil, http.StatusConflict},
		{"deleting the current run", owner, http.MethodDelete, "/v1/admin/runs/7", site, nil, http.StatusConflict},
		{"deleting the running run", owner, http.MethodDelete, "/v1/admin/runs/9", site, nil, http.StatusConflict},
		{"deleting no run", owner, http.MethodDelete, "/v1/admin/runs/99", site, nil, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{startErr: tt.startErr, stopErr: tt.startErr}
			rec := adminDoAs(t, f, tt.who, tt.method, tt.path, tt.origin, "")
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
}

func TestRunChangesCompareWithThePublishedRunUnlessAsked(t *testing.T) {
	const owner = "111111111111111111"
	tests := []struct {
		name, path   string
		want         int
		run, against int64
	}{
		{"against the published run", "/v1/admin/runs/9/changes", http.StatusOK, 9, 7},
		{"the published run against the run published before it", "/v1/admin/runs/7/changes", http.StatusOK, 7, 5},
		{"a run published later against the run published before it", "/v1/admin/runs/4/changes", http.StatusOK, 4, 7},
		{"a run never published against the published run", "/v1/admin/runs/6/changes", http.StatusOK, 6, 7},
		{"the first run published, with nothing before it", "/v1/admin/runs/5/changes", http.StatusNotFound, 0, 0},
		{"against a run asked for", "/v1/admin/runs/9/changes?against=8", http.StatusOK, 9, 8},
		{"a run that does not exist", "/v1/admin/runs/99/changes", http.StatusNotFound, 0, 0},
		{"against a run that does not exist", "/v1/admin/runs/9/changes?against=99", http.StatusNotFound, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := adminDoAs(t, &fakeAdmins{}, owner, http.MethodGet, tt.path, "", "")
			if rec.Code != tt.want {
				t.Fatalf("GET %s = %d %s, want %d", tt.path, rec.Code, rec.Body, tt.want)
			}
			if tt.want != http.StatusOK {
				return
			}
			var got RunChanges
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Run != tt.run || got.Against != tt.against || got.Ratings != int(tt.run) || got.RatingsBefore != int(tt.against) || got.Biggest[0].Change != -120 {
				t.Errorf("changes = %+v, want run %d against %d", got, tt.run, tt.against)
			}
		})
	}
}

func TestAnAdminStopsTheRunThatIsGoing(t *testing.T) {
	const owner = "111111111111111111"
	f := &fakeAdmins{}
	if rec := adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/runs/9/stop", "https://ttac-admin.example", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("POST stop = %d %s, want 204", rec.Code, rec.Body)
	}
	if strings.Join(f.notes, "|") != "stop" || strings.Join(f.audits, "|") != owner+" run.stop engine-dev-admin-x7k2p" {
		t.Errorf("changes %q, audits %q", f.notes, f.audits)
	}
}

func TestARunWhoseEngineEndedIsShownFailed(t *testing.T) {
	for _, tt := range []struct {
		engine, status, progress string
	}{
		{"", "running", "[3/10] models 2p"},
		{"ended", "failed", "the engine ended without finishing this run"},
		{"unset", "running", "[3/10] models 2p"},
		{"unreachable", "running", "[3/10] models 2p"},
	} {
		t.Run("engine "+tt.engine, func(t *testing.T) {
			rec := adminDoAs(t, &fakeAdmins{engine: tt.engine}, "111111111111111111", http.MethodGet, "/v1/admin/runs", "", "")
			var got []AdminRun
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
				t.Fatalf("GET runs = %d %s", rec.Code, rec.Body)
			}
			if string(got[0].Status) != tt.status || got[0].Progress != tt.progress || got[1].Status != "complete" {
				t.Errorf("runs = %+v, want run 9 %s with %q", got, tt.status, tt.progress)
			}
		})
	}
}

func TestAnAdminProtectsAndDeletesRunsAndEachIsAudited(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	f := &fakeAdmins{deleted: make(chan int64, 1)}
	if rec := adminDoAs(t, f, owner, http.MethodPatch, "/v1/admin/runs/5", site, `{"protected": true}`); rec.Code != http.StatusNoContent {
		t.Errorf("protect 5 = %d %s, want 204", rec.Code, rec.Body)
	}
	if rec := adminDoAs(t, f, owner, http.MethodPatch, "/v1/admin/runs/99", site, `{"protected": true}`); rec.Code != http.StatusNotFound {
		t.Errorf("protect 99 = %d %s, want 404", rec.Code, rec.Body)
	}
	if rec := adminDoAs(t, f, owner, http.MethodDelete, "/v1/admin/runs/8", site, ""); rec.Code != http.StatusAccepted {
		t.Fatalf("delete 8 = %d %s, want 202", rec.Code, rec.Body)
	}
	select {
	case got := <-f.deleted:
		if got != 8 {
			t.Errorf("the background delete removed run %d, want 8", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the background delete never ran")
	}
	if want := "protect 5 true|delete 8"; strings.Join(f.notes, "|") != want {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	if want := owner + " run.protect 5|" + owner + " run.delete 8"; strings.Join(f.audits, "|") != want {
		t.Errorf("audits = %q, want %q", f.audits, want)
	}
}

func TestACrashedRunCanBeDeletedWhenTheEngineIsNotGoing(t *testing.T) {
	f := &fakeAdmins{engine: "ended", deleted: make(chan int64, 1)}
	if rec := adminDoAs(t, f, "111111111111111111", http.MethodDelete, "/v1/admin/runs/9", "https://ttac-admin.example", ""); rec.Code != http.StatusAccepted {
		t.Errorf("delete 9 with the engine ended = %d %s, want 202", rec.Code, rec.Body)
	}
	<-f.deleted
}

func TestAdminRunsGiveEachRunsSize(t *testing.T) {
	rec := adminDoAs(t, &fakeAdmins{}, "111111111111111111", http.MethodGet, "/v1/admin/runs", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET runs = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "0", `{"id": 9, "bytes": 0}`)
	wantJSON(t, rec.Body.Bytes(), "1", `{"id": 8, "bytes": 3500000000, "disk_bytes": 9000000000}`)
	wantJSON(t, rec.Body.Bytes(), "2", `{"id": 7, "bytes": 4000000000}`)
}

func TestProtectingARunIsRefusedWhenItShouldBe(t *testing.T) {
	const site, owner, org = "https://ttac-admin.example", "111111111111111111", "222222222222222222"
	for _, tt := range []struct{ name, who, origin string }{
		{"an organiser", org, site},
		{"another site", owner, "https://evil.example"},
	} {
		f := &fakeAdmins{}
		rec := adminDoAs(t, f, tt.who, http.MethodPatch, "/v1/admin/runs/8", tt.origin, `{"protected": true}`)
		if rec.Code != http.StatusForbidden || len(f.notes)+len(f.audits) != 0 {
			t.Errorf("%s: PATCH protected = %d %s, changes %q; want 403 and nothing changed", tt.name, rec.Code, rec.Body, f.notes)
		}
	}
}
