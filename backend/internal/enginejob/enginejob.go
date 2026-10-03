// Package enginejob starts a rating run now, by creating a Job from the engine's CronJob through the Kubernetes API,
// as `kubectl create job --from=cronjob/...` does.
package enginejob

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

// ErrRunning means a Job of the engine's CronJob has not finished yet.
var ErrRunning = errors.New("a rating run is already going")

// ErrNotSetUp means this API has no engine CronJob to start runs from.
var ErrNotSetUp = errors.New("starting runs is not set up here")

// Starter creates engine Jobs in one namespace from one CronJob.
type Starter struct {
	API       string // the API server's base URL
	Namespace string
	CronJob   string
	Token     func() (string, error) // read on each call, as the cluster rotates it
	Client    *http.Client

	mu sync.Mutex // one start at a time, so two clicks cannot start two runs
}

// serviceAccount is where Kubernetes mounts a pod's token and the cluster's CA.
const serviceAccount = "/var/run/secrets/kubernetes.io/serviceaccount/"

// InCluster returns a Starter for a CronJob in the pod's own namespace that uses the pod's service account,
// or an error outside a cluster.
func InCluster(cronJob string) (*Starter, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, errors.New("not running in a cluster: KUBERNETES_SERVICE_HOST is not set")
	}
	ns, err := os.ReadFile(serviceAccount + "namespace")
	if err != nil {
		return nil, fmt.Errorf("reading the pod's namespace: %w", err)
	}
	namespace := string(bytes.TrimSpace(ns))
	ca, err := os.ReadFile(serviceAccount + "ca.crt")
	if err != nil {
		return nil, fmt.Errorf("reading the cluster CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("the cluster CA file holds no certificate")
	}
	return &Starter{
		API: "https://" + net.JoinHostPort(host, port), Namespace: namespace, CronJob: cronJob,
		Token: func() (string, error) {
			b, err := os.ReadFile(serviceAccount + "token")
			return string(bytes.TrimSpace(b)), err
		},
		Client: &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}},
	}, nil
}

// cronJob is the part of a CronJob a Job is made from.
type cronJob struct {
	Metadata struct {
		Name string `json:"name"`
		UID  string `json:"uid"`
	} `json:"metadata"`
	Spec struct {
		JobTemplate struct {
			Metadata struct {
				Labels      map[string]string `json:"labels,omitempty"`
				Annotations map[string]string `json:"annotations,omitempty"`
			} `json:"metadata"`
			Spec json.RawMessage `json:"spec"`
		} `json:"jobTemplate"`
	} `json:"spec"`
}

// jobList is the part of a Job list that says which Jobs are the CronJob's and whether they finished.
type jobList struct {
	Items []struct {
		Metadata struct {
			OwnerReferences []ownerRef `json:"ownerReferences"`
		} `json:"metadata"`
		Status struct {
			Conditions []condition `json:"conditions"`
		} `json:"status"`
	} `json:"items"`
}

// Start creates a Job from the CronJob's template and returns its name. It returns ErrRunning when one of the
// CronJob's Jobs, scheduled or started by hand, has not finished.
func (s *Starter) Start(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, err := s.Token()
	if err != nil {
		return "", fmt.Errorf("reading the service account token: %w", err)
	}
	base := s.API + "/apis/batch/v1/namespaces/" + s.Namespace
	var cj cronJob
	if err := s.call(ctx, token, http.MethodGet, base+"/cronjobs/"+s.CronJob, nil, &cj); err != nil {
		return "", err
	}
	var jobs jobList
	if err := s.call(ctx, token, http.MethodGet, base+"/jobs", nil, &jobs); err != nil {
		return "", err
	}
	for _, j := range jobs.Items {
		if ownedBy(j.Metadata.OwnerReferences, s.CronJob) && !finished(j.Status.Conditions) {
			return "", ErrRunning
		}
	}
	annotations := map[string]string{"cronjob.kubernetes.io/instantiate": "manual"}
	for k, v := range cj.Spec.JobTemplate.Metadata.Annotations {
		annotations[k] = v
	}
	job := map[string]any{
		"apiVersion": "batch/v1", "kind": "Job",
		"metadata": map[string]any{
			"generateName": s.CronJob + "-admin-", "labels": cj.Spec.JobTemplate.Metadata.Labels, "annotations": annotations,
			"ownerReferences": []map[string]any{{
				"apiVersion": "batch/v1", "kind": "CronJob", "name": cj.Metadata.Name, "uid": cj.Metadata.UID, "controller": true,
			}},
		},
		"spec": cj.Spec.JobTemplate.Spec,
	}
	body, err := json.Marshal(job)
	if err != nil {
		return "", fmt.Errorf("building the engine job: %w", err)
	}
	var created struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	}
	if err := s.call(ctx, token, http.MethodPost, base+"/jobs", body, &created); err != nil {
		return "", err
	}
	return created.Metadata.Name, nil
}

// call sends one request to the API server and decodes a 2xx answer into out.
func (s *Starter) call(ctx context.Context, token, method, url string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building a cluster request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return fmt.Errorf("%s %s: the cluster answered %d", method, req.URL.Path, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out); err != nil {
		return fmt.Errorf("%s %s: reading the answer: %w", method, req.URL.Path, err)
	}
	return nil
}

// ownerRef is who owns a Job.
type ownerRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// condition is one of a Job's status conditions.
type condition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

// ownedBy reports whether a CronJob of that name owns the Job.
func ownedBy(refs []ownerRef, cronJob string) bool {
	for _, r := range refs {
		if r.Kind == "CronJob" && r.Name == cronJob {
			return true
		}
	}
	return false
}

// finished reports whether a Job's conditions say it completed or failed.
func finished(conditions []condition) bool {
	for _, c := range conditions {
		if (c.Type == "Complete" || c.Type == "Failed") && c.Status == "True" {
			return true
		}
	}
	return false
}
