package app

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/businesstime"
	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	crmdomain "zyad.cloud/internal/modules/crm/domain"
	crmrepo "zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/modules/mikrotik"
	mikrotikservice "zyad.cloud/internal/modules/mikrotik/service"
	"zyad.cloud/internal/modules/receivable"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/shared/pricing"
)

// The mikrotik module must not import crm/receivable/catalog; these adapters
// are the only place where the modules meet. Every call passes the caller's
// scope through, so the lookups stay organization-scoped by the owning module.

const (
	mikrotikImportContactSource = "mikrotik_import"
	mikrotikImportContactTag    = "pppoe"
	mikrotikAccountSearchMax    = 200
)

// ---- AccountDirectory ----

type mikrotikAccountSource interface {
	Get(ctx context.Context, scope coretenant.Scope, id string) (receivabledomain.Account, error)
	List(ctx context.Context, scope coretenant.Scope, search string, limit, offset int) ([]receivabledomain.Account, int64, error)
	EnsureForSource(ctx context.Context, scope coretenant.Scope, in receivableservice.AccountInput, userID string) (receivabledomain.Account, error)
}

func accountRef(a receivabledomain.Account) mikrotikservice.AccountRef {
	return mikrotikservice.AccountRef{ID: a.ID, Name: a.Name, Phone: a.Phone, Email: a.Email}
}

type mikrotikAccountDirectory struct{ accounts mikrotikAccountSource }

func (d mikrotikAccountDirectory) Get(ctx context.Context, scope coretenant.Scope, id string) (mikrotikservice.AccountRef, error) {
	acc, err := d.accounts.Get(ctx, scope, id)
	if err != nil {
		if errors.Is(err, receivable.ErrAccountNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return mikrotikservice.AccountRef{}, mikrotik.ErrAccountNotFound
		}
		return mikrotikservice.AccountRef{}, err
	}
	return accountRef(acc), nil
}

func (d mikrotikAccountDirectory) Search(ctx context.Context, scope coretenant.Scope, q string, limit int) ([]mikrotikservice.AccountRef, error) {
	if limit <= 0 || limit > mikrotikAccountSearchMax {
		limit = mikrotikAccountSearchMax
	}
	list, _, err := d.accounts.List(ctx, scope, q, limit, 0)
	if err != nil {
		return nil, err
	}
	out := make([]mikrotikservice.AccountRef, 0, len(list))
	for _, a := range list {
		out = append(out, accountRef(a))
	}
	return out, nil
}

// ---- ContactProvisioner ----

type mikrotikContactCreator interface {
	Create(ctx context.Context, scope coretenant.Scope, params crmrepo.CreateContactParams) (crmdomain.Contact, error)
}

// mikrotikContactProvisioner creates a CRM customer contact (through the CRM
// service, so the crm.max_contacts quota applies) and its receivable account.
// A quota refusal is returned as the CRM *AppError (409 QUOTA_EXCEEDED), which
// the mikrotik handler passes through. The two writes are separate
// transactions; EnsureForSource is idempotent per contact, but a failure
// between them leaves an unlinked CRM contact.
type mikrotikContactProvisioner struct {
	contacts mikrotikContactCreator
	accounts mikrotikAccountSource
}

func (p mikrotikContactProvisioner) CreateCustomerFromSecret(ctx context.Context, scope coretenant.Scope, name, _ /* comment: crm_contacts has no notes column */, actorUserID string) (mikrotikservice.AccountRef, error) {
	name = strings.TrimSpace(name)
	contact, err := p.contacts.Create(ctx, scope, crmrepo.CreateContactParams{
		FirstName:      name,
		Source:         mikrotikImportContactSource,
		IsCustomer:     true,
		LifecycleStage: crmdomain.ContactLifecycleCustomer,
		Tags:           []string{mikrotikImportContactTag},
		CreatedBy:      actorUserID,
	})
	if err != nil {
		return mikrotikservice.AccountRef{}, err
	}
	acc, err := p.accounts.EnsureForSource(ctx, scope, receivableservice.AccountInput{
		Name: name, SourceType: receivabledomain.SourceTypeCRMContact, SourceID: contact.ID,
	}, actorUserID)
	if err != nil {
		return mikrotikservice.AccountRef{}, err
	}
	return accountRef(acc), nil
}

// ---- ProductCatalog ----

type mikrotikProductSource interface {
	Get(ctx context.Context, scope coretenant.Scope, id string) (catalogdomain.Product, error)
	FindByIDs(ctx context.Context, scope coretenant.Scope, ids []string) (map[string]catalogdomain.Product, error)
}

// mikrotikContractProductIDs lists product ids referenced by active contracts.
type mikrotikContractProductIDs interface {
	ActiveContractProductIDs(ctx context.Context, scope coretenant.Scope) ([]string, error)
}

type mikrotikProductCatalog struct {
	products mikrotikProductSource
	inUse    mikrotikContractProductIDs
}

func productInfo(p catalogdomain.Product) mikrotikservice.ProductInfo {
	return mikrotikservice.ProductInfo{
		ID: p.ID, Name: p.Name, SKU: p.SKU,
		Recurring: p.Pricing.ChargeType == pricing.Recurring, Active: p.IsActive,
	}
}

func (c mikrotikProductCatalog) Get(ctx context.Context, scope coretenant.Scope, id string) (mikrotikservice.ProductInfo, error) {
	p, err := c.products.Get(ctx, scope, id)
	if err != nil {
		var appErr *coreerrors.AppError
		if errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &appErr) && appErr.Status == http.StatusNotFound) {
			return mikrotikservice.ProductInfo{}, mikrotik.ErrProductNotFound
		}
		return mikrotikservice.ProductInfo{}, err
	}
	return productInfo(p), nil
}

func (c mikrotikProductCatalog) ListRecurringInUse(ctx context.Context, scope coretenant.Scope) ([]mikrotikservice.ProductInfo, error) {
	ids, err := c.inUse.ActiveContractProductIDs(ctx, scope)
	if err != nil || len(ids) == 0 {
		return []mikrotikservice.ProductInfo{}, err
	}
	found, err := c.products.FindByIDs(ctx, scope, ids)
	if err != nil {
		return nil, err
	}
	out := make([]mikrotikservice.ProductInfo, 0, len(found))
	for _, p := range found {
		if p.Pricing.ChargeType == pricing.Recurring {
			out = append(out, productInfo(p))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if a != b {
			return a < b
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// ---- read-only SQL over receivable tables ----

// mikrotikReceivableReader runs read-only, organization-scoped queries (RLS
// via app.organization_id plus an explicit organization_id predicate).
type mikrotikReceivableReader struct {
	db  *database.Pool
	now func() time.Time
}

func newMikrotikReceivableReader(db *database.Pool) mikrotikReceivableReader {
	return mikrotikReceivableReader{db: db, now: time.Now}
}

func (r mikrotikReceivableReader) scopedRead(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", scope.OrganizationID()); err != nil {
		return err
	}
	return fn(tx)
}

// ActiveContractProductIDs returns distinct product ids on items of active contracts.
func (r mikrotikReceivableReader) ActiveContractProductIDs(ctx context.Context, scope coretenant.Scope) ([]string, error) {
	out := []string{}
	err := r.scopedRead(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT ci.product_id::text
			FROM receivable_contract_items ci
			JOIN receivable_contracts c ON c.organization_id = ci.organization_id AND c.id = ci.contract_id
			WHERE c.organization_id = $1 AND c.status = 'active' AND ci.product_id IS NOT NULL`,
			scope.OrganizationID())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			out = append(out, id)
		}
		return rows.Err()
	})
	return out, err
}

// ForProduct counts the active contracts and accounts holding the product and
// the open invoices already past due (business date, WIB) from effectiveFrom on.
func (r mikrotikReceivableReader) ForProduct(ctx context.Context, scope coretenant.Scope, productID string, effectiveFrom time.Time) (mikrotikservice.ProductUsage, error) {
	var u mikrotikservice.ProductUsage
	today := businesstime.DayOf(r.now())
	from := businesstime.DayOf(effectiveFrom)
	err := r.scopedRead(ctx, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT count(DISTINCT c.id), count(DISTINCT c.account_id)
			FROM receivable_contracts c
			JOIN receivable_contract_items ci ON ci.organization_id = c.organization_id AND ci.contract_id = c.id
			WHERE c.organization_id = $1 AND c.status = 'active' AND ci.product_id = $2::uuid`,
			scope.OrganizationID(), productID).Scan(&u.ActiveContracts, &u.Accounts); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT count(DISTINCT i.id)
			FROM receivable_invoices i
			JOIN receivable_invoice_items ii ON ii.organization_id = i.organization_id AND ii.invoice_id = i.id
			WHERE i.organization_id = $1 AND i.status IN ('issued', 'overdue')
			  AND i.due_date < $3::date AND i.due_date >= $4::date AND ii.product_id = $2::uuid`,
			scope.OrganizationID(), productID, today, from).Scan(&u.OverdueInvoices)
	})
	return u, err
}
