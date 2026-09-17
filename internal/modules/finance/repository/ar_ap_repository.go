package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"zyad.cloud/internal/modules/finance/model"
	"zyad.cloud/internal/platform/database"
)

var ErrARAPTransactionTypeMismatch = errors.New("finance: transaction_type does not match partner_type")
var ErrARAPOverpayment = errors.New("finance: payment amount exceeds outstanding balance")

const businessPartnerSelectColumns = `
	bp.id,
	bp.partner_type,
	bp.code,
	bp.name,
	COALESCE(bp.tax_id, ''),
	COALESCE(bp.address, ''),
	bp.control_account_id,
	a.account_code,
	a.account_name,
	bp.is_active,
	bp.created_at,
	bp.updated_at,
	bp.deleted_at
`

const businessPartnerFrom = `
	FROM finance_business_partners bp
	JOIN finance_accounts a ON a.id = bp.control_account_id
`

type ARAPRepository struct {
	db *database.Pool
}

func NewARAPRepository(db *database.Pool) *ARAPRepository {
	return &ARAPRepository{db: db}
}

type CreateBusinessPartnerParams struct {
	PartnerType      model.PartnerType
	Code             string
	Name             string
	TaxID            string
	Address          string
	ControlAccountID string
}

type UpdateBusinessPartnerParams struct {
	ID       string
	Name     string
	TaxID    string
	Address  string
	IsActive bool
}

func (r *ARAPRepository) CreatePartner(ctx context.Context, params CreateBusinessPartnerParams) (model.BusinessPartner, error) {
	var id string
	err := r.db.QueryRow(ctx, `
		INSERT INTO finance_business_partners (partner_type, code, name, tax_id, address, control_account_id)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6::uuid)
		RETURNING id
	`, string(params.PartnerType), strings.TrimSpace(params.Code), strings.TrimSpace(params.Name),
		params.TaxID, params.Address, strings.TrimSpace(params.ControlAccountID),
	).Scan(&id)
	if err != nil {
		return model.BusinessPartner{}, err
	}
	return r.FindPartnerByID(ctx, id)
}

func (r *ARAPRepository) UpdatePartner(ctx context.Context, params UpdateBusinessPartnerParams) (model.BusinessPartner, error) {
	_, err := r.db.Exec(ctx, `
		UPDATE finance_business_partners
		SET name = $2, tax_id = NULLIF($3, ''), address = NULLIF($4, ''), is_active = $5, updated_at = now()
		WHERE id = $1::uuid AND deleted_at IS NULL
	`, strings.TrimSpace(params.ID), strings.TrimSpace(params.Name), params.TaxID, params.Address, params.IsActive)
	if err != nil {
		return model.BusinessPartner{}, err
	}
	return r.FindPartnerByID(ctx, params.ID)
}

func (r *ARAPRepository) FindPartnerByID(ctx context.Context, id string) (model.BusinessPartner, error) {
	var p model.BusinessPartner
	err := r.db.QueryRow(ctx, `
		SELECT `+businessPartnerSelectColumns+businessPartnerFrom+`
		WHERE bp.id = $1::uuid AND bp.deleted_at IS NULL
	`, strings.TrimSpace(id)).Scan(businessPartnerScanDest(&p)...)
	if err != nil {
		return model.BusinessPartner{}, err
	}
	return p, nil
}

type BusinessPartnerListFilter struct {
	PartnerType     model.PartnerType
	IncludeInactive bool
}

func (r *ARAPRepository) ListPartners(ctx context.Context, filter BusinessPartnerListFilter) ([]model.BusinessPartner, error) {
	where := " WHERE bp.deleted_at IS NULL"
	args := []any{}
	if filter.PartnerType != "" {
		args = append(args, string(filter.PartnerType))
		where += fmt.Sprintf(" AND bp.partner_type = $%d", len(args))
	}
	if !filter.IncludeInactive {
		where += " AND bp.is_active = true"
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+businessPartnerSelectColumns+businessPartnerFrom+where+`
		ORDER BY bp.code ASC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	partners := make([]model.BusinessPartner, 0)
	for rows.Next() {
		var p model.BusinessPartner
		if err := rows.Scan(businessPartnerScanDest(&p)...); err != nil {
			return nil, err
		}
		partners = append(partners, p)
	}
	return partners, rows.Err()
}

func businessPartnerScanDest(p *model.BusinessPartner) []any {
	return []any{
		&p.ID,
		&p.PartnerType,
		&p.Code,
		&p.Name,
		&p.TaxID,
		&p.Address,
		&p.ControlAccountID,
		&p.ControlAccountCode,
		&p.ControlAccountName,
		&p.IsActive,
		&p.CreatedAt,
		&p.UpdatedAt,
		&p.DeletedAt,
	}
}

type CreateARAPTransactionParams struct {
	PartnerID       string
	TransactionType model.ARAPTransactionType
	TransactionDate time.Time
	DueDate         time.Time
	ReferenceNumber string
	Amount          string
	ContraAccountID string
	Description     string
	EntryNumber     string
	CreatedBy       *string
}

// CreateTransaction posts the recognition journal entry (Dr/Cr resolved from
// transaction type against the partner's control account) and inserts the
// ar_ap_transactions row referencing it, in one DB transaction.
func (r *ARAPRepository) CreateTransaction(ctx context.Context, params CreateARAPTransactionParams) (model.ARAPTransaction, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.ARAPTransaction{}, err
	}
	defer tx.Rollback(ctx)

	var partnerType string
	var controlAccountID string
	if err := tx.QueryRow(ctx, `SELECT partner_type, control_account_id FROM finance_business_partners WHERE id = $1::uuid`, params.PartnerID).
		Scan(&partnerType, &controlAccountID); err != nil {
		return model.ARAPTransaction{}, err
	}
	if (params.TransactionType == model.ARAPTransactionTypeReceivable && partnerType != string(model.PartnerTypeCustomer)) ||
		(params.TransactionType == model.ARAPTransactionTypePayable && partnerType != string(model.PartnerTypeVendor)) {
		return model.ARAPTransaction{}, ErrARAPTransactionTypeMismatch
	}

	subledgerType := model.SubledgerCustomer
	var debitAccountID, creditAccountID string
	if params.TransactionType == model.ARAPTransactionTypeReceivable {
		debitAccountID, creditAccountID = controlAccountID, params.ContraAccountID
	} else {
		debitAccountID, creditAccountID = params.ContraAccountID, controlAccountID
		subledgerType = model.SubledgerVendor
	}

	controlLine := CreateJournalLineParams{AccountID: controlAccountID, SubledgerType: subledgerType, SubledgerID: &params.PartnerID}
	var lines []CreateJournalLineParams
	if params.TransactionType == model.ARAPTransactionTypeReceivable {
		controlLine.Debit = params.Amount
		lines = []CreateJournalLineParams{controlLine, {AccountID: creditAccountID, Credit: params.Amount}}
	} else {
		controlLine.Credit = params.Amount
		lines = []CreateJournalLineParams{{AccountID: debitAccountID, Debit: params.Amount}, controlLine}
	}

	entryID, err := InsertJournalEntryTx(ctx, tx, CreateJournalEntryParams{
		EntryNumber: params.EntryNumber,
		EntryDate:   params.TransactionDate,
		SourceType:  arapJournalSourceType(params.TransactionType),
		Reference:   params.ReferenceNumber,
		Description: params.Description,
		CreatedBy:   params.CreatedBy,
		Lines:       lines,
	})
	if err != nil {
		return model.ARAPTransaction{}, err
	}

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO finance_ar_ap_transactions (
			partner_id, transaction_type, transaction_date, due_date, reference_number,
			amount, contra_account_id, description, journal_entry_id, created_by
		)
		VALUES ($1::uuid, $2, $3, $4, NULLIF($5, ''), $6::numeric, $7::uuid, NULLIF($8, ''), $9::uuid, $10::uuid)
		RETURNING id
	`,
		params.PartnerID, string(params.TransactionType), params.TransactionDate, params.DueDate, params.ReferenceNumber,
		amountOrZero(params.Amount), params.ContraAccountID, params.Description, entryID, nullableUUID(params.CreatedBy),
	).Scan(&id)
	if err != nil {
		return model.ARAPTransaction{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.ARAPTransaction{}, err
	}
	return r.FindTransactionByID(ctx, id)
}

const arapTransactionSelectColumns = `
	t.id,
	t.partner_id,
	bp.code,
	bp.name,
	t.transaction_type,
	t.transaction_date,
	t.due_date,
	COALESCE(t.reference_number, ''),
	t.amount::text,
	t.contra_account_id,
	ca.account_code,
	ca.account_name,
	COALESCE(t.description, ''),
	t.status,
	t.journal_entry_id,
	t.created_at,
	t.updated_at,
	COALESCE(alloc.paid, 0)::text,
	(t.amount - COALESCE(alloc.paid, 0))::text
`

const arapTransactionFrom = `
	FROM finance_ar_ap_transactions t
	JOIN finance_business_partners bp ON bp.id = t.partner_id
	JOIN finance_accounts ca ON ca.id = t.contra_account_id
	LEFT JOIN (
		SELECT ar_ap_transaction_id, SUM(allocated_amount) AS paid
		FROM finance_ar_ap_payment_allocations
		GROUP BY ar_ap_transaction_id
	) alloc ON alloc.ar_ap_transaction_id = t.id
`

func (r *ARAPRepository) FindTransactionByID(ctx context.Context, id string) (model.ARAPTransaction, error) {
	var t model.ARAPTransaction
	err := r.db.QueryRow(ctx, `
		SELECT `+arapTransactionSelectColumns+arapTransactionFrom+`
		WHERE t.id = $1::uuid
	`, strings.TrimSpace(id)).Scan(arapTransactionScanDest(&t)...)
	if err != nil {
		return model.ARAPTransaction{}, err
	}
	return t, nil
}

type ARAPTransactionListFilter struct {
	PartnerID       string
	TransactionType model.ARAPTransactionType
	Status          model.ARAPTransactionStatus
	Limit           int
	Offset          int
}

func (r *ARAPRepository) ListTransactions(ctx context.Context, filter ARAPTransactionListFilter) ([]model.ARAPTransaction, int64, error) {
	conditions := []string{}
	args := []any{}
	if filter.PartnerID != "" {
		args = append(args, filter.PartnerID)
		conditions = append(conditions, fmt.Sprintf("t.partner_id = $%d::uuid", len(args)))
	}
	if filter.TransactionType != "" {
		args = append(args, string(filter.TransactionType))
		conditions = append(conditions, fmt.Sprintf("t.transaction_type = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		conditions = append(conditions, fmt.Sprintf("t.status = $%d", len(args)))
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT count(*) "+arapTransactionFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit, offset := pagination(filter.Limit, filter.Offset)
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, `
		SELECT `+arapTransactionSelectColumns+arapTransactionFrom+where+`
		ORDER BY t.due_date ASC, t.created_at ASC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	transactions := make([]model.ARAPTransaction, 0)
	for rows.Next() {
		var t model.ARAPTransaction
		if err := rows.Scan(arapTransactionScanDest(&t)...); err != nil {
			return nil, 0, err
		}
		transactions = append(transactions, t)
	}
	return transactions, total, rows.Err()
}

func arapTransactionScanDest(t *model.ARAPTransaction) []any {
	return []any{
		&t.ID,
		&t.PartnerID,
		&t.PartnerCode,
		&t.PartnerName,
		&t.TransactionType,
		&t.TransactionDate,
		&t.DueDate,
		&t.ReferenceNumber,
		&t.Amount,
		&t.ContraAccountID,
		&t.ContraAccountCode,
		&t.ContraAccountName,
		&t.Description,
		&t.Status,
		&t.JournalEntryID,
		&t.CreatedAt,
		&t.UpdatedAt,
		&t.PaidAmount,
		&t.OutstandingAmount,
	}
}

type CreateARAPPaymentParams struct {
	ARAPTransactionID string
	PaymentDate       time.Time
	Amount            string
	CashBankAccountID string
	Notes             string
	EntryNumber       string
	CreatedBy         *string
}

// CreatePayment posts the settlement journal entry (Dr/Cr resolved from the
// transaction's type against the partner's control account and the chosen
// cash/bank account), inserts the payment row, allocates the full payment
// amount to the given transaction, and recomputes that transaction's status
// — all within a single DB transaction.
func (r *ARAPRepository) CreatePayment(ctx context.Context, params CreateARAPPaymentParams) (model.ARAPPayment, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.ARAPPayment{}, err
	}
	defer tx.Rollback(ctx)

	var partnerID, transactionType, txStatus string
	var txAmount string
	if err := tx.QueryRow(ctx, `
		SELECT partner_id, transaction_type, status, amount::text
		FROM finance_ar_ap_transactions WHERE id = $1::uuid FOR UPDATE
	`, params.ARAPTransactionID).Scan(&partnerID, &transactionType, &txStatus, &txAmount); err != nil {
		return model.ARAPPayment{}, err
	}
	if txStatus == string(model.ARAPStatusPaid) || txStatus == string(model.ARAPStatusVoid) {
		return model.ARAPPayment{}, fmt.Errorf("finance: ar/ap transaction is already %s", txStatus)
	}

	var controlAccountID string
	if err := tx.QueryRow(ctx, `SELECT control_account_id FROM finance_business_partners WHERE id = $1::uuid`, partnerID).
		Scan(&controlAccountID); err != nil {
		return model.ARAPPayment{}, err
	}
	var cashBankGLAccountID string
	if err := tx.QueryRow(ctx, `SELECT account_id FROM finance_cash_bank_accounts WHERE id = $1::uuid`, params.CashBankAccountID).
		Scan(&cashBankGLAccountID); err != nil {
		return model.ARAPPayment{}, err
	}

	subledgerType := model.SubledgerCustomer
	controlLine := CreateJournalLineParams{AccountID: controlAccountID, SubledgerType: subledgerType, SubledgerID: &partnerID}
	var lines []CreateJournalLineParams
	if transactionType == string(model.ARAPTransactionTypeReceivable) {
		controlLine.Credit = params.Amount
		lines = []CreateJournalLineParams{{AccountID: cashBankGLAccountID, Debit: params.Amount}, controlLine}
	} else {
		controlLine.SubledgerType = model.SubledgerVendor
		controlLine.Debit = params.Amount
		lines = []CreateJournalLineParams{controlLine, {AccountID: cashBankGLAccountID, Credit: params.Amount}}
	}

	entryID, err := InsertJournalEntryTx(ctx, tx, CreateJournalEntryParams{
		EntryNumber: params.EntryNumber,
		EntryDate:   params.PaymentDate,
		SourceType:  arapJournalSourceType(model.ARAPTransactionType(transactionType)),
		Description: params.Notes,
		CreatedBy:   params.CreatedBy,
		Lines:       lines,
	})
	if err != nil {
		return model.ARAPPayment{}, err
	}

	var paymentID string
	err = tx.QueryRow(ctx, `
		INSERT INTO finance_ar_ap_payments (partner_id, payment_date, amount, cash_bank_account_id, journal_entry_id, notes, created_by)
		VALUES ($1::uuid, $2, $3::numeric, $4::uuid, $5::uuid, NULLIF($6, ''), $7::uuid)
		RETURNING id
	`, partnerID, params.PaymentDate, amountOrZero(params.Amount), params.CashBankAccountID, entryID, params.Notes, nullableUUID(params.CreatedBy)).
		Scan(&paymentID)
	if err != nil {
		return model.ARAPPayment{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO finance_ar_ap_payment_allocations (payment_id, ar_ap_transaction_id, allocated_amount)
		VALUES ($1::uuid, $2::uuid, $3::numeric)
	`, paymentID, params.ARAPTransactionID, amountOrZero(params.Amount)); err != nil {
		return model.ARAPPayment{}, err
	}

	var totalPaid string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(allocated_amount), 0)::text
		FROM finance_ar_ap_payment_allocations
		WHERE ar_ap_transaction_id = $1::uuid
	`, params.ARAPTransactionID).Scan(&totalPaid); err != nil {
		return model.ARAPPayment{}, err
	}

	amountRat, err1 := parseMoneyForCompare(txAmount)
	paidRat, err2 := parseMoneyForCompare(totalPaid)
	if err1 != nil || err2 != nil {
		return model.ARAPPayment{}, fmt.Errorf("finance: invalid amount on transaction %s", params.ARAPTransactionID)
	}
	if paidRat.Cmp(amountRat) > 0 {
		return model.ARAPPayment{}, ErrARAPOverpayment
	}

	newStatus := arapStatusFromPayments(txAmount, totalPaid)
	if _, err := tx.Exec(ctx, `
		UPDATE finance_ar_ap_transactions SET status = $2, updated_at = now() WHERE id = $1::uuid
	`, params.ARAPTransactionID, newStatus); err != nil {
		return model.ARAPPayment{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.ARAPPayment{}, err
	}
	return r.FindPaymentByID(ctx, paymentID)
}

const arapPaymentSelectColumns = `
	p.id,
	p.partner_id,
	p.payment_date,
	p.amount::text,
	p.cash_bank_account_id,
	a.account_code || ' — ' || a.account_name,
	p.journal_entry_id,
	COALESCE(p.notes, ''),
	p.created_at
`

const arapPaymentFrom = `
	FROM finance_ar_ap_payments p
	JOIN finance_cash_bank_accounts cba ON cba.id = p.cash_bank_account_id
	JOIN finance_accounts a ON a.id = cba.account_id
`

func (r *ARAPRepository) FindPaymentByID(ctx context.Context, id string) (model.ARAPPayment, error) {
	var pmt model.ARAPPayment
	err := r.db.QueryRow(ctx, `
		SELECT `+arapPaymentSelectColumns+arapPaymentFrom+`
		WHERE p.id = $1::uuid
	`, strings.TrimSpace(id)).Scan(
		&pmt.ID, &pmt.PartnerID, &pmt.PaymentDate, &pmt.Amount, &pmt.CashBankAccountID,
		&pmt.CashBankAccountLabel, &pmt.JournalEntryID, &pmt.Notes, &pmt.CreatedAt,
	)
	if err != nil {
		return model.ARAPPayment{}, err
	}
	return pmt, nil
}

type AgingBucketRow struct {
	PartnerID   string
	PartnerCode string
	PartnerName string
	DueDate     time.Time
	Outstanding string
}

// AgingReport returns every open/partially_paid transaction of the given
// type with its outstanding balance (amount minus allocated payments) as of
// asOf, for the caller to bucket by due date.
func (r *ARAPRepository) AgingReport(ctx context.Context, transactionType model.ARAPTransactionType, asOf time.Time) ([]AgingBucketRow, error) {
	rows, err := r.db.Query(ctx, `
		WITH allocations AS (
			SELECT a.ar_ap_transaction_id, SUM(a.allocated_amount) AS paid
			FROM finance_ar_ap_payment_allocations a
			JOIN finance_ar_ap_payments p ON p.id = a.payment_id
			WHERE p.payment_date <= $2
			GROUP BY a.ar_ap_transaction_id
		)
		SELECT t.partner_id, bp.code, bp.name, t.due_date, (t.amount - COALESCE(al.paid, 0))::text
		FROM finance_ar_ap_transactions t
		JOIN finance_business_partners bp ON bp.id = t.partner_id
		LEFT JOIN allocations al ON al.ar_ap_transaction_id = t.id
		WHERE t.transaction_type = $1
			AND t.status <> 'void'
			AND t.transaction_date <= $2
			AND (t.amount - COALESCE(al.paid, 0)) > 0.005
		ORDER BY bp.code ASC, t.due_date ASC
	`, string(transactionType), asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]AgingBucketRow, 0)
	for rows.Next() {
		var row AgingBucketRow
		if err := rows.Scan(&row.PartnerID, &row.PartnerCode, &row.PartnerName, &row.DueDate, &row.Outstanding); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// arapJournalSourceType maps the AR/AP-domain transaction type to the
// finance_journal_entries.source_type value the DB check constraint allows
// ('ar'/'ap'), which is intentionally shorter than 'receivable'/'payable'.
func arapJournalSourceType(transactionType model.ARAPTransactionType) model.JournalSourceType {
	if transactionType == model.ARAPTransactionTypeReceivable {
		return model.JournalSourceAR
	}
	return model.JournalSourceAP
}

func arapStatusFromPayments(totalAmount, paidAmount string) string {
	amount, err1 := parseMoneyForCompare(totalAmount)
	paid, err2 := parseMoneyForCompare(paidAmount)
	if err1 != nil || err2 != nil {
		return string(model.ARAPStatusOpen)
	}
	switch {
	case paid.Cmp(amount) >= 0:
		return string(model.ARAPStatusPaid)
	case paid.Sign() > 0:
		return string(model.ARAPStatusPartiallyPaid)
	default:
		return string(model.ARAPStatusOpen)
	}
}
