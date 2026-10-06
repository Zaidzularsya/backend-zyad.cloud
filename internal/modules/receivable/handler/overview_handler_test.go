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
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/shared/pricing"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

type fakeOverview struct{}

func (fakeOverview) Get(context.Context, coretenant.Scope) (service.Overview, error) {
	return service.Overview{
		ActiveContracts: 2,
		Recurring:       []service.RecurringTotal{{Frequency: pricing.Monthly, Label: "Bulanan", Amount: "333000.00"}},
		Upcoming: []service.UpcomingBilling{{ContractID: ctrID, ContractNumber: "CTR-1", AccountName: "Budi", BillOn: d("2026-10-29"),
			PeriodStart: d("2026-11-05"), PeriodEnd: d("2026-12-04"), Amount: "333000.00", PaymentTiming: pricing.Prepaid}},
		Unpaid:      repository.UnpaidSummary{Count: 3, OverdueCount: 1, TotalBalance: "999000.00"},
		FailedSends: []repository.FailedSend{{InvoiceID: invID, InvoiceNumber: "INV-1", AccountName: "Budi", Channel: "email", Error: "smtp down", SentAt: d("2026-10-05")}},
	}, nil
}

func (fakeOverview) ContractUpcoming(context.Context, coretenant.Scope, domain.Contract, int) ([]service.UpcomingBilling, error) {
	return []service.UpcomingBilling{{ContractID: ctrID, ItemID: "A", Description: "Internet", BillOn: d("2026-10-29"),
		PeriodStart: d("2026-11-05"), PeriodEnd: d("2026-12-04"), Amount: "333000.00"}}, nil
}

type fakeContractSvc struct{ service.ContractService }

func (fakeContractSvc) Get(context.Context, coretenant.Scope, string) (domain.Contract, error) {
	return domain.Contract{ID: ctrID, ContractNumber: "CTR-1", Status: domain.ContractActive, StartDate: d("2026-10-05")}, nil
}

const ctrID = "cccccccc-cccc-cccc-cccc-cccccccccccc"

func overviewRouter(t *testing.T, perms ...string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: "11111111-1111-1111-1111-111111111111", OrganizationSlug: "a", OrganizationType: coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive, MembershipID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", MembershipStatus: "active",
		MembershipVersion: 1, ResolutionSource: coretenant.ResolutionSourceSession, DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tc))
		permissionmiddleware.SetUserID(c, "u1")
		c.Next()
	})
	group := r.Group("/app/receivable")
	NewOverviewHandler(fakeOverview{}).RegisterRoutes(group, grant(perms...))
	NewContractHandler(fakeContractSvc{}).WithUpcoming(fakeOverview{}).RegisterRoutes(group, grant(perms...))
	return r
}

func TestOverviewRequiresContractRead(t *testing.T) {
	r := overviewRouter(t)
	if w := call(r, http.MethodGet, base("/overview"), ""); w.Code != http.StatusForbidden {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestOverviewPayload(t *testing.T) {
	w := call(overviewRouter(t, "contract.read"), http.MethodGet, base("/overview"), "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body)
	}
	for _, want := range []string{
		`"active_contracts":2`,
		`"recurring_by_frequency":[{"frequency":"monthly","label":"Bulanan","amount":"333000.00"}]`,
		`"bill_on":"2026-10-29"`, `"period_start":"2026-11-05"`, `"period_end":"2026-12-04"`, `"payment_timing":"prepaid"`,
		`"contract_number":"CTR-1"`, `"account_name":"Budi"`,
		`"unpaid":{"count":3,"total_balance":"999000.00","overdue_count":1}`,
		`"failed_sends":[{"invoice_id":"` + invID + `","invoice_number":"INV-1","account_name":"Budi","channel":"email","error":"smtp down"`,
	} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("response missing %s\n%s", want, w.Body)
		}
	}
}

func TestContractDetailIncludesUpcoming(t *testing.T) {
	w := call(overviewRouter(t, "contract.read"), http.MethodGet, base("/contracts/"+ctrID), "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"item_id":"A","description":"Internet","bill_on":"2026-10-29"`) {
		t.Fatalf("detail = %d %s", w.Code, w.Body)
	}
}
