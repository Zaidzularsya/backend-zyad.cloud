package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/modules/receivable/service"
)

const payoutID = "33333333-3333-3333-3333-333333333333"

type fakePayoutSvc struct {
	service.PayoutAccountService
	submitOrg, cancelOrg, verifyID, assignOrg string
	submitIn                                  service.PayoutAccountInput
	submitAudit                               repository.PayoutAudit
	assignNote                                string
	err                                       error
}

func (f *fakePayoutSvc) Submit(_ context.Context, org string, in service.PayoutAccountInput, a repository.PayoutAudit) (repository.PayoutAccount, error) {
	f.submitOrg, f.submitIn, f.submitAudit = org, in, a
	return repository.PayoutAccount{ID: payoutID, OrganizationID: org, Status: "pending", AccountNumber: "1234567890", SubmittedAt: time.Now().UTC()}, f.err
}

func (f *fakePayoutSvc) Cancel(_ context.Context, org, id string, _ repository.PayoutAudit) (repository.PayoutAccount, error) {
	f.cancelOrg = org
	return repository.PayoutAccount{ID: id, OrganizationID: org, Status: "cancelled"}, f.err
}

func (f *fakePayoutSvc) Verify(_ context.Context, id, _ string, _ repository.PayoutAudit) (repository.PayoutAccount, error) {
	f.verifyID = id
	return repository.PayoutAccount{ID: id, Status: "verified"}, f.err
}

func (f *fakePayoutSvc) Assign(_ context.Context, org string, _ service.PayoutAccountInput, note string, _ repository.PayoutAudit) (repository.PayoutAccount, error) {
	f.assignOrg, f.assignNote = org, note
	return repository.PayoutAccount{ID: payoutID, OrganizationID: org, Status: "verified"}, f.err
}

func (f *fakePayoutSvc) Overview(context.Context, string) (service.PayoutOverview, error) {
	return service.PayoutOverview{}, f.err
}

func payoutRouter(t *testing.T, svc *fakePayoutSvc, orgID string, orgType coretenant.OrganizationType, perms ...string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: orgID, OrganizationSlug: "x", OrganizationType: orgType,
		OrganizationStatus: coretenant.OrganizationStatusActive, MembershipID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", MembershipStatus: "active",
		MembershipVersion: 1, ResolutionSource: coretenant.ResolutionSourceSession, DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tc))
		permissionmiddleware.SetUserID(c, "user-1")
		c.Next()
	})
	g := r.Group("")
	// Handler saldo ikut didaftarkan agar konflik rute (/:orgId/entries vs /payout-accounts) ketahuan.
	sh := NewSettlementHandler(&fakeSettlementSvc{})
	sh.RegisterPlatformRoutes(g, forbidding{grant(perms...)})
	sh.RegisterOwnRoutes(g, grant(perms...))
	h := NewPayoutAccountHandler(svc)
	h.RegisterPlatformRoutes(g, forbidding{grant(perms...)})
	h.RegisterOwnRoutes(g, grant(perms...))
	return r
}

func TestOwnSubmitUsesTenantOrgOnly(t *testing.T) {
	svc := &fakePayoutSvc{}
	r := payoutRouter(t, svc, setOrgA, coretenant.OrganizationTypeCustomer, "settlement.read", "payout_account.manage")
	w := call(r, http.MethodPost, "/app/settlement/payout-accounts",
		`{"organization_id":"`+setOrgB+`","bank_code":"BCA","account_number":"1234567890","account_holder_name":"Budi"}`)
	if w.Code != http.StatusCreated || svc.submitOrg != setOrgA || svc.submitIn.BankCode != "BCA" || svc.submitAudit.ActorUserID != "user-1" {
		t.Fatalf("submit = %d %s org=%s audit=%+v", w.Code, w.Body, svc.submitOrg, svc.submitAudit)
	}
	if !strings.Contains(w.Body.String(), `"account_number":"1234567890"`) {
		t.Fatalf("nomor harus utuh: %s", w.Body)
	}
	if w := call(r, http.MethodPost, "/app/settlement/payout-accounts/"+payoutID+"/cancel", ""); w.Code != http.StatusOK || svc.cancelOrg != setOrgA {
		t.Fatalf("cancel = %d org=%s", w.Code, svc.cancelOrg)
	}
	if w := call(r, http.MethodPost, "/app/settlement/payout-accounts/bad/cancel", ""); w.Code != http.StatusNotFound {
		t.Fatalf("bad id = %d", w.Code)
	}
}

func TestOwnSubmitForbiddenWithoutManagePermission(t *testing.T) {
	svc := &fakePayoutSvc{}
	r := payoutRouter(t, svc, setOrgA, coretenant.OrganizationTypeCustomer, "settlement.read")
	for _, p := range []string{"/app/settlement/payout-accounts", "/app/settlement/payout-accounts/" + payoutID + "/cancel"} {
		if w := call(r, http.MethodPost, p, `{}`); w.Code != http.StatusForbidden {
			t.Fatalf("%s = %d", p, w.Code)
		}
	}
	if svc.submitOrg != "" || svc.cancelOrg != "" {
		t.Fatal("service dipanggil tanpa izin")
	}
	if w := call(r, http.MethodGet, "/app/settlement/payout-accounts", ""); w.Code != http.StatusOK {
		t.Fatalf("overview = %d", w.Code)
	}
	if w := call(r, http.MethodGet, "/app/settlement/banks", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"BCA"`) {
		t.Fatalf("banks = %d %s", w.Code, w.Body)
	}
}

func TestPlatformVerifyRequiresSettlementManage(t *testing.T) {
	svc := &fakePayoutSvc{}
	r := payoutRouter(t, svc, setPlatOrg, coretenant.OrganizationTypePlatform, "settlement.read")
	for _, tc := range []struct{ m, p string }{
		{http.MethodPost, "/platform/settlements/payout-accounts/" + payoutID + "/verify"},
		{http.MethodPost, "/platform/settlements/payout-accounts/" + payoutID + "/reject"},
		{http.MethodGet, "/platform/settlements/payout-accounts"},
		{http.MethodGet, "/platform/settlements/banks"},
		{http.MethodPost, "/platform/settlements/" + setOrgA + "/payout-accounts"},
	} {
		if w := call(r, tc.m, tc.p, `{}`); w.Code != http.StatusForbidden {
			t.Fatalf("%s %s = %d", tc.m, tc.p, w.Code)
		}
	}
	if svc.verifyID != "" {
		t.Fatal("verify dipanggil tanpa izin")
	}
	r = payoutRouter(t, svc, setPlatOrg, coretenant.OrganizationTypePlatform, "settlement.manage")
	if w := call(r, http.MethodPost, "/platform/settlements/payout-accounts/"+payoutID+"/verify", `{}`); w.Code != http.StatusOK || svc.verifyID != payoutID {
		t.Fatalf("verify = %d %s", w.Code, w.Body)
	}
	w := call(r, http.MethodPost, "/platform/settlements/"+setOrgB+"/payout-accounts", `{"bank_code":"BCA","account_number":"12345","account_holder_name":"Budi","note":"via telepon"}`)
	if w.Code != http.StatusCreated || svc.assignOrg != setOrgB || svc.assignNote != "via telepon" {
		t.Fatalf("assign = %d %s", w.Code, w.Body)
	}
	if w := call(r, http.MethodGet, "/platform/settlements/banks", ""); w.Code != http.StatusOK {
		t.Fatalf("banks = %d", w.Code)
	}
	if w := call(r, http.MethodPost, "/platform/settlements/payout-accounts/bad/verify", `{}`); w.Code != http.StatusNotFound {
		t.Fatalf("bad id = %d", w.Code)
	}
	if w := call(r, http.MethodPost, "/platform/settlements/not-uuid/payout-accounts", `{}`); w.Code != http.StatusNotFound {
		t.Fatalf("bad org = %d", w.Code)
	}
}

func TestPayoutErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{repository.ErrPayoutAccountPendingExists, 409, "PAYOUT_ACCOUNT_PENDING_EXISTS"},
		{repository.ErrPayoutAccountNotFound, 404, "PAYOUT_ACCOUNT_NOT_FOUND"},
		{repository.ErrPayoutAccountInvalidState, 409, "PAYOUT_ACCOUNT_INVALID_STATE"},
		{service.ErrPayoutNoteRequired, 422, "VALIDATION_ERROR"},
		{service.ErrInvalidPayoutAccount, 422, "VALIDATION_ERROR"},
		{service.ErrSettlementBeneficiaryNotFound, 404, "ORGANIZATION_NOT_FOUND"},
		{service.ErrSettlementBeneficiaryInvalid, 422, "BENEFICIARY_NOT_CUSTOMER"},
	}
	for _, tc := range cases {
		svc := &fakePayoutSvc{err: tc.err}
		r := payoutRouter(t, svc, setPlatOrg, coretenant.OrganizationTypePlatform, "settlement.manage")
		w := call(r, http.MethodPost, "/platform/settlements/payout-accounts/"+payoutID+"/verify", `{}`)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatalf("%v => %d %s", tc.err, w.Code, w.Body)
		}
	}
}

func TestRecordWithoutVerifiedPayoutReturns422(t *testing.T) {
	svc := &fakeSettlementSvc{recordErr: service.ErrPayoutAccountNotVerified}
	r := settlementRouter(t, svc, setPlatOrg, coretenant.OrganizationTypePlatform, "settlement.manage")
	w := call(r, http.MethodPost, "/platform/settlements/"+setOrgA+"/entries", `{"kind":"settlement","amount":"1"}`)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "PAYOUT_ACCOUNT_NOT_VERIFIED") {
		t.Fatalf("record = %d %s", w.Code, w.Body)
	}
}

type fakeSummary struct{}

func (fakeSummary) Summary(_ context.Context, ids []string) (map[string]repository.PayoutSummary, error) {
	return map[string]repository.PayoutSummary{setOrgA: {Verified: &repository.PayoutAccount{ID: payoutID, BankName: "BCA", AccountNumber: "1234567890", Status: "verified"}, HasPending: true}}, nil
}

func TestListBalancesIncludesPayoutSummary(t *testing.T) {
	svc := &fakeSettlementSvc{}
	h := NewSettlementHandler(svc).WithPayoutSummary(fakeSummary{})
	r := settlementRouterWith(t, h, setPlatOrg, coretenant.OrganizationTypePlatform, "settlement.manage")
	w := call(r, http.MethodGet, "/platform/settlements", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"has_pending_payout_account":true`) || !strings.Contains(w.Body.String(), `"account_number":"1234567890"`) {
		t.Fatalf("list = %d %s", w.Code, w.Body)
	}
	// tanpa summary: aditif, payout_account null
	r = settlementRouter(t, svc, setPlatOrg, coretenant.OrganizationTypePlatform, "settlement.manage")
	w = call(r, http.MethodGet, "/platform/settlements", "")
	if !strings.Contains(w.Body.String(), `"payout_account":null`) || !strings.Contains(w.Body.String(), `"has_pending_payout_account":false`) {
		t.Fatalf("default = %s", w.Body)
	}
}

func TestEntriesExposeOnlyMaskedPayoutAccount(t *testing.T) {
	svc := &fakeSettlementSvc{entries: map[string][]repository.SettlementEntry{setOrgA: {
		{ID: "e1", OrganizationID: setOrgA, Kind: "settlement", PayoutAccountID: payoutID, PayoutBankName: "BCA", PayoutAccountNumber: "1234567890"},
		{ID: "e2", OrganizationID: setOrgA, Kind: "payment"},
	}}}
	r := settlementRouter(t, svc, setOrgA, coretenant.OrganizationTypeCustomer, "settlement.read")
	w := call(r, http.MethodGet, "/app/settlement/entries", "")
	body := w.Body.String()
	if strings.Contains(body, "1234567890") || !strings.Contains(body, `"account_number_masked":"••••7890"`) || !strings.Contains(body, `"payout_account":null`) {
		t.Fatalf("entries = %s", body)
	}
}
