// The audit log for admins: who changed what and when.

package httpapi

import (
	"context"
	"strings"
	"unicode/utf8"

	"ttacommunity/backend/internal/store"
)

func (s *server) GetAdminAudit(ctx context.Context, req GetAdminAuditRequestObject) (GetAdminAuditResponseObject, error) {
	_, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return GetAdminAudit403JSONResponse{forbidden("only admins read the audit log")}, nil
	}
	p := req.Params
	filter := store.AuditFilter{From: p.From, To: p.To}
	if p.Action != nil {
		filter.Action = *p.Action
	}
	if p.Q != nil {
		filter.Text = strings.TrimSpace(*p.Q)
	}
	page := 1
	if p.Page != nil {
		page = *p.Page
	}
	msg := ""
	switch {
	case page < 1:
		msg = "bad parameter: page"
	case utf8.RuneCountInString(filter.Action) > 100:
		msg = "an action of at most 100 characters"
	case utf8.RuneCountInString(filter.Text) > 100:
		msg = "a search of at most 100 characters"
	case filter.From != nil && filter.To != nil && filter.To.Before(*filter.From):
		msg = "to must not be before from"
	}
	if msg != "" {
		return GetAdminAudit400JSONResponse{badRequest400(msg)}, nil
	}
	log, err := s.admins.AuditLog(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	entries := make([]AdminAuditEntry, len(log.Entries))
	for i, e := range log.Entries {
		entries[i] = AdminAuditEntry{Id: e.ID, At: e.At.UTC(), Admin: e.Admin, ByPlayer: e.ByPlayer, Action: e.Action, Target: e.Target, Detail: string(e.Detail)}
	}
	body := AdminAuditPage{Page: log.Page, Pages: log.Pages, Total: log.Total, Actions: log.Actions, Entries: entries}
	return GetAdminAudit200JSONResponse{Body: body, Headers: GetAdminAudit200ResponseHeaders{CacheControl: cacheNone}}, nil
}
