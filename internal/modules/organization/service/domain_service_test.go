package service

import (
	"context"
	"errors"
	"testing"
	"time"

	coreauth "zyad.cloud/internal/core/auth"
	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

type domainStoreStub struct {
	domain             model.OrganizationDomain
	createParams       repository.CreateDomainParams
	verificationParams repository.UpdateDomainVerificationParams
	activated          bool
	primary            bool
	actorUserID        string
}

func (s *domainStoreStub) Create(
	_ context.Context,
	params repository.CreateDomainParams,
) (model.OrganizationDomain, error) {
	s.createParams = params
	s.domain = model.OrganizationDomain{
		ID:                        "22222222-2222-2222-2222-222222222222",
		OrganizationID:            params.OrganizationID,
		Type:                      params.Type,
		CanonicalHost:             params.CanonicalHost,
		Status:                    model.DomainStatusPending,
		VerificationChallengeHash: params.VerificationChallengeHash,
		SSLStatus:                 params.SSLStatus,
		CreatedAt:                 time.Now(),
		UpdatedAt:                 time.Now(),
	}
	return s.domain, nil
}

func (s *domainStoreStub) FindByID(
	context.Context,
	string,
	string,
) (model.OrganizationDomain, error) {
	return s.domain, nil
}

func (s *domainStoreStub) ListByOrganization(
	context.Context,
	string,
	repository.DomainListFilter,
) ([]model.OrganizationDomain, int64, error) {
	return []model.OrganizationDomain{s.domain}, 1, nil
}

func (s *domainStoreStub) UpdateVerification(
	_ context.Context,
	_ string,
	_ string,
	params repository.UpdateDomainVerificationParams,
) (model.OrganizationDomain, error) {
	s.verificationParams = params
	s.domain.Status = params.Status
	s.domain.VerificationError = params.VerificationError
	s.domain.VerifiedAt = params.VerifiedAt
	s.domain.VerificationAttempts++
	return s.domain, nil
}

func (s *domainStoreStub) Activate(
	_ context.Context,
	_ string,
	_ string,
	isPrimary bool,
	_ time.Time,
	actorUserID string,
) (model.OrganizationDomain, error) {
	s.activated = true
	s.actorUserID = actorUserID
	s.domain.Status = model.DomainStatusActive
	s.domain.IsPrimary = isPrimary
	return s.domain, nil
}

func (s *domainStoreStub) SetPrimary(
	context.Context,
	string,
	string,
	time.Time,
	string,
) (model.OrganizationDomain, error) {
	s.primary = true
	s.domain.IsPrimary = true
	return s.domain, nil
}

func (s *domainStoreStub) Delete(context.Context, string, string, time.Time, string) error {
	return nil
}

type domainVerifierStub struct {
	err          error
	recordName   string
	expectedHash string
}

func (v *domainVerifierStub) VerifyTXT(
	_ context.Context,
	recordName string,
	expectedHash string,
) error {
	v.recordName = recordName
	v.expectedHash = expectedHash
	return v.err
}

func TestDomainServiceCreateStoresOnlyChallengeHash(t *testing.T) {
	store := &domainStoreStub{}
	service := NewDomainService(store, nil, "example.test")
	service.generateToken = func(int) (string, error) {
		return "plain-domain-token", nil
	}

	challenge, err := service.Create(
		context.Background(),
		"11111111-1111-1111-1111-111111111111",
		dto.CreateDomainRequest{
			Type:          "custom",
			CanonicalHost: " WWW.Example.COM. ",
		},
		"33333333-3333-3333-3333-333333333333",
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if challenge.RecordValue != "plain-domain-token" ||
		challenge.RecordName != "_zyad-verification.www.example.com" {
		t.Fatalf("Create() challenge = %#v", challenge)
	}
	if store.createParams.VerificationChallengeHash !=
		coreauth.HashToken("plain-domain-token") ||
		store.createParams.VerificationChallengeHash == challenge.RecordValue ||
		store.createParams.ActorUserID != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("Create() params = %#v", store.createParams)
	}
}

func TestDomainServiceVerifyFailsClosed(t *testing.T) {
	store := &domainStoreStub{domain: model.OrganizationDomain{
		ID:                        "22222222-2222-2222-2222-222222222222",
		OrganizationID:            "11111111-1111-1111-1111-111111111111",
		CanonicalHost:             "www.example.com",
		Status:                    model.DomainStatusPending,
		VerificationChallengeHash: coreauth.HashToken("expected"),
		CreatedAt:                 time.Now(),
		UpdatedAt:                 time.Now(),
	}}
	verifier := &domainVerifierStub{err: ErrDomainChallengeNotFound}
	service := NewDomainService(store, verifier, "example.test")

	_, err := service.Verify(
		context.Background(),
		store.domain.OrganizationID,
		store.domain.ID,
		"33333333-3333-3333-3333-333333333333",
	)
	if err == nil {
		t.Fatal("Verify() error = nil, want verification failure")
	}
	if store.verificationParams.Status != model.DomainStatusFailed ||
		store.verificationParams.ActorUserID != "33333333-3333-3333-3333-333333333333" ||
		store.activated {
		t.Fatalf(
			"Verify() verification=%#v activated=%v",
			store.verificationParams,
			store.activated,
		)
	}
}

func TestDomainServiceVerifyActivatesMatchingChallenge(t *testing.T) {
	store := &domainStoreStub{domain: model.OrganizationDomain{
		ID:                        "22222222-2222-2222-2222-222222222222",
		OrganizationID:            "11111111-1111-1111-1111-111111111111",
		CanonicalHost:             "www.example.com",
		Status:                    model.DomainStatusPending,
		VerificationChallengeHash: coreauth.HashToken("expected"),
		SSLStatus:                 model.DomainSSLStatusPending,
		CreatedAt:                 time.Now(),
		UpdatedAt:                 time.Now(),
	}}
	verifier := &domainVerifierStub{}
	service := NewDomainService(store, verifier, "example.test")

	domain, err := service.Verify(
		context.Background(),
		store.domain.OrganizationID,
		store.domain.ID,
		"33333333-3333-3333-3333-333333333333",
	)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if domain.Status != "active" ||
		store.verificationParams.Status != model.DomainStatusVerified ||
		!store.activated ||
		store.actorUserID != "33333333-3333-3333-3333-333333333333" ||
		verifier.recordName != "_zyad-verification.www.example.com" {
		t.Fatalf(
			"Verify() domain=%#v verification=%#v verifier=%#v",
			domain,
			store.verificationParams,
			verifier,
		)
	}
}

func TestDomainServiceVerifyDoesNotPersistResolverFailure(t *testing.T) {
	store := &domainStoreStub{domain: model.OrganizationDomain{
		ID:                        "22222222-2222-2222-2222-222222222222",
		OrganizationID:            "11111111-1111-1111-1111-111111111111",
		CanonicalHost:             "www.example.com",
		Status:                    model.DomainStatusPending,
		VerificationChallengeHash: coreauth.HashToken("expected"),
		CreatedAt:                 time.Now(),
		UpdatedAt:                 time.Now(),
	}}
	service := NewDomainService(
		store,
		&domainVerifierStub{err: errors.New("temporary resolver failure")},
		"example.test",
	)

	_, err := service.Verify(
		context.Background(),
		store.domain.OrganizationID,
		store.domain.ID,
		"33333333-3333-3333-3333-333333333333",
	)
	if err == nil {
		t.Fatal("Verify() error = nil, want resolver unavailable")
	}
	if store.verificationParams.Status != "" || store.activated {
		t.Fatalf(
			"Verify() verification=%#v activated=%v",
			store.verificationParams,
			store.activated,
		)
	}
}

func TestDomainServiceRejectsInvalidHostAndPrematurePrimary(t *testing.T) {
	service := NewDomainService(&domainStoreStub{}, nil, "example.test")
	_, err := service.Create(
		context.Background(),
		"11111111-1111-1111-1111-111111111111",
		dto.CreateDomainRequest{
			Type:          "custom",
			CanonicalHost: "https://example.com/path",
		},
		"",
	)
	if err == nil {
		t.Fatal("Create() error = nil, want invalid host")
	}
	_, err = service.Create(
		context.Background(),
		"11111111-1111-1111-1111-111111111111",
		dto.CreateDomainRequest{
			Type:          "custom",
			CanonicalHost: "www.example.com",
			IsPrimary:     true,
		},
		"",
	)
	if err == nil {
		t.Fatal("Create() error = nil, want premature primary rejection")
	}
}
