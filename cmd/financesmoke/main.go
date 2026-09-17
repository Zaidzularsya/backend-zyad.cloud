package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"zyad.cloud/internal/config"
	"zyad.cloud/internal/modules/finance/dto"
	financerepo "zyad.cloud/internal/modules/finance/repository"
	financeservice "zyad.cloud/internal/modules/finance/service"
	"zyad.cloud/internal/platform/database"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()
	db, err := database.Connect(ctx, cfg.Database, slog.Default())
	if err != nil {
		fmt.Println("connect error:", err)
		os.Exit(1)
	}
	defer db.Close()

	accountRepo := financerepo.NewAccountRepository(db)
	fiscalRepo := financerepo.NewFiscalRepository(db)
	journalRepo := financerepo.NewJournalRepository(db)
	ledgerRepo := financerepo.NewLedgerRepository(db)
	cashBankRepo := financerepo.NewCashBankRepository(db)
	arapRepo := financerepo.NewARAPRepository(db)

	fiscalSvc := financeservice.NewFiscalService(fiscalRepo)
	cashBankSvc := financeservice.NewCashBankService(cashBankRepo, journalRepo, ledgerRepo)
	arapSvc := financeservice.NewARAPService(arapRepo, journalRepo)
	ledgerSvc := financeservice.NewLedgerService(ledgerRepo, accountRepo)

	fy, err := fiscalSvc.CreateFiscalYear(ctx, dto.CreateFiscalYearRequest{Year: 2097})
	must(err, "create fiscal year")
	fmt.Println("fiscal year:", fy.Year)

	accounts, err := accountRepo.List(ctx, financerepo.AccountListFilter{})
	must(err, "list accounts")
	var piutangID, utangID, pendapatanID, bebanID, bankID string
	for _, a := range accounts {
		switch a.AccountCode {
		case "1103":
			piutangID = a.ID
		case "2101":
			utangID = a.ID
		case "4101":
			pendapatanID = a.ID
		case "5201":
			bebanID = a.ID
		case "1102":
			bankID = a.ID
		}
	}

	bankCba, err := cashBankSvc.CreateAccount(ctx, dto.CreateCashBankAccountRequest{AccountID: bankID, Type: "bank"})
	must(err, "create bank cba")

	customer, err := arapSvc.CreatePartner(ctx, dto.CreateBusinessPartnerRequest{
		PartnerType: "customer", Code: "CUST-SMOKE", Name: "PT Pelanggan Uji", ControlAccountID: piutangID,
	})
	must(err, "create customer")
	vendor, err := arapSvc.CreatePartner(ctx, dto.CreateBusinessPartnerRequest{
		PartnerType: "vendor", Code: "VEND-SMOKE", Name: "CV Pemasok Uji", ControlAccountID: utangID,
	})
	must(err, "create vendor")
	fmt.Println("partners:", customer.ID, vendor.ID)

	// AR: invoice pelanggan 2.000.000
	arTx, err := arapSvc.CreateTransaction(ctx, dto.CreateARAPTransactionRequest{
		PartnerID: customer.ID, TransactionType: "receivable", TransactionDate: "2097-03-01", DueDate: "2097-03-31",
		Amount: "2000000", ContraAccountID: pendapatanID, Description: "Invoice langganan (smoke test)",
	}, "")
	must(err, "create AR transaction")
	fmt.Println("AR tx:", arTx.Status, arTx.OutstandingAmount)

	// AP: tagihan vendor 800.000
	apTx, err := arapSvc.CreateTransaction(ctx, dto.CreateARAPTransactionRequest{
		PartnerID: vendor.ID, TransactionType: "payable", TransactionDate: "2097-03-05", DueDate: "2097-04-05",
		Amount: "800000", ContraAccountID: bebanID, Description: "Tagihan hosting (smoke test)",
	}, "")
	must(err, "create AP transaction")
	fmt.Println("AP tx:", apTx.Status, apTx.OutstandingAmount)

	// Partial payment on AR: 1.200.000
	arPayment, err := arapSvc.CreatePayment(ctx, dto.CreateARAPPaymentRequest{
		PartnerID: customer.ID, ARAPTransactionID: arTx.ID, PaymentDate: "2097-03-15",
		Amount: "1200000", CashBankAccountID: bankCba.ID,
	}, "")
	must(err, "create AR payment")
	fmt.Println("AR payment:", arPayment.Amount)

	arTxAfter, err := arapRepoFindTx(ctx, arapRepo, arTx.ID)
	must(err, "refetch AR tx")
	fmt.Println("AR tx after partial payment: status=", arTxAfter.Status, "paid=", arTxAfter.PaidAmount, "outstanding=", arTxAfter.OutstandingAmount)
	if string(arTxAfter.Status) != "partially_paid" {
		fmt.Println("FAIL: expected partially_paid status")
		os.Exit(1)
	}

	// Full payment on AP: 800.000
	_, err = arapSvc.CreatePayment(ctx, dto.CreateARAPPaymentRequest{
		PartnerID: vendor.ID, ARAPTransactionID: apTx.ID, PaymentDate: "2097-03-20",
		Amount: "800000", CashBankAccountID: bankCba.ID,
	}, "")
	must(err, "create AP payment")
	apTxAfter, err := arapRepoFindTx(ctx, arapRepo, apTx.ID)
	must(err, "refetch AP tx")
	fmt.Println("AP tx after full payment: status=", apTxAfter.Status, "outstanding=", apTxAfter.OutstandingAmount)
	if string(apTxAfter.Status) != "paid" {
		fmt.Println("FAIL: expected paid status")
		os.Exit(1)
	}

	aging, err := arapSvc.AgingReport(ctx, dto.AgingReportQuery{TransactionType: "receivable", AsOfDate: "2097-12-31"})
	must(err, "aging report")
	fmt.Println("aging rows:", len(aging.Rows), "grand total:", aging.GrandTotal)

	tb, err := ledgerSvc.TrialBalance(ctx, dto.TrialBalanceQuery{AsOfDate: "2097-12-31"})
	must(err, "trial balance")
	fmt.Println("trial balance:", tb.TotalDebit, tb.TotalCredit, "balanced:", tb.IsBalanced)
	if !tb.IsBalanced {
		fmt.Println("FAIL: trial balance not balanced")
		os.Exit(1)
	}

	fmt.Println("SMOKE TEST PASSED")
}

func arapRepoFindTx(ctx context.Context, repo *financerepo.ARAPRepository, id string) (findTxResult, error) {
	t, err := repo.FindTransactionByID(ctx, id)
	if err != nil {
		return findTxResult{}, err
	}
	return findTxResult{Status: string(t.Status), PaidAmount: t.PaidAmount, OutstandingAmount: t.OutstandingAmount}, nil
}

type findTxResult struct {
	Status            string
	PaidAmount        string
	OutstandingAmount string
}

func must(err error, step string) {
	if err != nil {
		fmt.Println("FAIL at", step, ":", err)
		os.Exit(1)
	}
}
