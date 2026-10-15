// Package runcache keeps answers computed from a rating run in memory, for the
// most recently used runs only, so memory stays bounded however many runs are asked for.
package runcache

import (
	"slices"
	"sync"
)

// Runs is how many runs a cache keeps: the current one and the one before, which pages
// cached before a publish still ask for.
const Runs = 2

// Cache holds one value per run and key, for the Runs most recently used runs. It is safe
// for concurrent use.
type Cache[K comparable, V any] struct {
	mu   sync.Mutex
	runs []int64 // most recently used first
	vals map[int64]map[K]*entry[V]
}

type entry[V any] struct {
	mu     sync.Mutex
	loaded bool
	val    V
}

// New returns an empty cache.
func New[K comparable, V any]() *Cache[K, V] {
	return &Cache[K, V]{vals: map[int64]map[K]*entry[V]{}}
}

// Get returns run's value for key, calling load the first time it is asked for and keeping
// what it returns; an error is returned and not kept. Callers asking for the same run and
// key together wait for one load; callers for other keys do not wait.
func (c *Cache[K, V]) Get(run int64, key K, load func() (V, error)) (V, error) {
	e := c.entry(run, key)
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.loaded {
		return e.val, nil
	}
	v, err := load()
	if err != nil {
		return v, err
	}
	e.val, e.loaded = v, true
	return v, nil
}

// entry returns run's entry for key, making run the most recently used and dropping the
// least recently used run past Runs.
func (c *Cache[K, V]) entry(run int64, key K) *entry[V] {
	c.mu.Lock()
	defer c.mu.Unlock()
	if i := slices.Index(c.runs, run); i >= 0 {
		c.runs = slices.Delete(c.runs, i, i+1)
	}
	c.runs = slices.Insert(c.runs, 0, run)
	if len(c.runs) > Runs {
		delete(c.vals, c.runs[Runs])
		c.runs = c.runs[:Runs]
	}
	keys, ok := c.vals[run]
	if !ok {
		keys = map[K]*entry[V]{}
		c.vals[run] = keys
	}
	e, ok := keys[key]
	if !ok {
		e = &entry[V]{}
		keys[key] = e
	}
	return e
}
