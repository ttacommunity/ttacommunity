// What the bot may do in a channel, worked out as Discord does from the server's roles and the channel's
// overwrites, and whether it has what a bot channel needs.
package notify

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// Discord's permission bits that a bot channel needs or checks.
const (
	administrator        uint64 = 1 << 3
	viewChannel          uint64 = 1 << 10
	manageThreads        uint64 = 1 << 34
	createPrivateThreads uint64 = 1 << 36
	sendInThreads        uint64 = 1 << 38
	allPermissions              = ^uint64(0)
)

// Discord's application flags for the Server Members intent: in 100 servers or more, and in fewer.
const (
	membersIntent        = 1 << 14
	membersIntentLimited = 1 << 15
)

// overwrite is a channel's permission overwrite for a role (Kind 0) or a member (Kind 1).
type overwrite struct {
	ID          string
	Kind        int
	Allow, Deny uint64
}

// channelPermissions is what member user, with the roles memberOf, may do in a channel of server guild owned by
// owner, as Discord works it out: everything for the owner or an administrator; else @everyone's (the role whose id
// is the server's) and its roles' permissions, then the channel's overwrites for @everyone, for its roles (their
// denies, then their allows) and for the member.
func channelPermissions(guild, owner, user string, roles map[string]uint64, memberOf []string, overwrites []overwrite) uint64 {
	if user == owner {
		return allPermissions
	}
	perms := roles[guild]
	for _, r := range memberOf {
		perms |= roles[r]
	}
	if perms&administrator != 0 {
		return allPermissions
	}
	find := func(id string, kind int) (overwrite, bool) {
		i := slices.IndexFunc(overwrites, func(o overwrite) bool { return o.ID == id && o.Kind == kind })
		if i < 0 {
			return overwrite{}, false
		}
		return overwrites[i], true
	}
	if o, ok := find(guild, 0); ok {
		perms = perms&^o.Deny | o.Allow
	}
	var allow, deny uint64
	for _, r := range memberOf {
		if o, ok := find(r, 0); ok {
			allow, deny = allow|o.Allow, deny|o.Deny
		}
	}
	perms = perms&^deny | allow
	if o, ok := find(user, 1); ok {
		perms = perms&^o.Deny | o.Allow
	}
	return perms
}

// LackingError means the bot lacks, in a bot channel, what it needs (Needed) or what one step needs (Wanted), each
// named as Discord's settings name it.
type LackingError struct{ Needed, Wanted []string }

func (e *LackingError) Error() string {
	var parts []string
	if len(e.Needed) > 0 {
		parts = append(parts, "the bot lacks "+list(e.Needed))
	}
	if len(e.Wanted) > 0 {
		parts = append(parts, "without "+list(e.Wanted)+" it cannot lock the threads of games a round paired again no longer has")
	}
	return strings.Join(parts, "; ")
}

// needs names what a bot channel needs, by bit; Manage Threads is only wanted.
var needs = []struct {
	bit  uint64
	name string
}{
	{viewChannel, "View Channel"},
	{createPrivateThreads, "Create Private Threads"},
	{sendInThreads, "Send Messages in Threads"},
}

// CheckChannel says whether the bot has what a bot channel needs in channel: the permissions to make and post in
// private threads, and the Server Members intent to find players. It returns nil, or a *LackingError naming what it
// lacks; Manage Threads lacking alone is a LackingError with Wanted only.
func (b *Bot) CheckChannel(ctx context.Context, channel string) error {
	type rawOverwrite struct {
		ID    string `json:"id"`
		Type  int    `json:"type"`
		Allow string `json:"allow"`
		Deny  string `json:"deny"`
	}
	var c struct {
		GuildID    string         `json:"guild_id"`
		Overwrites []rawOverwrite `json:"permission_overwrites"`
	}
	if err := b.call(ctx, http.MethodGet, "/channels/"+url.PathEscape(channel), nil, &c); err != nil {
		return err
	}
	if c.GuildID == "" {
		return errors.New("the bot's channel is in no server")
	}
	var me struct {
		ID string `json:"id"`
	}
	var app struct {
		Flags int `json:"flags"`
	}
	var guild struct {
		OwnerID string `json:"owner_id"`
		Roles   []struct {
			ID          string `json:"id"`
			Permissions string `json:"permissions"`
		} `json:"roles"`
	}
	var member struct {
		Roles []string `json:"roles"`
	}
	g := "/guilds/" + url.PathEscape(c.GuildID)
	for _, step := range []struct {
		path string
		out  any
	}{{"/users/@me", &me}, {"/applications/@me", &app}, {g, &guild}} {
		if err := b.call(ctx, http.MethodGet, step.path, nil, step.out); err != nil {
			return err
		}
	}
	if err := b.call(ctx, http.MethodGet, g+"/members/"+url.PathEscape(me.ID), nil, &member); err != nil {
		return err
	}
	bits := func(s string) uint64 { v, _ := strconv.ParseUint(s, 10, 64); return v }
	roles := map[string]uint64{}
	for _, r := range guild.Roles {
		roles[r.ID] = bits(r.Permissions)
	}
	overwrites := make([]overwrite, len(c.Overwrites))
	for i, o := range c.Overwrites {
		overwrites[i] = overwrite{ID: o.ID, Kind: o.Type, Allow: bits(o.Allow), Deny: bits(o.Deny)}
	}
	perms := channelPermissions(c.GuildID, guild.OwnerID, me.ID, roles, member.Roles, overwrites)
	lacking := &LackingError{}
	for _, n := range needs {
		if perms&n.bit == 0 {
			lacking.Needed = append(lacking.Needed, n.name)
		}
	}
	if app.Flags&(membersIntent|membersIntentLimited) == 0 {
		lacking.Needed = append(lacking.Needed, "the Server Members Intent (the app's Bot page)")
	}
	if perms&manageThreads == 0 {
		lacking.Wanted = append(lacking.Wanted, "Manage Threads")
	}
	if len(lacking.Needed) == 0 && len(lacking.Wanted) == 0 {
		return nil
	}
	return lacking
}
