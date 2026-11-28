package store

import "strings"

// typedCode returns a share code as typed, trimmed and in capitals, as the database stores it, so it matches.
func typedCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}
