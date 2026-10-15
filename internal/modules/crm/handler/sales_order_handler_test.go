package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/modules/crm/service"
)

type soGrant map[string]bool

func (g soGrant) check(perms []string) error {
	for _, p := range perms {
		if !g[p] {
			return http.ErrNotSupported
		}
	}
	return nil
}
func (g soGrant) Can(_ context.Context, _ string, perms []string) error { return g.check(perms) }
func (g soGrant) CanOrganization(_ context.Context, _, _ string, perms []string) error {
	return g.check(perms)
}

type soSvcFake struct {
	service.SalesOrderService
	confirmErr error
	delivery   service.DeliveryInput
}

func (f *soSvcFake) Confirm(context.Context, coretenant.Scope, string, string) (domain.SalesOrder, error) {
	return domain.SalesOrder{ID: "so1", Status: domain.SalesOrderConfirmed}, f.confirmErr
}
func (f *soSvcFake) ConfirmDelivery(_ context.Context, _ coretenant.Scope, _ string, in service.DeliveryInput, _ string) (domain.SalesOrder, error) {
	f.delivery = in
	if in.BatchKey == "" {
		return domain.SalesOrder{}, service.ErrBatchKeyRequired
	}
	return domain.SalesOrder{ID: "so1"}, nil
}

func newSORouter(t *testing.T, svc service.SalesOrderService, perms ...string) *gin.Engine {
	t.Helper()
	r := newTestRouter(t)
	r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "u1"); c.Next() })
	g := soGrant{}
	for _, p := range perms {
		g[p] = true
	}
	NewSalesOrderHandler(svc, SalesOrderRefs{}).RegisterRoutes(r.Group("/app/crm"), g)
	return r
}

func TestSalesOrderConfirmRequiresPermission(t *testing.T) {
	r := newSORouter(t, &soSvcFake{}, "sales_order.read")
	if w := performJSON(r, http.MethodPost, "/app/crm/sales-orders/so1/confirm", ""); w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestSalesOrderConfirmIncompleteReturnsFields(t *testing.T) {
	r := newSORouter(t, &soSvcFake{confirmErr: &service.IncompleteError{Fields: []string{"start_date"}}}, "sales_order.confirm")
	w := performJSON(r, http.MethodPost, "/app/crm/sales-orders/so1/confirm", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Code string `json:"code"`
		Data struct {
			Fields []string `json:"fields"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Code != "SALES_ORDER_INCOMPLETE" || len(body.Data.Fields) != 1 || body.Data.Fields[0] != "start_date" {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestSalesOrderNotDraftIs409(t *testing.T) {
	r := newSORouter(t, &soSvcFake{confirmErr: repository.ErrSalesOrderNotDraft}, "sales_order.confirm")
	w := performJSON(r, http.MethodPost, "/app/crm/sales-orders/so1/confirm", "")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "SALES_ORDER_NOT_DRAFT") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestSalesOrderDeliveriesRequireBatchKey(t *testing.T) {
	svc := &soSvcFake{}
	r := newSORouter(t, svc, "sales_order.confirm")
	w := performJSON(r, http.MethodPost, "/app/crm/sales-orders/so1/deliveries", `{"item_ids":["a"]}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	w = performJSON(r, http.MethodPost, "/app/crm/sales-orders/so1/deliveries", `{"item_ids":["a"],"batch_key":"b1","delivered_at":"2026-10-05"}`)
	if w.Code != http.StatusOK || svc.delivery.BatchKey != "b1" || svc.delivery.DeliveredAt.Format("2006-01-02") != "2026-10-05" {
		t.Fatalf("status=%d delivery=%+v", w.Code, svc.delivery)
	}
	_ = httptest.NewRecorder
}
