package httpapi

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// jsonAt returns the value at a dotted path ("seasons.0.podium", or "" for the whole document) in a JSON document, and
// whether it is there.
func jsonAt(body []byte, path string) (any, bool) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, false
	}
	if path == "" {
		return v, true
	}
	for _, key := range strings.Split(path, ".") {
		switch node := v.(type) {
		case map[string]any:
			next, ok := node[key]
			if !ok {
				return nil, false
			}
			v = next
		case []any:
			i, err := strconv.Atoi(key)
			if err != nil || i < 0 || i >= len(node) {
				return nil, false
			}
			v = node[i]
		default:
			return nil, false
		}
	}
	return v, true
}

// matches reports whether got has everything want has: an object's named keys (others ignored), an array's every
// element in order and no more, and any other value exactly.
func matches(got, want any) bool {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for k, wv := range w {
			gv, ok := g[k]
			if !ok || !matches(gv, wv) {
				return false
			}
		}
		return true
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return false
		}
		for i := range w {
			if !matches(g[i], w[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(got, want)
	}
}

// wantJSON fails unless the value at path in body has everything the JSON fragment want has.
func wantJSON(t *testing.T, body []byte, path, want string) {
	t.Helper()
	var w any
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("the expected %s is not JSON: %v", path, err)
	}
	got, ok := jsonAt(body, path)
	if !ok {
		t.Errorf("%s: not in %s", path, body)
		return
	}
	if !matches(got, w) {
		g, _ := json.Marshal(got)
		t.Errorf("%s = %s, want %s", path, g, want)
	}
}

func TestWantJSONChecksOnlyWhatItNames(t *testing.T) {
	body := []byte(`{"run_id":7,"seasons":[{"label":"2024","first_game":null,"podium":[{"place":1,"name":"sora"}],"extra":true}]}`)
	for _, tt := range []struct {
		name, path, want string
		ok               bool
	}{
		{"a value", "run_id", `7`, true},
		{"the whole document", "", `{"run_id":7}`, true},
		{"a wrong value", "run_id", `8`, false},
		{"a null", "seasons.0.first_game", `null`, true},
		{"an object's named keys, ignoring the rest", "seasons.0", `{"label":"2024","podium":[{"place":1}]}`, true},
		{"a key the object lacks", "seasons.0", `{"missing":1}`, false},
		{"an array with one element too many", "seasons.0.podium", `[{"place":1},{"place":2}]`, false},
		{"an array with more elements than named", "seasons", `[]`, false},
		{"an empty array is not null", "seasons.0.first_game", `[]`, false},
		{"a path that is not there", "seasons.1", `{}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var w any
			if err := json.Unmarshal([]byte(tt.want), &w); err != nil {
				t.Fatal(err)
			}
			got, there := jsonAt(body, tt.path)
			if ok := there && matches(got, w); ok != tt.ok {
				t.Errorf("%s ~ %s = %v, want %v", tt.path, tt.want, ok, tt.ok)
			}
		})
	}
}
