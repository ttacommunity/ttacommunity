// Package configbackup exports what exists in one place only, for the nightly configuration backup to encrypt and
// upload: the namespace's Secrets, read from the Kubernetes API, and the Cloudflare account's settings that are
// not in git, read from Cloudflare's API with a read-only token.
package configbackup

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
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

// Cluster is how to reach the Kubernetes API as the pod's service account.
type Cluster struct {
	API, Namespace, Token string
	Client                *http.Client
}

// ClusterFrom builds a Cluster from the pod's environment (getenv) and the service account files Kubernetes mounts
// in dir: namespace, ca.crt and token. It refuses when any is missing.
func ClusterFrom(getenv func(string) string, dir string) (Cluster, error) {
	host, port := getenv("KUBERNETES_SERVICE_HOST"), getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return Cluster{}, errors.New("not running in a cluster: KUBERNETES_SERVICE_HOST is not set")
	}
	read := func(name string) (string, error) {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return "", fmt.Errorf("reading the service account's %s: %w", name, err)
		}
		return string(bytes.TrimSpace(b)), nil
	}
	namespace, err := read("namespace")
	if err != nil {
		return Cluster{}, err
	}
	token, err := read("token")
	if err != nil {
		return Cluster{}, err
	}
	ca, err := read("ca.crt")
	if err != nil {
		return Cluster{}, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(ca)) {
		return Cluster{}, errors.New("the cluster CA file holds no certificate")
	}
	return Cluster{
		API: "https://" + net.JoinHostPort(host, port), Namespace: namespace, Token: token,
		Client: &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}},
	}, nil
}

// secret is the part of a Secret a restore needs, with its keys in the order kubectl prints them.
type secret struct {
	APIVersion string            `json:"apiVersion"`
	Data       map[string]string `json:"data"`
	Kind       string            `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Type string `json:"type"`
}

// Secrets reads every Secret in the namespace and returns them as a List that kubectl apply accepts: each Secret's
// name, namespace, type and data, sorted by name, and nothing the cluster added.
func Secrets(ctx context.Context, c Cluster) ([]byte, error) {
	var list struct {
		Items []secret `json:"items"`
	}
	target := c.API + "/api/v1/namespaces/" + url.PathEscape(c.Namespace) + "/secrets"
	if err := getJSON(ctx, c.Client, target, c.Token, nil, &list); err != nil {
		return nil, fmt.Errorf("reading the Secrets: %w", err)
	}
	for i := range list.Items {
		list.Items[i].APIVersion, list.Items[i].Kind = "v1", "Secret"
		if list.Items[i].Data == nil {
			list.Items[i].Data = map[string]string{}
		}
	}
	sort.Slice(list.Items, func(i, j int) bool { return list.Items[i].Metadata.Name < list.Items[j].Metadata.Name })
	out := struct {
		APIVersion string   `json:"apiVersion"`
		Items      []secret `json:"items"`
		Kind       string   `json:"kind"`
	}{"v1", list.Items, "List"}
	return json.Marshal(out)
}

// Account is how to reach Cloudflare's API for the account that holds Zone, with a read-only token.
type Account struct {
	API, Token, Zone string
	Client           *http.Client
}

// jurisdiction is where the project's R2 buckets are; a listing without it does not see them.
const jurisdiction = "eu"

// perPage is the page size asked of every paged list.
const perPage = 50

// Cloudflare reads every setting of the zone's account that is not in git and returns them as one JSON object,
// keyed as docs/architecture.md lists: Zero Trust, DNS, Turnstile, Email Routing, the zone's rulesets, Workers
// Builds and R2 bucket rules. Every list is read to its last page, and any read that fails fails the export.
func Cloudflare(ctx context.Context, a Account) ([]byte, error) {
	cf := client{a}
	zoneID, accountID, err := cf.zone(ctx)
	if err != nil {
		return nil, err
	}
	acct, zone := "/accounts/"+accountID, "/zones/"+zoneID
	out := map[string]any{"zone": a.Zone}
	for _, l := range []struct {
		key, path string
		paged     bool
	}{
		{"organization", acct + "/access/organizations", false},
		{"access_applications", acct + "/access/apps", true},
		{"access_policies", acct + "/access/policies", true},
		{"access_groups", acct + "/access/groups", true},
		{"access_service_tokens", acct + "/access/service_tokens", true},
		{"identity_providers", acct + "/access/identity_providers", true},
		{"dns_records", zone + "/dns_records", true},
		{"turnstile_widgets", acct + "/challenges/widgets", true},
		{"workers", acct + "/workers/scripts", false},
	} {
		if out[l.key], err = cf.read(ctx, l.path, l.paged, nil); err != nil {
			return nil, err
		}
	}
	email := map[string]any{}
	for _, l := range []struct {
		key, path string
		paged     bool
	}{
		{"settings", zone + "/email/routing", false},
		{"rules", zone + "/email/routing/rules", true},
		{"catch_all", zone + "/email/routing/rules/catch_all", false},
		{"addresses", acct + "/email/routing/addresses", true},
	} {
		if email[l.key], err = cf.read(ctx, l.path, l.paged, nil); err != nil {
			return nil, err
		}
	}
	out["email_routing"] = email
	if out["rulesets"], err = cf.rulesets(ctx, zone); err != nil {
		return nil, err
	}
	workers, err := cf.workers(ctx, out["workers"].(json.RawMessage))
	if err != nil {
		return nil, err
	}
	if out["builds"], out["build_triggers"], out["build_variables"], err = cf.builds(ctx, acct, workers); err != nil {
		return nil, err
	}
	if out["r2_buckets"], err = cf.buckets(ctx, acct); err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// client reads Cloudflare's API for one Account.
type client struct{ a Account }

// envelope is the shape of every Cloudflare API answer.
type envelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result     json.RawMessage `json:"result"`
	ResultInfo struct {
		Page       int    `json:"page"`
		TotalPages int    `json:"total_pages"`
		Cursor     string `json:"cursor"`
	} `json:"result_info"`
}

// errNotFound is a 404: the one answer some reads may treat as "none".
var errNotFound = errors.New("not found")

// get reads one page at path with query and headers, and returns the envelope. A refused request, an answer that
// is not JSON and success false are all errors.
func (c client) get(ctx context.Context, path string, query url.Values, headers map[string]string) (envelope, error) {
	target := c.a.API + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return envelope{}, fmt.Errorf("building a Cloudflare request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.a.Token)
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.a.Client.Do(req)
	if err != nil {
		return envelope{}, fmt.Errorf("GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return envelope{}, fmt.Errorf("GET %s: reading the answer: %w", path, err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return envelope{}, fmt.Errorf("GET %s: %w", path, errNotFound)
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return envelope{}, fmt.Errorf("GET %s: Cloudflare answered %d, not JSON", path, resp.StatusCode)
	}
	if resp.StatusCode/100 != 2 || !env.Success {
		msg := "no message"
		if len(env.Errors) > 0 {
			msg = fmt.Sprintf("%d %s", env.Errors[0].Code, env.Errors[0].Message)
		}
		return envelope{}, fmt.Errorf("GET %s: Cloudflare answered %d: %s", path, resp.StatusCode, msg)
	}
	return env, nil
}

// read returns the result at path: one page, or when paged, every page's results joined into one array.
func (c client) read(ctx context.Context, path string, paged bool, headers map[string]string) (json.RawMessage, error) {
	if !paged {
		env, err := c.get(ctx, path, nil, headers)
		if err != nil {
			return nil, err
		}
		return env.Result, nil
	}
	var all []json.RawMessage
	for page := 1; ; page++ {
		env, err := c.get(ctx, path, url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(perPage)}}, headers)
		if err != nil {
			return nil, err
		}
		var items []json.RawMessage
		if err := json.Unmarshal(env.Result, &items); err != nil {
			return nil, fmt.Errorf("GET %s: the result is not a list", path)
		}
		all = append(all, items...)
		if page >= env.ResultInfo.TotalPages {
			break
		}
	}
	if all == nil {
		all = []json.RawMessage{}
	}
	return json.Marshal(all)
}

// cursored returns every page of a cursor-paged list at path, joined into one array of the items under field
// (the result itself when field is empty).
func (c client) cursored(ctx context.Context, path, field string, headers map[string]string) ([]json.RawMessage, error) {
	all := []json.RawMessage{}
	query := url.Values{"per_page": {strconv.Itoa(perPage)}}
	for {
		env, err := c.get(ctx, path, query, headers)
		if err != nil {
			return nil, err
		}
		result := env.Result
		if field != "" {
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(env.Result, &obj); err != nil {
				return nil, fmt.Errorf("GET %s: the result is not an object", path)
			}
			result = obj[field]
		}
		var items []json.RawMessage
		if len(result) > 0 {
			if err := json.Unmarshal(result, &items); err != nil {
				return nil, fmt.Errorf("GET %s: the result is not a list", path)
			}
		}
		all = append(all, items...)
		if env.ResultInfo.Cursor == "" {
			return all, nil
		}
		query.Set("cursor", env.ResultInfo.Cursor)
	}
}

// zone finds the zone by name and returns its id and its account's id. An unknown zone is an error.
func (c client) zone(ctx context.Context) (string, string, error) {
	env, err := c.get(ctx, "/zones", url.Values{"name": {c.a.Zone}}, nil)
	if err != nil {
		return "", "", err
	}
	var zones []struct {
		ID      string `json:"id"`
		Account struct {
			ID string `json:"id"`
		} `json:"account"`
	}
	if err := json.Unmarshal(env.Result, &zones); err != nil || len(zones) != 1 || zones[0].ID == "" || zones[0].Account.ID == "" {
		return "", "", fmt.Errorf("the zone %s is not one zone of this token's account", c.a.Zone)
	}
	return zones[0].ID, zones[0].Account.ID, nil
}

// rulesets returns the zone's own rulesets, each read whole (the list leaves out the rules), keyed by id.
func (c client) rulesets(ctx context.Context, zone string) (map[string]json.RawMessage, error) {
	items, err := c.cursored(ctx, zone+"/rulesets", "", nil)
	if err != nil {
		return nil, err
	}
	out := map[string]json.RawMessage{}
	for _, item := range items {
		var rs struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(item, &rs); err != nil || rs.ID == "" {
			return nil, errors.New("a ruleset in the list has no id")
		}
		if rs.Kind != "zone" {
			continue
		}
		if out[rs.ID], err = c.read(ctx, zone+"/rulesets/"+rs.ID, false, nil); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// workers returns the tag of every Worker script in the list, in order.
func (c client) workers(_ context.Context, list json.RawMessage) ([]string, error) {
	var scripts []struct {
		Tag string `json:"tag"`
	}
	if err := json.Unmarshal(list, &scripts); err != nil {
		return nil, errors.New("the Workers list is not a list")
	}
	tags := make([]string, 0, len(scripts))
	for _, s := range scripts {
		if s.Tag == "" {
			return nil, errors.New("a Worker in the list has no tag")
		}
		tags = append(tags, s.Tag)
	}
	return tags, nil
}

// builds returns, for each Worker tag that Workers Builds knows, its build configuration and its triggers, and for
// each trigger its variables, each keyed by tag or trigger. A Worker deployed by hand has no configuration (404)
// and is left out.
func (c client) builds(ctx context.Context, acct string, tags []string) (map[string]json.RawMessage, map[string]json.RawMessage, map[string]json.RawMessage, error) {
	builds, triggers, variables := map[string]json.RawMessage{}, map[string]json.RawMessage{}, map[string]json.RawMessage{}
	for _, tag := range tags {
		cfg, err := c.read(ctx, acct+"/builds/workers/"+url.PathEscape(tag), false, nil)
		if errors.Is(err, errNotFound) {
			continue
		}
		if err != nil {
			return nil, nil, nil, err
		}
		builds[tag] = cfg
		if triggers[tag], err = c.read(ctx, acct+"/builds/workers/"+url.PathEscape(tag)+"/triggers", false, nil); err != nil {
			return nil, nil, nil, err
		}
		var ts []struct {
			UUID string `json:"trigger_uuid"`
		}
		if err := json.Unmarshal(triggers[tag], &ts); err != nil {
			return nil, nil, nil, errors.New("the build triggers are not a list")
		}
		for _, t := range ts {
			if t.UUID == "" {
				return nil, nil, nil, errors.New("a build trigger has no uuid")
			}
			if variables[t.UUID], err = c.read(ctx, acct+"/builds/triggers/"+url.PathEscape(t.UUID)+"/environment_variables", false, nil); err != nil {
				return nil, nil, nil, err
			}
		}
	}
	return builds, triggers, variables, nil
}

// buckets returns each EU bucket's lifecycle and lock rules, keyed by bucket name.
func (c client) buckets(ctx context.Context, acct string) (map[string]any, error) {
	headers := map[string]string{"cf-r2-jurisdiction": jurisdiction}
	items, err := c.cursored(ctx, acct+"/r2/buckets", "buckets", headers)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, item := range items {
		var b struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(item, &b); err != nil || b.Name == "" {
			return nil, errors.New("a bucket in the list has no name")
		}
		rules := map[string]json.RawMessage{}
		for key, part := range map[string]string{"lifecycle": "lifecycle", "locks": "lock"} {
			if rules[key], err = c.read(ctx, acct+"/r2/buckets/"+url.PathEscape(b.Name)+"/"+part, false, headers); err != nil {
				return nil, err
			}
		}
		out[b.Name] = rules
	}
	return out, nil
}

// getJSON reads a 2xx JSON answer from target with a bearer token into out.
func getJSON(ctx context.Context, client *http.Client, target, token string, headers map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("building the request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return fmt.Errorf("GET %s: answered %d", req.URL.Path, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(out); err != nil {
		return fmt.Errorf("GET %s: the answer is not JSON", req.URL.Path)
	}
	return nil
}
