package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
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
	fixedAssetRepo := financerepo.NewFixedAssetRepository(db)
	taxRepo := financerepo.NewTaxRepository(db)

	fiscalSvc := financeservice.NewFiscalService(fiscalRepo)
	cashBankSvc := financeservice.NewCashBankService(cashBankRepo, journalRepo, ledgerRepo)
	arapSvc := financeservice.NewARAPService(arapRepo, journalRepo)
	fixedAssetSvc := financeservice.NewFixedAssetService(fixedAssetRepo, journalRepo)
	taxSvc := financeservice.NewTaxService(taxRepo, journalRepo)
	ledgerSvc := financeservice.NewLedgerService(ledgerRepo, accountRepo)

	fy, err := fiscalSvc.CreateFiscalYear(ctx, dto.CreateFiscalYearRequest{Year: 2097})
	must(err, "create fiscal year")
	fmt.Println("fiscal year:", fy.Year)

	accounts, err := accountRepo.List(ctx, financerepo.AccountListFilter{})
	must(err, "list accounts")
	var piutangID, utangID, pendapatanID, bebanID, bankID, peralatanID, akumPenyusutanID, bebanPenyusutanID, ppnKeluaranID string
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
		case "1201":
			peralatanID = a.ID
		case "1202":
			akumPenyusutanID = a.ID
		case "5205":
			bebanPenyusutanID = a.ID
		case "2102":
			ppnKeluaranID = a.ID
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

	// Fixed asset: peralatan kantor 3.600.000, umur 12 bulan, dibeli tunai dari bank.
	category, err := fixedAssetSvc.CreateCategory(ctx, dto.CreateAssetCategoryRequest{
		Code: "PERALATAN-SMOKE", Name: "Peralatan Kantor (smoke test)",
		AssetAccountID: peralatanID, AccumulatedDepreciationAccountID: akumPenyusutanID,
		DepreciationExpenseAccountID: bebanPenyusutanID,
	})
	must(err, "create asset category")

	asset, err := fixedAssetSvc.CreateAsset(ctx, dto.CreateFixedAssetRequest{
		AssetCategoryID: category.ID, AssetCode: "AST-SMOKE-001", AssetName: "Laptop Kantor (smoke test)",
		AcquisitionDate: "2097-01-15", AcquisitionCost: "3600000", UsefulLifeMonths: 12, ContraAccountID: bankID,
	}, "")
	must(err, "create fixed asset")
	fmt.Println("fixed asset:", asset.AssetCode, "cost", asset.AcquisitionCost, "book value", asset.BookValue)

	schedule, err := fixedAssetSvc.ListSchedules(ctx, asset.ID)
	must(err, "list depreciation schedule")
	if len(schedule) != 12 {
		fmt.Println("FAIL: expected 12 depreciation schedule rows, got", len(schedule))
		os.Exit(1)
	}
	scheduleTotal, err := sumScheduleAmounts(schedule)
	must(err, "sum depreciation schedule")
	if scheduleTotal != "3600000.00" {
		fmt.Println("FAIL: expected schedule to sum to 3600000.00, got", scheduleTotal)
		os.Exit(1)
	}
	fmt.Println("depreciation schedule: 12 rows, total", scheduleTotal)

	// Post depreciation through end of January — should post exactly period 1.
	postResult, err := fixedAssetSvc.PostDepreciation(ctx, dto.PostDepreciationRequest{AsOfDate: "2097-01-31"}, "")
	must(err, "post depreciation (first run)")
	fmt.Println("post depreciation run 1: posted", postResult.PostedCount, "failed", postResult.FailedCount)
	if postResult.PostedCount != 1 {
		fmt.Println("FAIL: expected exactly 1 schedule row posted, got", postResult.PostedCount)
		os.Exit(1)
	}

	assetAfterPost, err := fixedAssetSvc.ListAssets(ctx, dto.FixedAssetListQuery{AssetCategoryID: category.ID})
	must(err, "list assets after posting")
	if len(assetAfterPost.Items) != 1 || assetAfterPost.Items[0].AccumulatedDepreciation != "300000.00" {
		fmt.Println("FAIL: expected accumulated depreciation 300000.00 after posting period 1")
		os.Exit(1)
	}
	fmt.Println("asset after posting: accumulated depreciation", assetAfterPost.Items[0].AccumulatedDepreciation,
		"book value", assetAfterPost.Items[0].BookValue)

	// Re-running the same as-of date must be a no-op (idempotent).
	postResultAgain, err := fixedAssetSvc.PostDepreciation(ctx, dto.PostDepreciationRequest{AsOfDate: "2097-01-31"}, "")
	must(err, "post depreciation (second run)")
	if postResultAgain.PostedCount != 0 {
		fmt.Println("FAIL: expected re-running post-depreciation for the same date to post 0 rows, got", postResultAgain.PostedCount)
		os.Exit(1)
	}
	fmt.Println("post depreciation run 2 (idempotency check): posted", postResultAgain.PostedCount)

	// Tax: PPN Keluaran collected on a cash sale, then settled to the state.
	taxTypes, err := taxSvc.ListTypes(ctx)
	must(err, "list tax types")
	var ppnKeluaranTypeID string
	for _, tt := range taxTypes {
		if tt.Code == "PPN_KELUARAN" {
			ppnKeluaranTypeID = tt.ID
		}
	}
	if ppnKeluaranTypeID == "" {
		fmt.Println("FAIL: PPN_KELUARAN tax type not found (seed missing?)")
		os.Exit(1)
	}

	_, err = taxSvc.CreateTransaction(ctx, dto.CreateTaxTransactionRequest{
		TaxTypeID: ppnKeluaranTypeID, TransactionDate: "2097-02-10", Amount: "220000",
		Direction: "increase", TaxAccountID: ppnKeluaranID, ContraAccountID: bankID,
		Description: "PPN keluaran atas penjualan tunai (smoke test)",
	}, "")
	must(err, "create tax transaction (increase)")

	_, err = taxSvc.CreateTransaction(ctx, dto.CreateTaxTransactionRequest{
		TaxTypeID: ppnKeluaranTypeID, TransactionDate: "2097-03-10", Amount: "220000",
		Direction: "decrease", TaxAccountID: ppnKeluaranID, ContraAccountID: bankID,
		Description: "Setor PPN ke kas negara (smoke test)",
	}, "")
	must(err, "create tax transaction (decrease)")

	taxSummary, err := taxSvc.Summary(ctx, dto.TaxSummaryQuery{StartDate: "2097-01-01", EndDate: "2097-12-31"})
	must(err, "tax summary")
	var ppnRow *dto.TaxSummaryRowResponse
	for i := range taxSummary.Rows {
		if taxSummary.Rows[i].TaxTypeCode == "PPN_KELUARAN" {
			ppnRow = &taxSummary.Rows[i]
		}
	}
	if ppnRow == nil || ppnRow.Increase != "220000.00" || ppnRow.Decrease != "220000.00" || ppnRow.Net != "0.00" {
		fmt.Println("FAIL: expected PPN_KELUARAN summary increase=220000.00 decrease=220000.00 net=0.00, got", ppnRow)
		os.Exit(1)
	}
	fmt.Println("tax summary PPN_KELUARAN: increase", ppnRow.Increase, "decrease", ppnRow.Decrease, "net", ppnRow.Net)

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

func sumScheduleAmounts(schedule []dto.DepreciationScheduleResponse) (string, error) {
	total := new(big.Rat)
	for _, row := range schedule {
		amount, ok := new(big.Rat).SetString(row.DepreciationAmount)
		if !ok {
			return "", fmt.Errorf("invalid depreciation amount %q", row.DepreciationAmount)
		}
		total.Add(total, amount)
	}
	return total.FloatString(2), nil
}
