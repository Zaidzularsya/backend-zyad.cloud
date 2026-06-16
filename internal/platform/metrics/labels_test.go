package metrics

import (
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
)

func TestNewLabelSetAcceptsBoundedLabels(t *testing.T) {
	labels, err := NewLabelSet(map[string]string{
		"organization_type":   "Customer",
		"organization_status": "Active",
		"data_placement":      "Shared",
	})
	if err != nil {
		t.Fatalf("NewLabelSet() error = %v", err)
	}
	if labels["organization_type"] != "customer" ||
		labels["organization_status"] != "active" ||
		labels["data_placement"] != "shared" {
		t.Fatalf("labels = %#v", labels)
	}
}

func TestNewLabelSetRejectsOrganizationID(t *testing.T) {
	_, err := NewLabelSet(map[string]string{
		"organization_id": "11111111-1111-1111-1111-111111111111",
	})
	if !errors.Is(err, ErrHighCardinality) {
		t.Fatalf("NewLabelSet() error = %v", err)
	}
}

func TestNewLabelSetRejectsInvalidLabelValue(t *testing.T) {
	_, err := NewLabelSet(map[string]string{
		"health_status": "healthy ok",
	})
	if !errors.Is(err, ErrInvalidLabel) {
		t.Fatalf("NewLabelSet() error = %v", err)
	}
}

func TestOrganizationHealthLabelsAreBounded(t *testing.T) {
	labels, err := OrganizationHealthLabels(
		coretenant.OrganizationTypeCustomer,
		coretenant.OrganizationStatusActive,
		coretenant.DataPlacementShared,
		"healthy",
	)
	if err != nil {
		t.Fatalf("OrganizationHealthLabels() error = %v", err)
	}
	got := labels.SortedAttrs()
	want := []string{
		"data_placement=shared",
		"health_status=healthy",
		"organization_status=active",
		"organization_type=customer",
	}
	if len(got) != len(want) {
		t.Fatalf("SortedAttrs() = %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SortedAttrs() = %#v, want %#v", got, want)
		}
	}
}

func TestResolutionLabelsAreBounded(t *testing.T) {
	labels, err := ResolutionLabels(coretenant.ResolutionSourceHeader, "success")
	if err != nil {
		t.Fatalf("ResolutionLabels() error = %v", err)
	}
	if labels["resolution_source"] != "header" || labels["result"] != "success" {
		t.Fatalf("labels = %#v", labels)
	}
}
