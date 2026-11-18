// Entries for /choose and /permutation: reading the list given, drawing from it and the reply.
package discordbot

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// The most entries and characters a list may have; escaped, a reply then stays within Discord's 2000 characters.
const maxItems, maxItemsLength = 100, 1000

// ParseItems reads entries separated by commas, trimmed, dropping empty ones and keeping duplicates, refusing none,
// more than 100, or over 1000 characters in all.
func ParseItems(s string) ([]string, error) {
	if utf8.RuneCountInString(s) > maxItemsLength {
		return nil, fmt.Errorf("write at most %d characters", maxItemsLength)
	}
	var items []string
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	switch {
	case len(items) == 0:
		return nil, errors.New("give names separated by commas: Alex, Beta, Claus")
	case len(items) > maxItems:
		return nil, fmt.Errorf("give at most %d names", maxItems)
	}
	return items, nil
}

// Choose draws count of items with pick, which returns a number from 0 to below the n it is given, and returns them
// in the order items has them; count must be 1 to len(items).
func Choose(items []string, count int, pick func(n int) int) ([]string, error) {
	drawn, err := draw(len(items), count, pick)
	if err != nil {
		return nil, err
	}
	slices.Sort(drawn)
	return pickedFrom(items, drawn), nil
}

// Permute draws count of items with pick, as Choose does, and returns them in the order drawn.
func Permute(items []string, count int, pick func(n int) int) ([]string, error) {
	drawn, err := draw(len(items), count, pick)
	if err != nil {
		return nil, err
	}
	return pickedFrom(items, drawn), nil
}

// draw is count of the indexes 0 to n-1 in a uniformly random order, by a partial Fisher–Yates shuffle.
func draw(n, count int, pick func(n int) int) ([]int, error) {
	if count < 1 || count > n {
		return nil, fmt.Errorf("pick 1 to %d of the %d given", n, n)
	}
	indexes := make([]int, n)
	for i := range indexes {
		indexes[i] = i
	}
	for i := range count {
		j := i + pick(n-i)
		indexes[i], indexes[j] = indexes[j], indexes[i]
	}
	return indexes[:count], nil
}

// pickedFrom is the items at indexes, in their order.
func pickedFrom(items []string, indexes []int) []string {
	picked := make([]string, len(indexes))
	for i, at := range indexes {
		picked[i] = items[at]
	}
	return picked
}

// ItemsText is the reply for items: each as written, separated by commas.
func ItemsText(items []string) string {
	escaped := make([]string, len(items))
	for i, item := range items {
		escaped[i] = EscapeMarkdown(item)
	}
	return strings.Join(escaped, ", ")
}
