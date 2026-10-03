package enginejob

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const cronJobJSON = `{
  "metadata": {"name": "engine-dev", "uid": "cj-uid-1"},
  "spec": {"jobTemplate": {
    "metadata": {"labels": {"app": "engine-dev"}},
    "spec": {"backoffLimit": 0, "template": {"spec": {"containers": [{"name": "engine", "args": ["-m", "tta_ratings_engine.run"]}]}}}
  }}
}`

// fakeAPI is a Kubernetes API with one CronJob and the given jobs; it records what was created.
type fakeAPI struct {
	jobs    string
	created []map[string]any
	deleted []string
	auths   []string
	status  int
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.auths = append(f.auths, r.Header.Get("Authorization"))
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/apis/batch/v1/namespaces/ttac/cronjobs/engine-dev":
		_, _ = io.WriteString(w, cronJobJSON)
	case r.Method == http.MethodGet && r.URL.Path == "/apis/batch/v1/namespaces/ttac/jobs":
		_, _ = io.WriteString(w, `{"items": [`+f.jobs+`]}`)
	case r.Method == http.MethodPost && r.URL.Path == "/apis/batch/v1/namespaces/ttac/jobs":
		var job map[string]any
		if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.created = append(f.created, job)
		meta := job["metadata"].(map[string]any)
		meta["name"] = meta["generateName"].(string) + "x7k2p"
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(job)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/apis/batch/v1/namespaces/ttac/jobs/"):
		f.deleted = append(f.deleted, strings.TrimPrefix(r.URL.Path, "/apis/batch/v1/namespaces/ttac/jobs/")+"?"+r.URL.RawQuery)
		_, _ = io.WriteString(w, `{}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func starter(t *testing.T, f *fakeAPI) *Starter {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return &Starter{
		API: srv.URL, Namespace: "ttac", CronJob: "engine-dev", Client: srv.Client(),
		Token: func() (string, error) { return "sa-token", nil },
	}
}

func ownedJob(name, conditions string) string {
	return `{"metadata": {"name": "` + name + `", "ownerReferences": [{"kind": "CronJob", "name": "engine-dev", "uid": "cj-uid-1"}]},
	         "status": {"conditions": [` + conditions + `]}}`
}

func TestStartCreatesAJobFromTheCronJobsTemplate(t *testing.T) {
	f := &fakeAPI{jobs: ownedJob("engine-dev-29", `{"type": "Complete", "status": "True"}`)}
	name, err := starter(t, f).Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if name != "engine-dev-admin-x7k2p" {
		t.Errorf("name = %q, want the generated name", name)
	}
	if len(f.created) != 1 {
		t.Fatalf("created %d jobs, want 1", len(f.created))
	}
	job, _ := json.Marshal(f.created[0])
	for _, want := range []string{
		`"generateName":"engine-dev-admin-"`, `"cronjob.kubernetes.io/instantiate":"manual"`,
		`"app":"engine-dev"`, `"args":["-m","tta_ratings_engine.run"]`, `"backoffLimit":0`,
		`"ownerReferences":[{"apiVersion":"batch/v1","controller":true,"kind":"CronJob","name":"engine-dev","uid":"cj-uid-1"}]`,
	} {
		if !strings.Contains(string(job), want) {
			t.Errorf("job = %s, want it to contain %s", job, want)
		}
	}
	for _, a := range f.auths {
		if a != "Bearer sa-token" {
			t.Errorf("Authorization = %q, want the service account token", a)
		}
	}
}

func TestStartRefusesWhileARunIsGoing(t *testing.T) {
	for _, tt := range []struct{ name, jobs string }{
		{"no conditions yet", ownedJob("engine-dev-29", "")},
		{"a condition that is not the end", ownedJob("engine-dev-29", `{"type": "Suspended", "status": "True"}`)},
		{"complete not yet true", ownedJob("engine-dev-29", `{"type": "Complete", "status": "False"}`)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAPI{jobs: tt.jobs}
			if _, err := starter(t, f).Start(context.Background()); !errors.Is(err, ErrRunning) {
				t.Errorf("Start = %v, want ErrRunning", err)
			}
			if len(f.created) != 0 {
				t.Errorf("created %d jobs while one was running", len(f.created))
			}
		})
	}
}

func TestStartIgnoresFinishedAndOtherJobs(t *testing.T) {
	f := &fakeAPI{jobs: strings.Join([]string{
		ownedJob("engine-dev-27", `{"type": "Failed", "status": "True"}`),
		ownedJob("engine-dev-28", `{"type": "SuccessCriteriaMet", "status": "True"}, {"type": "Complete", "status": "True"}`),
		`{"metadata": {"name": "load-1"}, "status": {}}`,
		`{"metadata": {"name": "other", "ownerReferences": [{"kind": "CronJob", "name": "backup"}]}, "status": {}}`,
	}, ",")}
	if _, err := starter(t, f).Start(context.Background()); err != nil {
		t.Errorf("Start: %v", err)
	}
}

func TestStartFailsWithoutCreatingWhenTheClusterRefuses(t *testing.T) {
	f := &fakeAPI{status: http.StatusForbidden}
	_, err := starter(t, f).Start(context.Background())
	if err == nil || errors.Is(err, ErrRunning) {
		t.Errorf("Start = %v, want an error", err)
	}
	if len(f.created) != 0 {
		t.Errorf("created %d jobs", len(f.created))
	}
}

func TestStartFailsWithoutAToken(t *testing.T) {
	f := &fakeAPI{}
	s := starter(t, f)
	s.Token = func() (string, error) { return "", errors.New("no token file") }
	if _, err := s.Start(context.Background()); err == nil {
		t.Error("Start without a token succeeded")
	}
	if len(f.auths) != 0 {
		t.Errorf("made %d requests without a token", len(f.auths))
	}
}

func TestGoingSaysWhetherAnEngineJobHasNotFinished(t *testing.T) {
	for _, tt := range []struct {
		name, jobs string
		want       bool
	}{
		{"one running", ownedJob("engine-dev-29", ""), true},
		{"all finished", ownedJob("engine-dev-28", `{"type": "Complete", "status": "True"}`), false},
		{"only another CronJob's", `{"metadata": {"name": "other", "ownerReferences": [{"kind": "CronJob", "name": "backup"}]}, "status": {}}`, false},
		{"none", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := starter(t, &fakeAPI{jobs: tt.jobs}).Going(context.Background())
			if err != nil || got != tt.want {
				t.Errorf("Going = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestStopDeletesOnlyTheEngineJobsThatHaveNotFinished(t *testing.T) {
	f := &fakeAPI{jobs: strings.Join([]string{
		ownedJob("engine-dev-28", `{"type": "Complete", "status": "True"}`),
		ownedJob("engine-dev-admin-x7k2p", ""),
		`{"metadata": {"name": "load-1"}, "status": {}}`,
	}, ",")}
	stopped, err := starter(t, f).Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if strings.Join(stopped, ",") != "engine-dev-admin-x7k2p" {
		t.Errorf("stopped = %q, want the unfinished engine job", stopped)
	}
	if strings.Join(f.deleted, ",") != "engine-dev-admin-x7k2p?propagationPolicy=Background" {
		t.Errorf("deleted = %q, want that job with its pods", f.deleted)
	}
}

func TestStopRefusesWhenNothingIsGoing(t *testing.T) {
	f := &fakeAPI{jobs: ownedJob("engine-dev-28", `{"type": "Failed", "status": "True"}`)}
	if _, err := starter(t, f).Stop(context.Background()); !errors.Is(err, ErrNotRunning) {
		t.Errorf("Stop = %v, want ErrNotRunning", err)
	}
	if len(f.deleted) != 0 {
		t.Errorf("deleted %q", f.deleted)
	}
}
