package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/modules/receivable/service"
)

const (
	setOrgA    = "11111111-1111-1111-1111-111111111111"
	setOrgB    = "22222222-2222-2222-2222-222222222222"
	setPlatOrg = "99999999-9999-9999-9999-999999999999"
)

// fakeSettlementSvc menyimpan entri per org sehingga test bisa membuktikan isolasi.
type fakeSettlementSvc struct {
	service.SettlementService
	entries     map[string][]repository.SettlementEntry
	balanceOrg  string
	entriesOrg  string
	recordedFor string
	recordedBy  string
	recordedIn  service.SettlementEntryInput
	recordErr   error
}

func (f *fakeSettlementSvc) Balance(_ context.Context, org string) (service.SettlementBalance, error) {
	f.balanceOrg = org
	return service.SettlementBalance{PaidIn: "150000.75", Settled: "0.00", Adjusted: "0.00", Remaining: "150000.75"}, nil
}

func (f *fakeSettlementSvc) ListEntries(_ context.Context, org string, _, _ int) ([]repository.SettlementEntry, int64, error) {
	f.entriesOrg = org
	return f.entries[org], int64(len(f.entries[org])), nil
}

func (f *fakeSettlementSvc) ListBalances(context.Context, service.SettlementBalanceFilter) ([]service.OrgBalance, int64, error) {
	return []service.OrgBalance{{OrganizationID: setOrgA, OrganizationName: "Acme", SettlementBalance: service.SettlementBalance{PaidIn: "1.00", Settled: "0.00", Adjusted: "0.00", Remaining: "1.00"}}}, 1, nil
}

func (f *fakeSettlementSvc) Record(_ context.Context, org string, in service.SettlementEntryInput, actor string) (repository.SettlementEntry, error) {
	f.recordedFor, f.recordedIn, f.recordedBy = org, in, actor
	if f.recordErr != nil {
		return repository.SettlementEntry{}, f.recordErr
	}
	return repository.SettlementEntry{ID: "e1", OrganizationID: org, Kind: in.Kind, Amount: in.Amount, Note: in.Note, OccurredAt: time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)}, nil
}

// forbidding: checker global asli mengembalikan AppError 403; grantChecker biasa mengembalikan galat polos.
type forbidding struct{ grantChecker }

func (f forbidding) Can(ctx context.Context, user string, perms []string) error {
	if f.grantChecker.Can(ctx, user, perms) != nil {
		return coreerrors.New("FORBIDDEN", "insufficient permissions", http.StatusForbidden)
	}
	return nil
}

func settlementRouter(t *testing.T, svc *fakeSettlementSvc, orgID string, orgType coretenant.OrganizationType, perms ...string) *gin.Engine {
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
	h := NewSettlementHandler(svc)
	g := r.Group("")
	h.RegisterPlatformRoutes(g, forbidding{grant(perms...)})
	h.RegisterOwnRoutes(g, grant(perms...))
	return r
}

func TestPlatformSettlementsRequirePlatformOrg(t *testing.T) {
	svc := &fakeSettlementSvc{}
	r := settlementRouter(t, svc, setOrgA, coretenant.OrganizationTypeCustomer, "settlement.manage", "settlement.read")
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/platform/settlements", ""},
		{http.MethodGet, "/platform/settlements/" + setOrgA + "/entries", ""},
		{http.MethodPost, "/platform/settlements/" + setOrgA + "/entries", `{"kind":"settlement","amount":"1"}`},
	} {
		if w := call(r, tc.method, tc.path, tc.body); w.Code != http.StatusForbidden {
			t.Fatalf("%s %s = %d, want 403", tc.method, tc.path, w.Code)
		}
	}
	if svc.recordedFor != "" {
		t.Fatal("service dipanggil oleh org non-platform")
	}
}

func TestPlatformSettlementsRequireManagePermission(t *testing.T) {
	r := settlementRouter(t, &fakeSettlementSvc{}, setPlatOrg, coretenant.OrganizationTypePlatform, "settlement.read")
	if w := call(r, http.MethodGet, "/platform/settlements", ""); w.Code != http.StatusForbidden {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestPlatformListAndRecord(t *testing.T) {
	svc := &fakeSettlementSvc{}
	r := settlementRouter(t, svc, setPlatOrg, coretenant.OrganizationTypePlatform, "settlement.manage")
	w := call(r, http.MethodGet, "/platform/settlements?search=ac", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"balance":"1.00"`) || !strings.Contains(w.Body.String(), `"total":1`) {
		t.Fatalf("list = %d %s", w.Code, w.Body)
	}
	w = call(r, http.MethodPost, "/platform/settlements/"+setOrgB+"/entries", `{"kind":"adjustment","amount":"-0.25","note":"koreksi","occurred_at":"2026-10-20T10:00:00+07:00"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("record = %d %s", w.Code, w.Body)
	}
	if svc.recordedFor != setOrgB || svc.recordedBy != "user-1" || svc.recordedIn.Amount != "-0.25" ||
		!svc.recordedIn.OccurredAt.Equal(time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)) {
		t.Fatalf("service input = %q %q %+v", svc.recordedFor, svc.recordedBy, svc.recordedIn)
	}
	// body tidak boleh menentukan org target
	call(r, http.MethodPost, "/platform/settlements/"+setOrgB+"/entries", `{"kind":"settlement","amount":"1","organization_id":"`+setOrgA+`"}`)
	if svc.recordedFor != setOrgB {
		t.Fatalf("org target = %s", svc.recordedFor)
	}
}

func TestPlatformRecordErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{service.ErrSettlementExceedsBalance, 422, "SETTLEMENT_EXCEEDS_BALANCE"},
		{service.ErrAdjustmentBelowZero, 422, "ADJUSTMENT_BELOW_ZERO"},
		{service.ErrAdjustmentNoteRequired, 422, "VALIDATION_ERROR"},
		{service.ErrInvalidSettlementEntry, 422, "VALIDATION_ERROR"},
		{service.ErrSettlementBeneficiaryInvalid, 422, "BENEFICIARY_NOT_CUSTOMER"},
		{service.ErrSettlementBeneficiaryNotFound, 404, "ORGANIZATION_NOT_FOUND"},
	}
	for _, tc := range cases {
		svc := &fakeSettlementSvc{recordErr: tc.err}
		r := settlementRouter(t, svc, setPlatOrg, coretenant.OrganizationTypePlatform, "settlement.manage")
		w := call(r, http.MethodPost, "/platform/settlements/"+setOrgA+"/entries", `{"kind":"settlement","amount":"1"}`)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatalf("%v => %d %s", tc.err, w.Code, w.Body)
		}
	}
	r := settlementRouter(t, &fakeSettlementSvc{}, setPlatOrg, coretenant.OrganizationTypePlatform, "settlement.manage")
	if w := call(r, http.MethodPost, "/platform/settlements/not-a-uuid/entries", `{}`); w.Code != http.StatusNotFound {
		t.Fatalf("bad uuid = %d", w.Code)
	}
	if w := call(r, http.MethodPost, "/platform/settlements/"+setOrgA+"/entries", `{bad`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad json = %d", w.Code)
	}
}

func TestOwnSettlementNeverLeaksOtherOrg(t *testing.T) {
	svc := &fakeSettlementSvc{entries: map[string][]repository.SettlementEntry{
		setOrgA: {{ID: "a1", OrganizationID: setOrgA, Kind: "payment", Amount: "10.00"}},
		setOrgB: {{ID: "b1", OrganizationID: setOrgB, Kind: "payment", Amount: "999.00"}},
	}}
	r := settlementRouter(t, svc, setOrgA, coretenant.OrganizationTypeCustomer, "settlement.read")
	for _, path := range []string{
		"/app/settlement/entries",
		"/app/settlement/entries?organization_id=" + setOrgB,
		"/app/settlement/entries?orgId=" + setOrgB + "&beneficiary_organization_id=" + setOrgB,
	} {
		w := call(r, http.MethodGet, path, "")
		if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "b1") || strings.Contains(w.Body.String(), setOrgB) || !strings.Contains(w.Body.String(), "a1") {
			t.Fatalf("%s = %d %s", path, w.Code, w.Body)
		}
		if svc.entriesOrg != setOrgA {
			t.Fatalf("%s: service org = %s", path, svc.entriesOrg)
		}
	}
	w := call(r, http.MethodGet, "/app/settlement/balance?organization_id="+setOrgB, "")
	var body struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != http.StatusOK || body.Data["balance"] != "150000.75" || svc.balanceOrg != setOrgA {
		t.Fatalf("balance = %d %s (org %s)", w.Code, w.Body, svc.balanceOrg)
	}
	// /app tidak punya rute yang menerima :orgId dan tidak punya rute tulis
	if w := call(r, http.MethodGet, "/app/settlement/"+setOrgB+"/entries", ""); w.Code != http.StatusNotFound {
		t.Fatalf("orgId path = %d", w.Code)
	}
	if w := call(r, http.MethodPost, "/app/settlement/entries", `{"kind":"settlement","amount":"1"}`); w.Code != http.StatusNotFound {
		t.Fatalf("post = %d", w.Code)
	}
}

func TestOwnSettlementRequiresReadPermissionAndCustomerOrg(t *testing.T) {
	r := settlementRouter(t, &fakeSettlementSvc{}, setOrgA, coretenant.OrganizationTypeCustomer)
	if w := call(r, http.MethodGet, "/app/settlement/balance", ""); w.Code != http.StatusForbidden {
		t.Fatalf("tanpa izin = %d", w.Code)
	}
	r = settlementRouter(t, &fakeSettlementSvc{}, setPlatOrg, coretenant.OrganizationTypePlatform, "settlement.read")
	if w := call(r, http.MethodGet, "/app/settlement/balance", ""); w.Code != http.StatusForbidden {
		t.Fatalf("org platform = %d", w.Code)
	}
}
