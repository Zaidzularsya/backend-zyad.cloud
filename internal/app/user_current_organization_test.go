package app

import (
	"context"
	"errors"
	"testing"

	organizationdto "zyad.cloud/internal/modules/organization/dto"
)

type fakeOrganizationLister struct {
	items []organizationdto.UserOrganizationResponse
	err   error
}

func (f fakeOrganizationLister) List(context.Context, string, string) ([]organizationdto.UserOrganizationResponse, error) {
	return f.items, f.err
}

func TestUserCurrentOrganizationProvider(t *testing.T) {
	other := organizationdto.UserOrganizationResponse{}
	current := organizationdto.UserOrganizationResponse{IsCurrent: true}

	got, err := userCurrentOrganizationProvider{fakeOrganizationLister{items: []organizationdto.UserOrganizationResponse{other, current}}}.
		CurrentOrganization(context.Background(), "u", "s")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if v, ok := got.(organizationdto.UserOrganizationResponse); !ok || !v.IsCurrent {
		t.Fatalf("got %#v, want the IsCurrent element", got)
	}

	got, err = userCurrentOrganizationProvider{fakeOrganizationLister{items: []organizationdto.UserOrganizationResponse{other}}}.
		CurrentOrganization(context.Background(), "u", "s")
	if err != nil || got != nil {
		t.Fatalf("got %#v err %v, want untyped nil", got, err)
	}

	_, err = userCurrentOrganizationProvider{fakeOrganizationLister{err: errors.New("boom")}}.
		CurrentOrganization(context.Background(), "u", "s")
	if err == nil {
		t.Fatal("expected error")
	}
}
