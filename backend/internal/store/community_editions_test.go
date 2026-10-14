//go:build integration

package store

import (
	"context"
	"testing"
)

func TestCommunityEditionsCRUD(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	r := NewRatingsReader(asReader(t, pool))

	// List initially
	initial, err := a.AdminCommunityEditions(ctx)
	if err != nil {
		t.Fatalf("AdminCommunityEditions error: %v", err)
	}

	// Create a new edition
	ed := CommunityEdition{
		Slug:           "race26_test",
		Name:           "Race to 2026 Test",
		Title:          "Official Race 2026 Title",
		ShortName:      "Race26T",
		Badge:          "⭐ Community",
		Maintainer:     "wolvs",
		MaintainerNote: "Integration test note",
		Year:           2026,
		CountingEvents: 8,
		Visible:        true,
		SortOrder:      100,
	}
	if err := a.CreateCommunityEdition(ctx, ed); err != nil {
		t.Fatalf("CreateCommunityEdition error: %v", err)
	}

	// Read visible editions from public reader
	pubList, err := r.CommunityEditions(ctx)
	if err != nil {
		t.Fatalf("CommunityEditions error: %v", err)
	}
	var found bool
	for _, e := range pubList {
		if e.Slug == "race26_test" {
			found = true
			if e.Name != ed.Name || e.CountingEvents != 8 {
				t.Errorf("got edition %+v, want %+v", e, ed)
			}
		}
	}
	if !found {
		t.Errorf("new edition not found in public list: %+v", pubList)
	}

	// Update edition: make invisible and change name
	newName := "Race 2026 Renamed"
	invisible := false
	if err := a.SetCommunityEdition(ctx, "race26_test", CommunityEditionChange{
		Name:    &newName,
		Visible: &invisible,
	}); err != nil {
		t.Fatalf("SetCommunityEdition error: %v", err)
	}

	// Verify not in public visible list anymore
	pubListAfter, err := r.CommunityEditions(ctx)
	if err != nil {
		t.Fatalf("CommunityEditions after error: %v", err)
	}
	for _, e := range pubListAfter {
		if e.Slug == "race26_test" {
			t.Errorf("expected race26_test to be hidden, but was visible: %+v", e)
		}
	}

	// Still visible in admin list
	adminList, err := a.AdminCommunityEditions(ctx)
	if err != nil {
		t.Fatalf("AdminCommunityEditions error: %v", err)
	}
	if len(adminList) != len(initial)+1 {
		t.Errorf("admin list count got %d, want %d", len(adminList), len(initial)+1)
	}
}
