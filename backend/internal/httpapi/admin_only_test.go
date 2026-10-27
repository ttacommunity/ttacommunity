package httpapi

import (
	"context"
	"testing"
)

func TestAnAdminOnlyRouteReachedWithoutTheGateIsAnErrorNotARefusal(t *testing.T) {
	s, ctx := &server{}, context.Background()
	tests := []struct {
		name string
		call func() (any, error)
	}{
		{"GetAdminBadges", func() (any, error) { return s.GetAdminBadges(ctx, GetAdminBadgesRequestObject{}) }},
		{"PostAdminBadge", func() (any, error) { return s.PostAdminBadge(ctx, PostAdminBadgeRequestObject{}) }},
		{"PutAdminBadge", func() (any, error) { return s.PutAdminBadge(ctx, PutAdminBadgeRequestObject{}) }},
		{"DeleteAdminBadge", func() (any, error) { return s.DeleteAdminBadge(ctx, DeleteAdminBadgeRequestObject{}) }},
		{"GetAdminAdmins", func() (any, error) { return s.GetAdminAdmins(ctx, GetAdminAdminsRequestObject{}) }},
		{"PutAdminAdmin", func() (any, error) { return s.PutAdminAdmin(ctx, PutAdminAdminRequestObject{}) }},
		{"DeleteAdminAdmin", func() (any, error) { return s.DeleteAdminAdmin(ctx, DeleteAdminAdminRequestObject{}) }},
		{"GetAdminCommunityEditions", func() (any, error) { return s.GetAdminCommunityEditions(ctx, GetAdminCommunityEditionsRequestObject{}) }},
		{"PostAdminCommunityEdition", func() (any, error) { return s.PostAdminCommunityEdition(ctx, PostAdminCommunityEditionRequestObject{}) }},
		{"PatchAdminCommunityEdition", func() (any, error) {
			return s.PatchAdminCommunityEdition(ctx, PatchAdminCommunityEditionRequestObject{})
		}},
		{"DeleteAdminAccount", func() (any, error) { return s.DeleteAdminAccount(ctx, DeleteAdminAccountRequestObject{}) }},
		{"DeleteAdminMember", func() (any, error) { return s.DeleteAdminMember(ctx, DeleteAdminMemberRequestObject{}) }},
		{"PutAdminMemberBan", func() (any, error) { return s.PutAdminMemberBan(ctx, PutAdminMemberBanRequestObject{}) }},
		{"DeleteAdminMemberBan", func() (any, error) { return s.DeleteAdminMemberBan(ctx, DeleteAdminMemberBanRequestObject{}) }},
		{"GetAdminRuns", func() (any, error) { return s.GetAdminRuns(ctx, GetAdminRunsRequestObject{}) }},
		{"PostAdminRun", func() (any, error) { return s.PostAdminRun(ctx, PostAdminRunRequestObject{}) }},
		{"PostAdminRunPublish", func() (any, error) { return s.PostAdminRunPublish(ctx, PostAdminRunPublishRequestObject{}) }},
		{"GetAdminRunChanges", func() (any, error) { return s.GetAdminRunChanges(ctx, GetAdminRunChangesRequestObject{}) }},
		{"PostAdminRunStop", func() (any, error) { return s.PostAdminRunStop(ctx, PostAdminRunStopRequestObject{}) }},
		{"PatchAdminRun", func() (any, error) { return s.PatchAdminRun(ctx, PatchAdminRunRequestObject{}) }},
		{"DeleteAdminRun", func() (any, error) { return s.DeleteAdminRun(ctx, DeleteAdminRunRequestObject{}) }},
		{"GetAdminTitles", func() (any, error) { return s.GetAdminTitles(ctx, GetAdminTitlesRequestObject{}) }},
		{"PutAdminTitle", func() (any, error) { return s.PutAdminTitle(ctx, PutAdminTitleRequestObject{}) }},
		{"DeleteAdminTitle", func() (any, error) { return s.DeleteAdminTitle(ctx, DeleteAdminTitleRequestObject{}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.call()
			if err == nil || got != nil {
				t.Errorf("%s without the gate = %v, %v; want no response and an error", tt.name, got, err)
			}
		})
	}
}
