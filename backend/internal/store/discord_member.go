// What a Discord account is to us, for the Discord bot: whether it signed in to the site, the member it is linked to
// and that member's ratings player. Read through the player login, which already reads sign-ins.
package store

import (
	"context"
	"fmt"
)

// The states of a Discord account, as DiscordMember reports them.
const (
	DiscordFound     = "found"
	DiscordNoPlayer  = "no_player"
	DiscordNotLinked = "not_linked"
	DiscordNoLogin   = "no_login"
)

// DiscordMember is a Discord account's state, and with DiscordFound its member's ratings player name.
type DiscordMember struct {
	State string
	Name  string
}

// DiscordMember returns what the Discord account discordID is to us.
func (s *PlayerStore) DiscordMember(ctx context.Context, discordID string) (DiscordMember, error) {
	var m DiscordMember
	var name *string
	err := s.pool.QueryRow(ctx, `
		SELECT CASE WHEN l.login_id IS NULL THEN $2
		            WHEN a.member_id IS NULL THEN $3
		            WHEN p.player_id IS NULL THEN $4
		            ELSE $5 END,
		       p.name
		FROM (SELECT 1) one
		LEFT JOIN logins l ON l.provider = 'discord' AND l.subject = $1
		LEFT JOIN accounts a ON a.account_id = l.account_id
		LEFT JOIN members m ON m.member_id = a.member_id
		LEFT JOIN players p ON p.player_id = m.player_id`,
		discordID, DiscordNoLogin, DiscordNotLinked, DiscordNoPlayer, DiscordFound).Scan(&m.State, &name)
	if err != nil {
		return DiscordMember{}, fmt.Errorf("reading what a Discord account is linked to: %w", err)
	}
	if name != nil {
		m.Name = *name
	}
	return m, nil
}
