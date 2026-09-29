// Package round rounds numbers to one decimal the two ways upstream's site
// does: Python's round() in its routes, SQLite's ROUND() in its queries.
package round

import (
	"math"
	"strconv"
)

// Tenth rounds x to one decimal as Python's round(x, 1) does: from the exact
// binary value, ties to even.
func Tenth(x float64) float64 {
	r, _ := strconv.ParseFloat(strconv.FormatFloat(x, 'f', 1, 64), 64)
	return r
}

// TenthAway rounds x to one decimal as SQLite's ROUND(x, 1) does: halves away from zero.
func TenthAway(x float64) float64 {
	return math.Round(x*10) / 10
}
