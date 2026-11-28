package service

import (
	"context"
	"reflect"
	"testing"
)

type fakeCheckoutRelations struct{ rows []CustomerRelation }

func (f fakeCheckoutRelations) CheckoutRelations(context.Context, string) ([]CustomerRelation, error) {
	return f.rows, nil
}

type fakeClaimedLinks struct{ rows []ClaimedLink }

func (f fakeClaimedLinks) ClaimedLinks(context.Context, string) ([]ClaimedLink, error) {
	return f.rows, nil
}

type fakeContactStatus struct{ deleted map[string]bool }

func (f fakeContactStatus) ContactActive(_ context.Context, _ string, contactID string) (bool, error) {
	return !f.deleted[contactID], nil
}

func TestForUserMergesCheckoutAndClaim(t *testing.T) {
	svc := NewRelationsService(
		fakeCheckoutRelations{rows: []CustomerRelation{
			{OrganizationID: "org-platform", OrganizationName: "Zyad", Source: RelationSourceCheckout},
			{OrganizationID: "org-isp", OrganizationName: "ISP Satu", Source: RelationSourceCheckout},
		}},
		fakeClaimedLinks{rows: []ClaimedLink{
			{ID: "link-1", OrganizationID: "org-isp", OrganizationName: "ISP Satu", ContactID: "c-1", Status: "active"},
		}},
		fakeContactStatus{},
	)

	got, err := svc.ForUser(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ForUser: %v", err)
	}
	want := []CustomerRelation{
		{OrganizationID: "org-platform", OrganizationName: "Zyad", Source: "checkout"},
		{OrganizationID: "org-isp", OrganizationName: "ISP Satu", Source: "checkout"},
		{OrganizationID: "org-isp", OrganizationName: "ISP Satu", Source: "claim", CustomerLinkID: "link-1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ForUser = %+v, want %+v", got, want)
	}
}

func TestForUserSkipsDeletedContact(t *testing.T) {
	svc := NewRelationsService(
		fakeCheckoutRelations{},
		fakeClaimedLinks{rows: []ClaimedLink{
			{ID: "link-1", OrganizationID: "org-a", OrganizationName: "A", ContactID: "c-deleted", Status: "active"},
			{ID: "link-2", OrganizationID: "org-b", OrganizationName: "B", ContactID: "c-live", Status: "active"},
		}},
		fakeContactStatus{deleted: map[string]bool{"c-deleted": true}},
	)

	got, err := svc.ForUser(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ForUser: %v", err)
	}
	if len(got) != 1 || got[0].CustomerLinkID != "link-2" {
		t.Fatalf("ForUser = %+v, want only link-2", got)
	}
}

func TestForUserSkipsRevokedLink(t *testing.T) {
	svc := NewRelationsService(
		fakeCheckoutRelations{},
		fakeClaimedLinks{rows: []ClaimedLink{
			{ID: "link-revoked", OrganizationID: "org-a", ContactID: "c-1", Status: "revoked"},
			{ID: "link-pending", OrganizationID: "org-b", ContactID: "c-2", Status: "pending_user"},
			{ID: "link-declined", OrganizationID: "org-c", ContactID: "c-3", Status: "declined"},
		}},
		fakeContactStatus{},
	)

	got, err := svc.ForUser(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ForUser: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ForUser = %+v, want none", got)
	}
}
